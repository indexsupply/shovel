package e2pg

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
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
	ChainID() uint64
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

func WithName(name string) Option {
	return func(t *Task) {
		t.Name = name
	}
}

func WithSource(s Source) Option {
	return func(t *Task) {
		if t.src != nil {
			panic("task can only have 1 src")
		}
		t.src = s
		t.ctx = wctx.WithChainID(t.ctx, s.ChainID())
		t.id = fmt.Sprintf("%d-main", t.src.ChainID())
		t.ctx = wctx.WithTaskID(t.ctx, t.id)
	}
}

func WithBackfillSource(s Source, name string) Option {
	return func(t *Task) {
		if t.src != nil {
			panic("task can only have 1 src")
		}
		t.backfill = true
		t.src = s
		t.ctx = wctx.WithChainID(t.ctx, s.ChainID())
		t.id = fmt.Sprintf("%d-backfill-%s", t.src.ChainID(), name)
		t.ctx = wctx.WithTaskID(t.ctx, t.id)
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
	Name     string
	ctx      context.Context
	id       string
	backfill bool

	src         Source
	pgp         *pgxpool.Pool
	dests       []Destination
	start, stop uint64

	dstatMut sync.Mutex
	dstat    map[string]Dstat

	filter    [][]byte
	batch     []eth.Block
	buffs     []geth.Buffer
	batchSize uint64
	workers   uint64
}

func (t *Task) dstatJSON() []byte {
	b, err := json.Marshal(t.dstat)
	if err != nil {
		slog.ErrorContext(t.ctx, "encoding dstat", err)
		return nil
	}
	return b
}

func (t *Task) dstatw(name string, n int64, d time.Duration) {
	t.dstatMut.Lock()
	defer t.dstatMut.Unlock()

	s := t.dstat[name]
	s.NRows = n
	s.Latency = jsonDuration(d)
	t.dstat[name] = s
}

func (task *Task) Insert(n uint64, h []byte) error {
	const q = `insert into e2pg.task (id, num, hash) values ($1, $2, $3)`
	_, err := task.pgp.Exec(context.Background(), q, task.id, n, h)
	return err
}

func (task *Task) Latest() (uint64, []byte, error) {
	const q = `SELECT num, hash FROM e2pg.task WHERE id = $1 ORDER BY num DESC LIMIT 1`
	var n, h = uint64(0), []byte{}
	err := task.pgp.QueryRow(context.Background(), q, task.id).Scan(&n, &h)
	if errors.Is(err, pgx.ErrNoRows) {
		return n, nil, nil
	}
	return n, h, err
}

func (task *Task) Setup() error {
	_, localHash, err := task.Latest()
	if err != nil {
		return err
	}
	if len(localHash) > 0 {
		// already setup
		return nil
	}
	if task.start > 0 {
		h, err := task.src.Hash(task.start - 1)
		if err != nil {
			return err
		}
		return task.Insert(task.start-1, h)
	}
	gethNum, _, err := task.src.Latest()
	if err != nil {
		return err
	}
	h, err := task.src.Hash(gethNum - 1)
	if err != nil {
		return fmt.Errorf("getting hash for %d: %w", gethNum-1, err)
	}
	return task.Insert(gethNum-1, h)
}

func (task *Task) Run1(updates chan<- string, notx bool) {
	switch err := task.Converge(notx); {
	case errors.Is(err, ErrDone):
		return
	case errors.Is(err, ErrNothingNew):
		time.Sleep(time.Second)
	case err != nil:
		time.Sleep(time.Second)
		slog.ErrorContext(task.ctx, "error", err)
	default:
		go func() {
			// try out best to deliver update
			// but don't stack up work
			select {
			case updates <- task.id:
			default:
			}
		}()
	}
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
		_, err = pg.Exec(task.ctx, lockq, wctx.ChainID(task.ctx))
		if err != nil {
			return fmt.Errorf("task lock %s: %w", task.id, err)
		}
	}
	for reorgs := 0; reorgs <= 10; {
		localNum, localHash := uint64(0), []byte{}
		const q = `SELECT num, hash FROM e2pg.task WHERE id = $1 ORDER BY num DESC LIMIT 1`
		err := pg.QueryRow(task.ctx, q, task.id).Scan(&localNum, &localHash)
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
			const dq = "delete from e2pg.task where id = $1 AND num >= $2"
			_, err := pg.Exec(task.ctx, dq, task.id, localNum)
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
					id,
					num,
					hash,
					src_num,
					src_hash,
					nblocks,
					nrows,
					latency,
					dstat
				)
				values ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			`
			_, err := pg.Exec(task.ctx, uq,
				task.id,
				last.Num(),
				last.Hash(),
				gethNum,
				gethHash,
				delta,
				nrows,
				time.Since(start),
				task.dstatJSON(),
			)
			if err != nil {
				return fmt.Errorf("updating task table: %w", err)
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
					t0 := time.Now()
					count, err := task.dests[j].Insert(task.ctx, pg, blks)
					task.dstatw(task.dests[j].Name(), count, time.Since(t0))
					nrows += count
					return err
				})
			}
			return eg3.Wait()
		})
	}
	return nrows, eg2.Wait()
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
	ID      string           `db:"id"`
	Num     uint64           `db:"num"`
	Hash    eth.Bytes        `db:"hash"`
	SrcNum  uint64           `db:"src_num"`
	SrcHash eth.Bytes        `db:"src_hash"`
	NBlocks uint64           `db:"nblocks"`
	NRows   uint64           `db:"nrows"`
	Latency jsonDuration     `db:"latency"`
	Dstat   map[string]Dstat `db:"dstat"`
}

func TaskUpdate1(ctx context.Context, pg wpg.Conn, id string) (TaskUpdate, error) {
	const q = `
		select
			id,
			num,
			hash,
			coalesce(src_num, 0) src_num,
			coalesce(src_hash, '\x00') src_hash,
			coalesce(nblocks, 0) nblocks,
			coalesce(nrows, 0) nrows,
			coalesce(latency, '0')::interval latency,
			coalesce(dstat, '{}') dstat
		from e2pg.task
		where id = $1
		order by num desc
		limit 1;
	`
	row, _ := pg.Query(ctx, q, id)
	return pgx.CollectOneRow(row, pgx.RowToStructByName[TaskUpdate])
}

func TaskUpdates(ctx context.Context, pg wpg.Conn) ([]TaskUpdate, error) {
	rows, _ := pg.Query(ctx, `
		select distinct on (id)
			id,
			num,
			hash,
			coalesce(src_num, 0) src_num,
			coalesce(src_hash, '\x00') src_hash,
			coalesce(nblocks, 0) nblocks,
			coalesce(nrows, 0) nrows,
			coalesce(latency, '0')::interval latency,
			coalesce(dstat, '{}') dstat
		from e2pg.task
		order by id, num desc;
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
	updates chan string
	pgp     *pgxpool.Pool
	conf    Config
}

func NewManager(pgp *pgxpool.Pool, conf Config) *Manager {
	return &Manager{
		restart: make(chan struct{}),
		updates: make(chan string),
		pgp:     pgp,
		conf:    conf,
	}
}

func (tm *Manager) Updates() string {
	return <-tm.updates
}

func (tm *Manager) runTask(t *Task) error {
	if err := t.Setup(); err != nil {
		return fmt.Errorf("setting up task: %w", err)
	}
	for {
		select {
		case <-tm.restart:
			slog.Info("restart-task", "name", t.Name)
			return nil
		default:
			t.Run1(tm.updates, false)
		}
	}
}

// Ensures all running tasks stop
// and calls [Manager.Run] in a new go routine.
func (tm *Manager) Restart() {
	close(tm.restart)
	go tm.Run()
}

// Loads ethereum sources and integrations from both the config file
// and the database and assembles the nessecary tasks and runs all
// tasks in a loop.
//
// Acquires a lock to ensure only on routine is running.
// Releases lock on return
func (tm *Manager) Run() error {
	tm.running.Lock()
	defer tm.running.Unlock()
	tm.restart = make(chan struct{})
	tasks, err := loadTasks(context.Background(), tm.pgp, tm.conf)
	if err != nil {
		return fmt.Errorf("loading tasks: %w", err)
	}
	var eg errgroup.Group
	for i := range tasks {
		i := i
		eg.Go(func() error {
			return tm.runTask(tasks[i])
		})
	}
	return eg.Wait()
}

func loadTasks(ctx context.Context, pgp *pgxpool.Pool, conf Config) ([]*Task, error) {
	allSources := map[string]SourceConfig{}
	dbSources, err := SourceConfigs(ctx, pgp)
	if err != nil {
		return nil, fmt.Errorf("loading sources: %w", err)
	}
	for _, sc := range dbSources {
		allSources[sc.Name] = sc
	}
	for _, sc := range conf.SourceConfigs {
		allSources[sc.Name] = sc
	}

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
	var tasks []*Task
	for _, sc := range allSources {
		src, err := getSource(sc)
		if err != nil {
			return nil, fmt.Errorf("unkown source: %s", sc.Name)
		}
		dests := destBySourceName[sc.Name]
		tasks = append(tasks, NewTask(
			WithName(sc.Name),
			WithSource(src),
			WithPG(pgp),
			WithRange(sc.Start, sc.Stop),
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
	Start         string           `json:"start"`
	Stop          string           `json:"stop"`
	Backfill      bool             `json:"backfill"`
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
	res, err := SourceConfigs(ctx, pgp)
	if err != nil {
		return nil, fmt.Errorf("loading db integrations: %w", err)
	}
	for i := range conf.SourceConfigs {
		res = append(res, conf.SourceConfigs[i])
	}
	return res, nil
}
