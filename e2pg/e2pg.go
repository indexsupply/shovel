package e2pg

import (
	"bytes"
	"cmp"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/indexsupply/x/abi2"
	"github.com/indexsupply/x/eth"
	"github.com/indexsupply/x/geth"
	"github.com/indexsupply/x/jrpc2"
	"github.com/indexsupply/x/rlps"
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
	LoadBlocks([][]byte, []geth.Buffer, []eth.Block) error
	Latest() (uint64, []byte, error)
	Hash(uint64) ([]byte, error)
}

type Destination interface {
	Name() string
	Insert(context.Context, wpg.Conn, []eth.Block) (int64, error)
	Delete(context.Context, wpg.Conn, uint64) error
	Events(context.Context) [][]byte
}

type Option func(t *Task)

func WithSource(chainID uint64, name string, src Source) Option {
	return func(t *Task) {
		if t.src != nil {
			panic("task can only have 1 src")
		}
		t.src = src
		t.srcName = name
		t.chainID = chainID
		t.ctx = wctx.WithChainID(t.ctx, chainID)
		t.ctx = wctx.WithSrcName(t.ctx, name)
	}
}

func WithBackfill(b bool) Option {
	return func(t *Task) {
		t.backfill = b
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

func WithConcurrency(workers, batch uint64) Option {
	return func(t *Task) {
		t.workers = workers
		t.batchSize = batch
		t.batch = make([]eth.Block, t.batchSize)
		t.buffs = make([]geth.Buffer, t.batchSize)
	}
}

func WithDestinations(dests ...Destination) Option {
	return func(t *Task) {
		var (
			filter [][]byte
			dstat  = make(map[string]Dstat)
		)
		for i := range dests {
			e := dests[i].Events(t.ctx)
			// if one integration has no filter
			// then the task must consider all data
			if len(e) == 0 {
				filter = filter[:0]
				break
			}
			filter = append(filter, e...)
			dstat[dests[i].Name()] = Dstat{}
		}
		t.dests = dests
		t.filter = filter
		t.dstat = dstat
		t.iub = newIUB(len(t.dests))
		t.destRanges = make([]destRange, len(t.dests))
	}
}

func NewTask(opts ...Option) *Task {
	t := &Task{
		ctx:       context.Background(),
		batch:     make([]eth.Block, 1),
		buffs:     make([]geth.Buffer, 1),
		batchSize: 1,
		workers:   1,
	}
	for _, opt := range opts {
		opt(t)
	}
	slog.InfoContext(t.ctx, "starting task", "dest-count", len(t.dests))
	return t
}

type Dstat struct {
	NRows   int64
	Latency jsonDuration
}

type Task struct {
	ctx      context.Context
	backfill bool

	src     Source
	srcName string
	chainID uint64

	pgp         *pgxpool.Pool
	dests       []Destination
	destRanges  []destRange
	start, stop uint64

	dstatMut sync.Mutex
	dstat    map[string]Dstat
	iub      *intgUpdateBuf

	filter    [][]byte
	batch     []eth.Block
	buffs     []geth.Buffer
	batchSize uint64
	workers   uint64
}

func (t *Task) dstatw(name string, n int64, d time.Duration) {
	t.dstatMut.Lock()
	defer t.dstatMut.Unlock()

	s := t.dstat[name]
	s.NRows = n
	s.Latency = jsonDuration(d)
	t.dstat[name] = s
}

func (t *Task) Setup() error {
	switch {
	case t.start > 0:
		h, err := t.src.Hash(t.start - 1)
		if err != nil {
			return err
		}
		if err := t.initRows(t.start-1, h); err != nil {
			return fmt.Errorf("init rows for user start: %w", err)
		}
	default:
		gethNum, _, err := t.src.Latest()
		if err != nil {
			return err
		}
		h, err := t.src.Hash(gethNum - 1)
		if err != nil {
			return fmt.Errorf("getting hash for %d: %w", gethNum-1, err)
		}
		if err := t.initRows(gethNum-1, h); err != nil {
			return fmt.Errorf("init rows for latest: %w", err)
		}
	}
	if !t.backfill {
		return nil
	}
	for i, d := range t.dests {
		err := t.destRanges[i].load(t.ctx, t.pgp, d.Name(), t.srcName)
		if err != nil {
			return fmt.Errorf("loading dest range for %s/%s: %w", d.Name(), t.srcName, err)
		}
	}
	return nil
}

// inserts an e2pg.task unless one with {src_name,backfill} already exists
// inserts a e2pg.intg for each t.dests[i] unless one with
// {name,src_name,backfill} already exists.
//
// There is no db transaction because this function can be called many
// times with varying degrees of success without overall problems.
func (t *Task) initRows(n uint64, h []byte) error {
	var exists bool
	const eq = `
		select true
		from e2pg.task
		where src_name = $1
		and backfill = $2
		limit 1
	`
	err := t.pgp.QueryRow(t.ctx, eq, t.srcName, t.backfill).Scan(&exists)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		const iq = `
			insert into e2pg.task(src_name, backfill, num, hash)
			values ($1, $2, $3, $4)
		`
		_, err := t.pgp.Exec(t.ctx, iq, t.srcName, t.backfill, n, h)
		if err != nil {
			return fmt.Errorf("inserting into task table: %w", err)
		}
	case err != nil:
		return fmt.Errorf("querying for existing task: %w", err)
	}
	for _, d := range t.dests {
		const eq = `
			select true
			from e2pg.intg
			where name = $1
			and src_name = $2
			and backfill = $3
			limit 1
		`
		err := t.pgp.QueryRow(t.ctx, eq, d.Name(), t.srcName, t.backfill).Scan(&exists)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			const iq = `
				insert into e2pg.intg(name, src_name, backfill, num)
				values ($1, $2, $3, $4)
			`
			_, err := t.pgp.Exec(t.ctx, iq, d.Name(), t.srcName, t.backfill, n)
			if err != nil {
				return fmt.Errorf("inserting into intg table: %w", err)
			}
		case err != nil:
			return fmt.Errorf("querying for existing intg: %w", err)
		}
	}
	return nil
}

var (
	ErrNothingNew = errors.New("no new blocks")
	ErrReorg      = errors.New("reorg")
	ErrDone       = errors.New("this is the end")
	ErrAhead      = errors.New("ahead")
)

// Indexes at most task.batchSize of the delta between min(g, limit) and pg.
// If pg contains an invalid latest block (ie reorg) then [ErrReorg]
// is returned and the caller may rollback the transaction resulting
// in no side-effects.
func (task *Task) Converge(notx bool) error {
	var (
		start             = time.Now()
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
		//crc32(task) == 1384045349
		const lockq = `select pg_advisory_xact_lock(1384045349, $1)`
		_, err = pg.Exec(task.ctx, lockq, task.chainID)
		if err != nil {
			return fmt.Errorf("task lock %d: %w", task.chainID, err)
		}
	}
	for reorgs := 0; reorgs <= 10; {
		localNum, localHash := uint64(0), []byte{}
		const q = `
			select num, hash
			from e2pg.task
			where src_name = $1
			and backfill = $2
			order by num desc
			limit 1
		`
		err := pg.QueryRow(task.ctx, q, task.srcName, task.backfill).Scan(&localNum, &localHash)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("getting latest from task: %w", err)
		}
		if task.stop > 0 && localNum >= task.stop { //don't sync past task.stop
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
			return ErrAhead
		}
		if localNum == gethNum {
			return ErrNothingNew
		}
		delta := min(gethNum-localNum, task.batchSize)
		if delta == 0 {
			return ErrNothingNew
		}
		for i := uint64(0); i < delta; i++ {
			task.batch[i].Reset()
			task.batch[i].SetNum(localNum + i + 1)
			task.buffs[i].Number = task.batch[i].Num()
		}
		switch nrows, err := task.loadinsert(localHash, pg, delta); {
		case errors.Is(err, ErrReorg):
			reorgs++
			slog.ErrorContext(task.ctx, "reorg", "n", localNum, "h", fmt.Sprintf("%.4x", localHash))
			const rq1 = `
				delete from e2pg.task
				where src_name = $1
				and backfill = $2
				and num >= $3
			`
			_, err := pg.Exec(task.ctx, rq1, task.srcName, task.backfill, localNum)
			if err != nil {
				return fmt.Errorf("deleting block from task table: %w", err)
			}
			const rq2 = `
				delete from e2pg.intg
				where src_name = $1
				and backfill = $2
				and num >= $3
			`
			_, err = pg.Exec(task.ctx, rq2, task.srcName, task.backfill, localNum)
			if err != nil {
				return fmt.Errorf("deleting block from task table: %w", err)
			}
			for _, dest := range task.dests {
				if err := dest.Delete(task.ctx, pg, localNum); err != nil {
					return fmt.Errorf("deleting block from integration: %w", err)
				}
			}
		case err != nil:
			err = errors.Join(rollback(), err)
			return err
		default:
			var last = task.batch[delta-1]
			const uq = `
				insert into e2pg.task (
					src_name,
					backfill,
					num,
					hash,
					src_num,
					src_hash,
					nblocks,
					nrows,
					latency,
					dstat
				)
				values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			`
			_, err := pg.Exec(task.ctx, uq,
				task.srcName,
				task.backfill,
				last.Num(),
				last.Hash(),
				gethNum,
				gethHash,
				delta,
				nrows,
				time.Since(start),
				task.dstat,
			)
			if err != nil {
				return fmt.Errorf("updating task table: %w", err)
			}
			if err := task.iub.write(task.ctx, pg); err != nil {
				return fmt.Errorf("updating integrations: %w", err)
			}
			if err := commit(); err != nil {
				return fmt.Errorf("commit converge tx: %w", err)
			}
			slog.InfoContext(task.ctx, "converge", "n", last.Num())
			return nil
		}
	}
	return errors.Join(ErrReorg, rollback())
}

// Fills in task.batch with block data (headers, bodies, receipts) from
// geth and then calls Index on the task's integrations with the block
// data.
//
// Once the block data has been read, it is checked against the parent
// hash to ensure consistency in the local chain. If the newly read data
// doesn't match [ErrReorg] is returned.
//
// The reading of block data and indexing of integrations happens concurrently
// with the number of go routines controlled by c.
func (task *Task) loadinsert(localHash []byte, pg wpg.Conn, delta uint64) (int64, error) {
	var (
		nrows int64
		eg1   = errgroup.Group{}
		wsize = task.batchSize / task.workers
	)
	for i := uint64(0); i < task.workers && i*wsize < delta; i++ {
		n := i * wsize
		m := n + wsize
		bfs := task.buffs[n:min(int(m), int(delta))]
		blks := task.batch[n:min(int(m), int(delta))]
		if len(blks) == 0 {
			continue
		}
		eg1.Go(func() error { return task.src.LoadBlocks(task.filter, bfs, blks) })
	}
	if err := eg1.Wait(); err != nil {
		return 0, err
	}
	if len(task.batch[0].Header.Parent) != 32 {
		return 0, fmt.Errorf("corrupt parent: %x\n", task.batch[0].Header.Parent)
	}
	if !bytes.Equal(localHash, task.batch[0].Header.Parent) {
		return 0, ErrReorg
	}
	var eg2 errgroup.Group
	for i := uint64(0); i < task.workers && i*wsize < delta; i++ {
		n := i * wsize
		m := n + wsize
		blks := task.batch[n:min(int(m), int(delta))]
		if len(blks) == 0 {
			continue
		}
		eg2.Go(func() error {
			var eg3 errgroup.Group
			for j := range task.dests {
				j := j
				eg3.Go(func() error {
					start := time.Now()
					blks = task.destRanges[j].filter(blks)
					count, err := task.dests[j].Insert(task.ctx, pg, blks)
					task.dstatw(task.dests[j].Name(), count, time.Since(start))
					task.iub.updates[j].Name = task.dests[j].Name()
					task.iub.updates[j].SrcName = task.srcName
					task.iub.updates[j].Backfill = task.backfill
					task.iub.updates[j].Num = blks[len(blks)-1].Num()
					task.iub.updates[j].NRows = count
					task.iub.updates[j].Latency = time.Since(start)
					nrows += count
					return err
				})
			}
			return eg3.Wait()
		})
	}
	return nrows, eg2.Wait()
}

type destRange struct{ start, stop uint64 }

func (r *destRange) load(ctx context.Context, pg wpg.Conn, name, srcName string) error {
	const startQuery = `
	   select num
	   from e2pg.intg
	   where name = $1
	   and src_name = $2
	   and backfill = true
	   order by num desc
	   limit 1
	`
	err := pg.QueryRow(ctx, startQuery, name, srcName).Scan(&r.start)
	if err != nil {
		return fmt.Errorf("start for %s/%s: %w", name, srcName, err)
	}
	const stopQuery = `
	   select num
	   from e2pg.intg
	   where name = $1
	   and src_name = $2
	   and backfill = false
	   order by num asc
	   limit 1
	`
	err = pg.QueryRow(ctx, stopQuery, name, srcName).Scan(&r.stop)
	if err != nil {
		return fmt.Errorf("stop for %s/%s: %w", name, srcName, err)
	}
	return nil
}

func (r *destRange) filter(blks []eth.Block) []eth.Block {
	switch {
	case r.stop == 0:
		return blks
	case len(blks) == 0:
		return blks
	case blks[0].Num() >= r.start && blks[len(blks)-1].Num() <= r.stop:
		return blks
	default:
		var n, m = 0, len(blks)
		for i := range blks {
			switch blks[i].Num() {
			case r.start:
				n = i
			case r.stop:
				m = i + 1
			}
		}
		return blks[n:m]
	}
}

type intgUpdate struct {
	Name     string        `db:"name"`
	SrcName  string        `db:"src_name"`
	Backfill bool          `db:"backfill"`
	Num      uint64        `db:"num"`
	Latency  time.Duration `db:"latency"`
	NRows    int64         `db:"nrows"`
}

func newIUB(n int) *intgUpdateBuf {
	iub := &intgUpdateBuf{}
	iub.updates = make([]intgUpdate, n)
	iub.table = pgx.Identifier{"e2pg", "intg"}
	iub.cols = []string{"name", "src_name", "backfill", "num", "latency", "nrows"}
	return iub
}

type intgUpdateBuf struct {
	i       int
	updates []intgUpdate
	out     [6]any
	table   pgx.Identifier
	cols    []string
}

func (b *intgUpdateBuf) Next() bool {
	return b.i < len(b.updates)
}

func (b *intgUpdateBuf) Err() error {
	return nil
}

func (b *intgUpdateBuf) Values() ([]any, error) {
	if b.i >= len(b.updates) {
		return nil, fmt.Errorf("no intg_update at idx %d len=%d", b.i, len(b.updates))
	}
	b.out[0] = b.updates[b.i].Name
	b.out[1] = b.updates[b.i].SrcName
	b.out[2] = b.updates[b.i].Backfill
	b.out[3] = b.updates[b.i].Num
	b.out[4] = b.updates[b.i].Latency
	b.out[5] = b.updates[b.i].NRows
	b.i++
	return b.out[:], nil
}

func (b *intgUpdateBuf) write(ctx context.Context, pg wpg.Conn) error {
	_, err := pg.CopyFrom(ctx, b.table, b.cols, b)
	b.i = 0 // reset
	return err
}

func PruneTask(ctx context.Context, pg wpg.Conn, n int) error {
	const q = `
		delete from e2pg.task
		where (src_name, backfill, num) not in (
			select src_name, backfill, num
			from (
				select
					src_name,
					backfill,
					num,
					row_number() over(partition by src_name, backfill order by num desc) as rn
				from e2pg.task
			) as s
			where rn <= $1
		)
	`
	cmd, err := pg.Exec(ctx, q, n)
	if err != nil {
		return fmt.Errorf("deleting e2pg.task: %w", err)
	}
	slog.InfoContext(ctx, "prune-task", "n", cmd.RowsAffected())
	return nil
}

func PruneIntg(ctx context.Context, pg wpg.Conn) error {
	const q = `
		delete from e2pg.intg
		where (name, src_name, backfill, num) not in (
			select name, src_name, backfill, max(num)
			from e2pg.intg
			group by name, src_name, backfill
			union
			select name, src_name, backfill, min(num)
			from e2pg.intg
			group by name, src_name, backfill
		)
	`
	cmd, err := pg.Exec(ctx, q)
	if err != nil {
		return fmt.Errorf("deleting e2pg.intg: %w", err)
	}
	slog.InfoContext(ctx, "prune-intg", "n", cmd.RowsAffected())
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
	return time.Duration(d).Round(time.Millisecond).String()
}

type TaskUpdate struct {
	SrcName  string           `db:"src_name"`
	Backfill bool             `db:"backfill"`
	Num      uint64           `db:"num"`
	Hash     eth.Bytes        `db:"hash"`
	SrcNum   uint64           `db:"src_num"`
	SrcHash  eth.Bytes        `db:"src_hash"`
	NBlocks  uint64           `db:"nblocks"`
	NRows    uint64           `db:"nrows"`
	Latency  jsonDuration     `db:"latency"`
	Dstat    map[string]Dstat `db:"dstat"`
}

func TaskUpdates(ctx context.Context, pg wpg.Conn) ([]TaskUpdate, error) {
	rows, _ := pg.Query(ctx, `
        with f as (
            select src_name, backfill, max(num) num
            from e2pg.task group by 1, 2
        )
        select
			f.src_name,
			f.backfill,
			f.num,
			hash,
			coalesce(src_num, 0) src_num,
			coalesce(src_hash, '\x00') src_hash,
			coalesce(nblocks, 0) nblocks,
			coalesce(nrows, 0) nrows,
			coalesce(latency, '0')::interval latency,
			coalesce(dstat, '{}') dstat
        from f
        left join e2pg.task
		on e2pg.task.src_name = f.src_name
		and e2pg.task.backfill = f.backfill
		and e2pg.task.num = f.num;
    `)
	return pgx.CollectRows(rows, pgx.RowToStructByName[TaskUpdate])
}

var compiled = map[string]Destination{}

// Loads, Starts, and provides method for Restarting tasks
// based on config stored in the DB and in the config file.
type Manager struct {
	running sync.Mutex
	restart chan struct{}
	tasks   []*Task
	updates chan uint64
	pgp     *pgxpool.Pool
	conf    Config
}

func NewManager(pgp *pgxpool.Pool, conf Config) *Manager {
	return &Manager{
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
			slog.Info("restart-task", "chain", t.chainID)
			return
		default:
			switch err := t.Converge(false); {
			case errors.Is(err, ErrDone):
				return
			case errors.Is(err, ErrNothingNew):
				time.Sleep(time.Second)
			case err != nil:
				time.Sleep(time.Second)
				slog.ErrorContext(t.ctx, "error", err)
			default:
				go func() {
					// try out best to deliver update
					// but don't stack up work
					select {
					case tm.updates <- t.chainID:
					default:
					}
				}()
			}
		}
	}
}

// Ensures all running tasks stop
// and calls [Manager.Run] in a new go routine.
func (tm *Manager) Restart() {
	close(tm.restart)
	go tm.Run()
}

func (tm *Manager) Load() (err error) {
	tm.tasks, err = loadTasks(context.Background(), tm.pgp, tm.conf)
	if err != nil {
		return fmt.Errorf("loading tasks: %w", err)
	}
	for i := range tm.tasks {
		if err = tm.tasks[i].Setup(); err != nil {
			return fmt.Errorf("setting up task: %w", err)
		}
	}
	return nil
}

// Loads ethereum sources and integrations from both the config file
// and the database and assembles the nessecary tasks and runs all
// tasks in a loop.
//
// Acquires a lock to ensure only on routine is running.
// Releases lock on return
func (tm *Manager) Run() {
	tm.running.Lock()
	defer tm.running.Unlock()
	tm.restart = make(chan struct{})
	var eg errgroup.Group
	for i := range tm.tasks {
		i := i
		eg.Go(func() error { tm.runTask(tm.tasks[i]); return nil })
	}
	eg.Wait()
}

func loadTasks(ctx context.Context, pgp *pgxpool.Pool, conf Config) ([]*Task, error) {
	allIntgs := map[string]Integration{}
	dbIntgs, err := Integrations(ctx, pgp)
	if err != nil {
		return nil, fmt.Errorf("loading integrations: %w", err)
	}
	for _, intg := range dbIntgs {
		allIntgs[intg.Name] = intg
	}
	for _, intg := range conf.Integrations {
		allIntgs[intg.Name] = intg
	}

	// Start per-source main tasks
	destBySourceName := map[string][]Destination{}
	for _, ig := range allIntgs {
		if !ig.Enabled {
			continue
		}
		dest, err := getDest(pgp, ig)
		if err != nil {
			return nil, fmt.Errorf("unable to build integration %s: %w", ig.Name, err)
		}
		for _, sc := range ig.SourceConfigs {
			destBySourceName[sc.Name] = append(destBySourceName[sc.Name], dest)
		}
	}
	allSources, err := conf.AllSourceConfigs(ctx, pgp)
	if err != nil {
		return nil, fmt.Errorf("loading source configs: %w", err)
	}

	var tasks []*Task
	for _, sc := range allSources {
		src, err := getSource(sc)
		if err != nil {
			return nil, fmt.Errorf("unkown source: %s", sc.Name)
		}
		dests := destBySourceName[sc.Name]
		tasks = append(tasks, NewTask(
			WithSource(sc.ChainID, sc.Name, src),
			WithPG(pgp),
			WithRange(sc.Start, sc.Stop),
			WithConcurrency(1, 512),
			WithDestinations(dests...),
		))
	}
	return tasks, nil
}

func getDest(pgp *pgxpool.Pool, ig Integration) (Destination, error) {
	switch {
	case len(ig.Compiled.Name) > 0:
		cig, ok := compiled[ig.Name]
		if !ok {
			return nil, fmt.Errorf("unable to find compiled integration: %s", ig.Name)
		}
		return cig, nil
	default:
		aig, err := abi2.New(ig.Name, ig.Event, ig.Block, ig.Table)
		if err != nil {
			return nil, fmt.Errorf("building abi integration: %w", err)
		}
		if err := abi2.CreateTable(context.Background(), pgp, aig.Table); err != nil {
			return nil, fmt.Errorf("setting up table for abi integration: %w", err)
		}
		return aig, nil
	}
}

func getSource(sc SourceConfig) (Source, error) {
	switch {
	case strings.Contains(sc.URL, "rlps"):
		return rlps.NewClient(sc.ChainID, sc.URL), nil
	case strings.HasPrefix(sc.URL, "http"):
		return jrpc2.New(sc.ChainID, sc.URL), nil
	default:
		// TODO add back support for local geth
		return nil, fmt.Errorf("unsupported src type: %v", sc)
	}
}

func Integrations(ctx context.Context, pg wpg.Conn) ([]Integration, error) {
	var res []Integration
	const q = `select conf from e2pg.integrations`
	rows, err := pg.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("querying integrations: %w", err)
	}
	for rows.Next() {
		var buf = []byte{}
		if err := rows.Scan(&buf); err != nil {
			return nil, fmt.Errorf("scanning integration: %w", err)
		}
		var intg Integration
		if err := json.Unmarshal(buf, &intg); err != nil {
			return nil, fmt.Errorf("unmarshaling integration: %w", err)
		}
		res = append(res, intg)
	}
	return res, nil
}

func SourceConfigs(ctx context.Context, pgp *pgxpool.Pool) ([]SourceConfig, error) {
	var res []SourceConfig
	const q = `select name, chain_id, url from e2pg.sources`
	rows, err := pgp.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("querying sources: %w", err)
	}
	for rows.Next() {
		var s SourceConfig
		if err := rows.Scan(&s.Name, &s.ChainID, &s.URL); err != nil {
			return nil, fmt.Errorf("scanning source: %w", err)
		}
		res = append(res, s)
	}
	return res, nil
}

type SourceConfig struct {
	Name    string `json:"name"`
	ChainID uint64 `json:"chain_id"`
	URL     string `json:"url"`
	Start   uint64 `json:"start"`
	Stop    uint64 `json:"stop"`
}

type Compiled struct {
	Name   string          `json:"name"`
	Config json.RawMessage `json:"config"`
}

type Integration struct {
	Name          string           `json:"name"`
	Enabled       bool             `json:"enabled"`
	SourceConfigs []SourceConfig   `json:"sources"`
	Table         abi2.Table       `json:"table"`
	Compiled      Compiled         `json:"compiled"`
	Block         []abi2.BlockData `json:"block"`
	Event         abi2.Event       `json:"event"`
}

type Config struct {
	PGURL         string         `json:"pg_url"`
	SourceConfigs []SourceConfig `json:"eth_sources"`
	Integrations  []Integration  `json:"integrations"`
}

func (conf Config) Empty() bool {
	return conf.PGURL == ""
}

func (conf Config) Valid(intg Integration) error {
	return nil
}

func (conf Config) AllIntegrations(ctx context.Context, pg wpg.Conn) ([]Integration, error) {
	res, err := Integrations(ctx, pg)
	if err != nil {
		return nil, fmt.Errorf("loading db integrations: %w", err)
	}
	for i := range conf.Integrations {
		res = append(res, conf.Integrations[i])
	}
	return res, nil
}

func (conf Config) IntegrationsBySource(ctx context.Context, pg wpg.Conn) (map[string][]Integration, error) {
	igs, err := conf.AllIntegrations(ctx, pg)
	if err != nil {
		return nil, fmt.Errorf("querying all integrations: %w", err)
	}
	res := make(map[string][]Integration)
	for _, ig := range igs {
		for _, sc := range ig.SourceConfigs {
			res[sc.Name] = append(res[sc.Name], ig)
		}
	}
	return res, nil
}

func (conf Config) AllSourceConfigs(ctx context.Context, pgp *pgxpool.Pool) ([]SourceConfig, error) {
	indb, err := SourceConfigs(ctx, pgp)
	if err != nil {
		return nil, fmt.Errorf("loading db integrations: %w", err)
	}

	var uniq = map[uint64]SourceConfig{}
	for _, sc := range indb {
		uniq[sc.ChainID] = sc
	}
	for _, sc := range conf.SourceConfigs {
		uniq[sc.ChainID] = sc
	}

	var res []SourceConfig
	for _, sc := range uniq {
		res = append(res, sc)
	}
	slices.SortFunc(res, func(a, b SourceConfig) int {
		return cmp.Compare(a.ChainID, b.ChainID)
	})
	return res, nil
}
