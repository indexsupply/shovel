package shovel

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/indexsupply/x/dig"
	"github.com/indexsupply/x/eth"
	"github.com/indexsupply/x/jrpc2"
	"github.com/indexsupply/x/rlps"
	"github.com/indexsupply/x/shovel/cache"
	"github.com/indexsupply/x/shovel/config"
	"github.com/indexsupply/x/shovel/glf"
	"github.com/indexsupply/x/wctx"
	"github.com/indexsupply/x/wpg"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"
)

//go:embed schema.sql
var Schema string

type Source interface {
	Get(*glf.Filter, uint64, uint64) ([]eth.Block, error)
	LoadBlocks(*glf.Filter, []eth.Block) error
	Latest() (uint64, []byte, error)
	Hash(uint64) ([]byte, error)
}

type Destination interface {
	Name() string
	Insert(context.Context, wpg.Conn, []eth.Block) (int64, error)
	Delete(context.Context, wpg.Conn, uint64) error
	Events(context.Context) [][]byte
	Filter() glf.Filter
}

type Option func(t *Task)

func WithContext(ctx context.Context) Option {
	return func(t *Task) {
		t.ctx = ctx
	}
}

func WithCache(c *cache.Cache) Option {
	return func(t *Task) {
		t.cache = c
	}
}

func WithSourceFactory(f func(config.Source) Source) Option {
	return func(t *Task) {
		t.srcFactory = f
	}
}

func WithSource(src Source) Option {
	return func(t *Task) {
		t.src = src
	}
}

func WithSourceConfig(sc config.Source) Option {
	return func(t *Task) {
		t.srcConfig = sc
		t.ctx = wctx.WithChainID(t.ctx, sc.ChainID)
		t.ctx = wctx.WithSrcName(t.ctx, sc.Name)
	}
}

func WithDestFactory(f func(config.Integration) (Destination, error)) Option {
	return func(t *Task) {
		t.destFactory = f
	}
}

func WithDestConfig(ig config.Integration) Option {
	return func(t *Task) {
		t.destConfig = ig
	}
}

func WithPG(pg *pgxpool.Pool) Option {
	return func(t *Task) {
		t.pgp = pg
	}
}

func WithRange(start, stop uint64) Option {
	return func(t *Task) {
		t.start, t.stop = start, stop
	}
}

func WithConcurrency(numParts, batchSize int) Option {
	return func(t *Task) {
		t.batchSize = max(batchSize, 1)
		t.parts = parts(max(numParts, 1), max(batchSize, 1))
	}
}

var compiled = map[string]Destination{}

func NewDestination(ig config.Integration) (Destination, error) {
	switch {
	case len(ig.Compiled.Name) > 0:
		dest, ok := compiled[ig.Name]
		if !ok {
			return nil, fmt.Errorf("unable to find compiled integration: %s", ig.Name)
		}
		return dest, nil
	default:
		dest, err := dig.New(ig.Name, ig.Event, ig.Block, ig.Table)
		if err != nil {
			return nil, fmt.Errorf("building abi integration: %w", err)
		}
		return dest, nil
	}
}

func NewSource(sc config.Source) Source {
	switch {
	case strings.Contains(string(sc.URL), "rlps"):
		return rlps.NewClient(sc.ChainID, string(sc.URL))
	case strings.HasPrefix(string(sc.URL), "http"):
		return jrpc2.New(string(sc.URL))
	default:
		// TODO add back support for local geth
		panic(fmt.Sprintf("unsupported src type: %v", sc))
	}
}

func NewTask(opts ...Option) (*Task, error) {
	t := &Task{
		ctx:         context.Background(),
		batchSize:   1,
		parts:       parts(1, 1),
		srcFactory:  NewSource,
		destFactory: NewDestination,
	}
	for _, opt := range opts {
		opt(t)
	}
	for i := range t.parts {
		dest, err := t.destFactory(t.destConfig)
		if err != nil {
			return nil, fmt.Errorf("initializing destination: %w", err)
		}
		t.parts[i].dest = dest
	}

	t.filter = t.parts[0].dest.Filter()
	t.filterID = t.filter.ID()

	//for i := range t.parts {
	//	t.parts[i].src = t.srcFactory(t.srcConfig)
	//}
	t.lockid = wpg.LockHash(fmt.Sprintf(
		"shovel-task-%s-%s",
		t.srcConfig.Name,
		t.destConfig.Name,
	))
	_, err := t.pgp.Exec(t.ctx, fmt.Sprintf(
		"set application_name = 'shovel-task-%s-%s-%s'",
		t.srcConfig.Name,
		t.destConfig.Name,
		wctx.Version(t.ctx),
	))
	if err != nil {
		return nil, fmt.Errorf("setting application_name: %w", err)
	}
	slog.InfoContext(t.ctx, "new-task",
		"filter", t.filterID,
		"src", t.srcConfig.Name,
		"dest", t.destConfig.Name,
	)
	return t, nil
}

type Task struct {
	ctx   context.Context
	pgp   *pgxpool.Pool
	cache *cache.Cache

	lockid      int64
	start, stop uint64

	src        Source
	srcFactory func(config.Source) Source
	srcConfig  config.Source

	destFactory func(config.Integration) (Destination, error)
	destConfig  config.Integration

	filter    glf.Filter
	filterID  uint64
	batchSize int
	parts     []part
}

// Depending on WithConcurrency, a Task may be configured with
// concurrent parts. This allows a task to concurrently download
// block data from its Source.
//
// m and n are used to slice a batch of blocks: batch[m:n]
//
// Each part has its own source since the Source may have buffers
// that aren't safe to share across go-routines.
type part struct {
	m, n, size int
	src        Source
	dest       Destination
}

func (p *part) slice(b []eth.Block) []eth.Block {
	if len(b) < p.m {
		return nil
	}
	return b[p.m:min(p.n, len(b))]
}

func parts(numParts, batchSize int) []part {
	var (
		p         = make([]part, numParts)
		size      = batchSize / numParts
		remainder = batchSize % numParts
	)
	for i := range p {
		p[i].size = size
		p[i].m = size * i
		p[i].n = size + p[i].m
	}
	if remainder > 0 {
		p[numParts-1].n += remainder
	}
	if p[numParts-1].n != batchSize {
		panic(fmt.Sprintf("batchSize error want: %d got: %d", batchSize, p[numParts-1].n))
	}
	return p
}

func (t *Task) update(
	pg wpg.Conn,
	hash []byte,
	num uint64,
	delta uint64,
	nrows int64,
	elapsed time.Duration,
) error {
	const uq = `
		insert into shovel.task_updates (
			chain_id,
			src_name,
			dest_name,
			hash,
			num,
			stop,
			nblocks,
			nrows,
			latency
		)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	_, err := pg.Exec(t.ctx, uq,
		t.srcConfig.ChainID,
		t.srcConfig.Name,
		t.destConfig.Name,
		hash,
		num,
		t.stop,
		delta,
		nrows,
		elapsed,
	)
	return err
}

func (t *Task) delete(pg wpg.Conn, n uint64) error {
	const q = `
		delete from shovel.task_updates
		where src_name = $1
		and dest_name = $2
		and num >= $3
	`
	_, err := pg.Exec(t.ctx, q, t.srcConfig.Name, t.destConfig.Name, n)
	if err != nil {
		return fmt.Errorf("deleting block from task table: %w", err)
	}
	if len(t.parts) == 0 {
		return fmt.Errorf("unable to delete. missing parts.")
	}
	err = t.parts[0].dest.Delete(t.ctx, pg, n)
	if err != nil {
		return fmt.Errorf("deleting block: %w", err)
	}
	return nil
}

func (t *Task) latest(pg wpg.Conn) (uint64, []byte, error) {
	const q = `
		select num, hash
		from shovel.task_updates
		where src_name = $1
		and dest_name = $2
		order by num desc
		limit 1
	`
	localNum, localHash := uint64(0), []byte{}
	err := pg.QueryRow(
		t.ctx,
		q,
		t.srcConfig.Name,
		t.destConfig.Name,
	).Scan(&localNum, &localHash)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		switch {
		case t.start > 0:
			n := t.start - 1
			h, err := t.src.Hash(n)
			if err != nil {
				return 0, nil, fmt.Errorf("getting hash for %d: %w", n, err)
			}
			slog.InfoContext(t.ctx, "start at config", "num", t.start)
			return n, h, nil
		default:
			n, _, err := t.src.Latest()
			if err != nil {
				return 0, nil, err
			}
			h, err := t.src.Hash(n - 1)
			if err != nil {
				return 0, nil, fmt.Errorf("getting hash for %d: %w", n-1, err)
			}
			slog.InfoContext(t.ctx, "start at latest", "num", n)
			return n - 1, h, nil
		}
	case err != nil:
		return 0, nil, fmt.Errorf("querying for latest: %w", err)
	default:
		return localNum, localHash, nil
	}
}

var (
	ErrNothingNew = errors.New("no new blocks")
	ErrReorg      = errors.New("reorg")
	ErrDone       = errors.New("this is the end")
	ErrAhead      = errors.New("ahead")
)

// Indexes at most batchSize of the delta between min(g, limit) and pg.
// If pg contains an invalid latest block (ie reorg) then [ErrReorg]
// is returned and the caller may rollback the transaction resulting
// in no side-effects.
func (task *Task) Converge(notx bool) error {
	var (
		pg       wpg.Conn = task.pgp
		commit            = func() error { return nil }
		rollback          = func() error { return nil }
	)
	if !notx {
		pgTx, err := task.pgp.Begin(task.ctx)
		if err != nil {
			return err
		}
		commit = func() error { return pgTx.Commit(task.ctx) }
		rollback = func() error { return pgTx.Rollback(task.ctx) }
		defer rollback()
		pg = wpg.NewTxLocker(pgTx)
		const lockq = `select pg_advisory_xact_lock($1)`
		_, err = pg.Exec(task.ctx, lockq, task.lockid)
		if err != nil {
			return fmt.Errorf("task lock %d: %w", task.srcConfig.ChainID, err)
		}
	}
	for reorgs := 0; reorgs <= 10; {
		localNum, localHash, err := task.latest(pg)
		if err != nil {
			return fmt.Errorf("getting latest from task: %w", err)
		}
		if task.stop > 0 && localNum >= task.stop {
			return ErrDone
		}
		gethNum, gethHash, err := task.src.Latest()
		if err != nil {
			return fmt.Errorf("getting latest from eth: %w", err)
		}
		if task.stop > 0 && gethNum > task.stop {
			gethNum = task.stop
		}
		if localNum > gethNum {
			slog.ErrorContext(task.ctx, "ahead",
				"local", fmt.Sprintf("%d %.4x", localNum, localHash),
				"remote", fmt.Sprintf("%d %.4x", gethNum, gethHash),
			)
			return ErrAhead
		}
		if localNum == gethNum {
			return ErrNothingNew
		}
		delta := min(gethNum-localNum, uint64(task.batchSize))
		if delta == 0 {
			return ErrNothingNew
		}

		/*
			blocks, err := task.getBlocks(localHash, localNum, delta)
			switch {
			case errors.Is(err, ErrReorg):
				reorgs++
				slog.ErrorContext(task.ctx, "reorg", "n", localNum, "h", fmt.Sprintf("%.4x", localHash))
				err = task.delete(pg, localNum)
				if err != nil {
					return fmt.Errorf("deleting during reorg: %w", err)
				}
				continue
			case err != nil:
				return fmt.Errorf("loading blocks: %w", err)
			}

			nrows, err := task.insert(pg, blocks)
			if err != nil {
				return err
			}
		*/
		err = task.loadinsert(pg, localHash, localNum+1, delta)
		switch {
		case errors.Is(err, ErrReorg):
			reorgs++
			slog.ErrorContext(task.ctx, "reorg", "n", localNum, "h", fmt.Sprintf("%.4x", localHash))
			err = task.delete(pg, localNum)
			if err != nil {
				return fmt.Errorf("deleting during reorg: %w", err)
			}
			continue
		case err != nil:
			return fmt.Errorf("loading blocks: %w", err)
		}
		if err := commit(); err != nil {
			return fmt.Errorf("commit converge tx: %w", err)
		}
		return nil
	}
	return errors.Join(ErrReorg, rollback())
}

func (t *Task) loadinsert(pg wpg.Conn, localHash []byte, start, limit uint64) error {
	var (
		t0       = time.Now()
		eg       errgroup.Group
		nrows    int64
		lastHash []byte
		lastNum  uint64
	)
	for i := range t.parts {
		i := i
		m := start + uint64(t.parts[i].m)
		n := min(uint64(t.parts[i].size), limit)
		eg.Go(func() error {
			b, err := t.src.Get(&t.filter, m, n)
			if err != nil {
				return fmt.Errorf("loading blocks: %w", err)
			}
			n, err := t.parts[i].dest.Insert(t.ctx, pg, b)
			if err != nil {
				return fmt.Errorf("inserting blocks: %w", err)
			}
			atomic.AddInt64(&nrows, n)
			last := b[len(b)-1]
			if last.Num() > lastNum {
				lastNum = last.Num()
				lastHash = last.Hash()
			}
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return err
	}
	err := t.update(pg, lastHash, lastNum, limit, nrows, time.Since(t0))
	if err != nil {
		return err
	}
	slog.InfoContext(t.ctx, "insert",
		"src", t.srcConfig.Name,
		"dst", t.destConfig.Name,
		"n", lastNum,
		"nrows", nrows,
		"elapsed", time.Since(t0),
	)
	return nil
}

func (t *Task) getBlocks(h []byte, n, delta uint64) ([]eth.Block, error) {
	blocks := make([]eth.Block, delta)
	for i := range blocks {
		blocks[i].SetNum(n + uint64(i+1))
	}
	var eg errgroup.Group
	for i := range t.parts {
		i := i
		b := t.parts[i].slice(blocks)
		if len(b) == 0 {
			continue
		}
		eg.Go(func() error { return t.src.LoadBlocks(&t.filter, b) })
	}
	if err := eg.Wait(); err != nil {
		return nil, err
	}
	switch {
	case t.filter.UseBlocks, t.filter.UseHeaders:
		return blocks, validateChain(t.ctx, h, blocks)
	default:
		return blocks, nil
	}
}

func (t *Task) insert(pg wpg.Conn, blocks []eth.Block) (int64, error) {
	var (
		nrows int64
		eg    errgroup.Group
	)
	for i := range t.parts {
		i := i
		b := t.parts[i].slice(blocks)
		if len(b) == 0 {
			continue
		}
		eg.Go(func() error {
			count, err := t.parts[i].dest.Insert(t.ctx, pg, b)
			nrows += count
			return err
		})
	}
	return nrows, eg.Wait()
}

func validateChain(ctx context.Context, parent []byte, blks []eth.Block) error {
	if len(blks[0].Header.Parent) != 32 {
		return fmt.Errorf("corrupt parent: %x", blks[0].Header.Parent)
	}
	if !bytes.Equal(parent, blks[0].Header.Parent) {
		return ErrReorg
	}
	if len(blks) <= 1 {
		return nil
	}
	for i := 1; i < len(blks); i++ {
		prev, curr := blks[i-1], blks[i]
		if !bytes.Equal(curr.Header.Parent, prev.Hash()) {
			slog.ErrorContext(ctx, "corrupt-chain-segment",
				"num", prev.Num(),
				"hash", fmt.Sprintf("%.4x", prev.Header.Hash),
				"next-num", curr.Num(),
				"next-parent", fmt.Sprintf("%.4x", curr.Header.Parent),
				"next-hash", fmt.Sprintf("%.4x", curr.Header.Hash),
			)
			return fmt.Errorf("corrupt chain segment")
		}
	}
	return nil
}

func PruneTask(ctx context.Context, pg wpg.Conn, n int) error {
	const q = `
		delete from shovel.task_updates
		where (src_name, dest_name, num) not in (
			select src_name, dest_name, num
			from (
				select
					src_name,
					dest_name,
					num,
					row_number() over(partition by src_name, dest_name order by num desc) as rn
				from shovel.task_updates
			) as s
			where rn <= $1
		)
	`
	cmd, err := pg.Exec(ctx, q, n)
	if err != nil {
		return fmt.Errorf("deleting shovel.task_updates: %w", err)
	}
	slog.InfoContext(ctx, "prune-task", "n", cmd.RowsAffected())
	return nil
}

type jsonDuration time.Duration

func (d *jsonDuration) ScanInterval(i pgtype.Interval) error {
	*d = jsonDuration(i.Microseconds * 1000)
	return nil
}

func (d *jsonDuration) UnmarshalJSON(data []byte) error {
	if len(data) < 2 {
		return fmt.Errorf("jsonDuration must be at leaset 2 bytes")
	}
	data = data[1 : len(data)-1] // remove quotes
	dur, err := time.ParseDuration(string(data))
	*d = jsonDuration(dur)
	return err
}

func (d jsonDuration) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%s"`, d.String())), nil
}

func (d jsonDuration) String() string {
	switch td := time.Duration(d); {
	case td < 100*time.Millisecond:
		return td.Truncate(time.Millisecond).String()
	case td < time.Second:
		return td.Truncate(100 * time.Millisecond).String()
	default:
		return td.Truncate(time.Second).String()
	}
}

type TaskUpdate struct {
	DOMID    string       `db:"-"`
	SrcName  string       `db:"src_name"`
	DestName string       `db:"dest_name"`
	Num      uint64       `db:"num"`
	Stop     uint64       `db:"stop"`
	Hash     eth.Bytes    `db:"hash"`
	SrcNum   uint64       `db:"src_num"`
	SrcHash  eth.Bytes    `db:"src_hash"`
	NBlocks  uint64       `db:"nblocks"`
	NRows    uint64       `db:"nrows"`
	Latency  jsonDuration `db:"latency"`
}

func TaskUpdates(ctx context.Context, pg wpg.Conn) ([]TaskUpdate, error) {
	rows, _ := pg.Query(ctx, `
        with f as (
            select src_name, dest_name, max(num) num
            from shovel.task_updates group by 1, 2
        )
        select
			f.src_name,
			f.dest_name,
			f.num,
			coalesce(stop, 0) stop,
			hash,
			coalesce(src_num, 0) src_num,
			coalesce(src_hash, '\x00') src_hash,
			coalesce(nblocks, 0) nblocks,
			coalesce(nrows, 0) nrows,
			coalesce(latency, '0')::interval latency
        from f
        left join shovel.task_updates
		on shovel.task_updates.src_name = f.src_name
		and shovel.task_updates.dest_name= f.dest_name
		and shovel.task_updates.num = f.num;
    `)
	tus, err := pgx.CollectRows(rows, pgx.RowToStructByName[TaskUpdate])
	if err != nil {
		return nil, fmt.Errorf("querying for task updates: %w", err)
	}
	for i := range tus {
		tus[i].DOMID = fmt.Sprintf("%s-%s", tus[i].SrcName, tus[i].DestName)
	}
	return tus, nil
}

// Loads, Starts, and provides method for Restarting tasks
// based on config stored in the DB and in the config file.
type Manager struct {
	ctx     context.Context
	cache   *cache.Cache
	running sync.Mutex
	restart chan struct{}
	tasks   []*Task
	updates chan uint64
	pgp     *pgxpool.Pool
	conf    config.Root
}

func NewManager(ctx context.Context, pgp *pgxpool.Pool, conf config.Root) *Manager {
	return &Manager{
		ctx:     ctx,
		cache:   cache.New(100),
		restart: make(chan struct{}),
		updates: make(chan uint64),
		pgp:     pgp,
		conf:    conf,
	}
}

func (tm *Manager) Updates() uint64 {
	return <-tm.updates
}

func (tm *Manager) runTask(t *Task) {
	for {
		select {
		case <-tm.restart:
			slog.Info("restart-task", "chain", t.srcConfig.ChainID)
			return
		default:
			switch err := t.Converge(false); {
			case errors.Is(err, ErrDone):
				slog.InfoContext(t.ctx, "done")
				return
			case errors.Is(err, ErrNothingNew):
				time.Sleep(time.Second)
			case err != nil:
				time.Sleep(time.Second)
				slog.ErrorContext(t.ctx, "converge", "error", err, "dest_name", t.destConfig.Name)
			default:
				go func() {
					// try out best to deliver update
					// but don't stack up work
					select {
					case tm.updates <- t.srcConfig.ChainID:
					default:
					}
				}()
			}
		}
	}
}

// Ensures all running tasks stop
// and calls [Manager.Run] in a new go routine.
func (tm *Manager) Restart() error {
	close(tm.restart)
	ec := make(chan error)
	go tm.Run(ec)
	return <-ec
}

// Loads ethereum sources and integrations from both the config file
// and the database and assembles the nessecary tasks and runs all
// tasks in a loop.
//
// Acquires a lock to ensure only on routine is running.
// Releases lock on return
func (tm *Manager) Run(ec chan error) {
	tm.running.Lock()
	defer tm.running.Unlock()

	var err error
	tm.tasks, err = loadTasks(tm.ctx, tm.cache, tm.pgp, tm.conf)
	if err != nil {
		ec <- fmt.Errorf("loading tasks: %w", err)
		return
	}
	close(ec)

	tm.restart = make(chan struct{})
	var wg sync.WaitGroup
	for i := range tm.tasks {
		i := i
		wg.Add(1)
		go func() {
			tm.runTask(tm.tasks[i])
			wg.Done()
		}()
	}
	wg.Wait()
}

func loadTasks(ctx context.Context, cache *cache.Cache, pgp *pgxpool.Pool, c config.Root) ([]*Task, error) {
	allIntegrations, err := c.AllIntegrations(ctx, pgp)
	if err != nil {
		return nil, fmt.Errorf("loading integrations: %w", err)
	}
	scByName, err := c.AllSourcesByName(ctx, pgp)
	if err != nil {
		return nil, fmt.Errorf("loading source configs: %w", err)
	}
	var sources = map[string]Source{}
	for _, sc := range scByName {
		sources[sc.Name] = jrpc2.New(sc.URL)
	}
	var tasks []*Task
	for _, ig := range allIntegrations {
		if !ig.Enabled {
			continue
		}
		for _, scRef := range ig.Sources {
			sc, ok := scByName[scRef.Name]
			if !ok {
				return nil, fmt.Errorf("finding source config for %s", scRef.Name)
			}
			src, ok := sources[scRef.Name]
			if !ok {
				return nil, fmt.Errorf("finding source for %s", scRef.Name)
			}
			task, err := NewTask(
				WithContext(ctx),
				WithCache(cache),
				WithPG(pgp),
				WithRange(scRef.Start, scRef.Stop),
				WithConcurrency(sc.Concurrency, sc.BatchSize),
				WithSource(src),
				WithDestConfig(ig),
			)
			if err != nil {
				return nil, fmt.Errorf("setting up main task: %w", err)
			}
			tasks = append(tasks, task)
		}
	}
	return tasks, nil
}
