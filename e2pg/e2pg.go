package e2pg

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/indexsupply/x/bint"
	"github.com/indexsupply/x/bloom"
	"github.com/indexsupply/x/eth"
	"github.com/indexsupply/x/freezer"
	"github.com/indexsupply/x/geth"
	"github.com/indexsupply/x/jrpc"
	"github.com/indexsupply/x/rlp"
	"github.com/indexsupply/x/txlocker"

	"github.com/bmizerany/perks/quantile"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"
	"golang.org/x/text/message"
)

type ctxkey int

const (
	taskIDKey  ctxkey = 1
	chainIDKey ctxkey = 2
)

func WithChainID(ctx context.Context, id uint64) context.Context {
	return context.WithValue(ctx, chainIDKey, id)
}

func ChainID(ctx context.Context) uint64 {
	id, _ := ctx.Value(chainIDKey).(uint64)
	return id
}
func WithTaskID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, taskIDKey, id)
}

func TaskID(ctx context.Context) string {
	id, _ := ctx.Value(taskIDKey).(string)
	return id
}

//go:embed schema.sql
var Schema string

type Node interface {
	ChainID() uint64
	LoadBlocks([][]byte, []geth.Buffer, []eth.Block) error
	Latest() (uint64, []byte, error)
	Hash(uint64) ([]byte, error)
}

type PG interface {
	CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error)
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type Integration interface {
	Insert(context.Context, PG, []eth.Block) (int64, error)
	Delete(context.Context, PG, uint64) error
	Events(context.Context) [][]byte
}

type Option func(t *Task)

func WithName(name string) Option {
	return func(t *Task) {
		t.Name = name
	}
}

func WithNode(n Node) Option {
	return func(t *Task) {
		if t.node != nil {
			panic("task can only have 1 node")
		}
		t.node = n
		t.ctx = WithChainID(t.ctx, n.ChainID())
		t.id = fmt.Sprintf("%d-main", t.node.ChainID())
		t.ctx = WithTaskID(t.ctx, t.id)
	}
}

func WithBackfillNode(n Node, name string) Option {
	return func(t *Task) {
		if t.node != nil {
			panic("task can only have 1 node")
		}
		t.backfill = true
		t.node = n
		t.ctx = WithChainID(t.ctx, n.ChainID())
		t.id = fmt.Sprintf("%d-backfill-%s", t.node.ChainID(), name)
		t.ctx = WithTaskID(t.ctx, t.id)
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

func WithIntegrations(intgs ...Integration) Option {
	return func(t *Task) {
		var filter [][]byte
		for i := range intgs {
			e := intgs[i].Events(t.ctx)
			// if one integration has no filter
			// then the task must consider all data
			if len(e) == 0 {
				filter = filter[:0]
				break
			}
			filter = append(filter, e...)
		}
		t.intgs = intgs
		t.filter = filter
	}
}

func NewTask(opts ...Option) *Task {
	t := &Task{
		ctx:       context.Background(),
		batch:     make([]eth.Block, 1),
		buffs:     make([]geth.Buffer, 1),
		batchSize: 1,
		workers:   1,
		stat: status{
			tlat:  quantile.NewTargeted(0.50, 0.90, 0.99),
			glat:  quantile.NewTargeted(0.50, 0.90, 0.99),
			pglat: quantile.NewTargeted(0.50, 0.90, 0.99),
		},
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

type Task struct {
	Name     string
	ctx      context.Context
	id       string
	backfill bool

	node        Node
	pgp         *pgxpool.Pool
	intgs       []Integration
	start, stop uint64

	filter    [][]byte
	batch     []eth.Block
	buffs     []geth.Buffer
	batchSize uint64
	workers   uint64
	stat      status
}

type status struct {
	ehash, ihash      []byte
	enum, inum        uint64
	tlat, glat, pglat *quantile.Stream

	reset          time.Time
	err            error
	blocks, events int64
}

type StatusSnapshot struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	ChainID         uint64 `json:"chainID"`
	EthHash         string `json:"eth_hash"`
	EthNum          string `json:"eth_num"`
	Hash            string `json:"hash"`
	Num             string `json:"num"`
	EventCount      string `json:"event_count"`
	BlockCount      string `json:"block_count"`
	TotalLatencyP50 string `json:"total_latency_p50"`
	TotalLatencyP95 string `json:"total_latency_p95"`
	TotalLatencyP99 string `json:"total_latency_p99"`
	Error           string `json:"error"`
}

func (task *Task) Status() StatusSnapshot {
	printer := message.NewPrinter(message.MatchLanguage("en"))
	snap := StatusSnapshot{}
	snap.ID = task.id
	snap.Name = task.Name
	snap.ChainID = ChainID(task.ctx)
	snap.EthHash = fmt.Sprintf("%.4x", task.stat.ehash)
	snap.EthNum = fmt.Sprintf("%d", task.stat.enum)
	snap.Hash = fmt.Sprintf("%.4x", task.stat.ihash)
	snap.Num = printer.Sprintf("%d", task.stat.inum)
	snap.BlockCount = fmt.Sprintf("%d", atomic.SwapInt64(&task.stat.blocks, 0))
	snap.EventCount = fmt.Sprintf("%d", atomic.SwapInt64(&task.stat.events, 0))
	snap.TotalLatencyP50 = fmt.Sprintf("%s", time.Duration(task.stat.tlat.Query(0.50)).Round(time.Millisecond))
	snap.TotalLatencyP95 = fmt.Sprintf("%.2f", task.stat.tlat.Query(0.95))
	snap.TotalLatencyP99 = fmt.Sprintf("%.2f", task.stat.tlat.Query(0.99))
	snap.Error = fmt.Sprintf("%v", task.stat.err)
	task.stat.tlat.Reset()
	return snap
}

func (task *Task) Insert(n uint64, h []byte) error {
	const q = `insert into e2pg.task (id, number, hash) values ($1, $2, $3)`
	_, err := task.pgp.Exec(context.Background(), q, task.id, n, h)
	return err
}

func (task *Task) Latest() (uint64, []byte, error) {
	const q = `SELECT number, hash FROM e2pg.task WHERE id = $1 ORDER BY number DESC LIMIT 1`
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
		h, err := task.node.Hash(task.start - 1)
		if err != nil {
			return err
		}
		return task.Insert(task.start-1, h)
	}
	gethNum, _, err := task.node.Latest()
	if err != nil {
		return err
	}
	h, err := task.node.Hash(gethNum - 1)
	if err != nil {
		return fmt.Errorf("getting hash for %d: %w", gethNum-1, err)
	}
	return task.Insert(gethNum-1, h)
}

func (task *Task) Run(snaps chan<- StatusSnapshot, notx bool) {
	for {
		switch err := task.Converge(notx); {
		case err == nil:
			go func() {
				snap := task.Status()
				slog.InfoContext(task.ctx, "", "n", snap.Num, "h", snap.Hash)
				select {
				case snaps <- snap:
				default:
				}
			}()
		case errors.Is(err, ErrDone):
			return
		case errors.Is(err, ErrNothingNew):
			time.Sleep(time.Second)
		default:
			time.Sleep(time.Second)
			slog.ErrorContext(task.ctx, "error", err)
		}
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
		start       = time.Now()
		pg       PG = task.pgp
		commit      = func() error { return nil }
		rollback    = func() error { return nil }
	)
	if !notx {
		pgTx, err := task.pgp.Begin(task.ctx)
		if err != nil {
			return err
		}
		commit = func() error { return pgTx.Commit(task.ctx) }
		rollback = func() error { return pgTx.Rollback(task.ctx) }
		defer rollback()
		pg = txlocker.NewTx(pgTx)
		//crc32(task) == 1384045349
		const lockq = `select pg_advisory_xact_lock(1384045349, $1)`
		_, err = pg.Exec(task.ctx, lockq, ChainID(task.ctx))
		if err != nil {
			return fmt.Errorf("task lock %s: %w", task.id, err)
		}
	}
	for reorgs := 0; reorgs <= 10; {
		localNum, localHash := uint64(0), []byte{}
		const q = `SELECT number, hash FROM e2pg.task WHERE id = $1 ORDER BY number DESC LIMIT 1`
		err := pg.QueryRow(task.ctx, q, task.id).Scan(&localNum, &localHash)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("getting latest from task: %w", err)
		}
		if task.stop > 0 && localNum >= task.stop { //don't sync past task.stop
			return ErrDone
		}
		gethNum, gethHash, err := task.node.Latest()
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
		switch err := task.writeIndex(localHash, pg, delta); {
		case errors.Is(err, ErrReorg):
			reorgs++
			slog.ErrorContext(task.ctx, "reorg", "n", localNum, "h", fmt.Sprintf("%.4x", localHash))
			const dq = "delete from e2pg.task where id = $1 AND number >= $2"
			_, err := pg.Exec(task.ctx, dq, task.id, localNum)
			if err != nil {
				return fmt.Errorf("deleting block from task table: %w", err)
			}
			for _, ig := range task.intgs {
				if err := ig.Delete(task.ctx, pg, localNum); err != nil {
					return fmt.Errorf("deleting block from integration: %w", err)
				}
			}
		case err != nil:
			err = errors.Join(rollback(), err)
			task.stat.err = err
			return err
		default:
			task.stat.enum = gethNum
			task.stat.ehash = gethHash
			task.stat.blocks += int64(delta)
			task.stat.tlat.Insert(float64(time.Since(start)))
			return commit()
		}
	}
	return errors.Join(ErrReorg, rollback())
}

func (task *Task) skip(bf bloom.Filter) bool {
	if len(task.filter) == 0 {
		return false
	}
	for _, sig := range task.filter {
		if !bf.Missing(sig) {
			return false
		}
	}
	return true
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
func (task *Task) writeIndex(localHash []byte, pg PG, delta uint64) error {
	var (
		eg    = errgroup.Group{}
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
		eg.Go(func() error { return task.node.LoadBlocks(task.filter, bfs, blks) })
	}
	if err := eg.Wait(); err != nil {
		return err
	}
	if len(task.batch[0].Header.Parent) != 32 {
		return fmt.Errorf("corrupt parent: %x\n", task.batch[0].Header.Parent)
	}
	if !bytes.Equal(localHash, task.batch[0].Header.Parent) {
		return ErrReorg
	}
	eg = errgroup.Group{}
	for i := uint64(0); i < task.workers && i*wsize < delta; i++ {
		n := i * wsize
		m := n + wsize
		blks := task.batch[n:min(int(m), int(delta))]
		if len(blks) == 0 {
			continue
		}
		eg.Go(func() error {
			var igeg errgroup.Group
			for _, ig := range task.intgs {
				ig := ig
				igeg.Go(func() error {
					count, err := ig.Insert(task.ctx, pg, blks)
					atomic.AddInt64(&task.stat.events, count)
					return err
				})
			}
			return igeg.Wait()
		})
	}
	if err := eg.Wait(); err != nil {
		return fmt.Errorf("writing indexed data: %w", err)
	}
	var last = task.batch[delta-1]
	const uq = "insert into e2pg.task (id, number, hash) values ($1, $2, $3)"
	_, err := pg.Exec(context.Background(), uq, task.id, last.Num(), last.Hash())
	if err != nil {
		return fmt.Errorf("updating task table: %w", err)
	}
	task.stat.inum = last.Num()
	task.stat.ihash = last.Hash()
	return nil
}

func NewGeth(fc freezer.FileCache, rc *jrpc.Client) *Geth {
	return &Geth{fc: fc, rc: rc}
}

type Geth struct {
	fc freezer.FileCache
	rc *jrpc.Client
}

func (g *Geth) ChainID() uint64 { return 0 }

func (g *Geth) Hash(num uint64) ([]byte, error) {
	return geth.Hash(num, g.fc, g.rc)
}

func (g *Geth) Latest() (uint64, []byte, error) {
	n, h, err := geth.Latest(g.rc)
	if err != nil {
		return 0, nil, fmt.Errorf("getting last block hash: %w", err)
	}
	return bint.Uint64(n), h, nil
}

func skip(filter [][]byte, bf bloom.Filter) bool {
	if len(filter) == 0 {
		return false
	}
	for i := range filter {
		if !bf.Missing(filter[i]) {
			return false
		}
	}
	return true
}

func (g *Geth) LoadBlocks(filter [][]byte, bfs []geth.Buffer, blks []eth.Block) error {
	err := geth.Load(filter, bfs, g.fc, g.rc)
	if err != nil {
		return fmt.Errorf("loading data: %w", err)
	}
	for i := range blks {
		blks[i].Header.UnmarshalRLP(bfs[i].Header())
		if skip(filter, bloom.Filter(blks[i].Header.LogsBloom)) {
			continue
		}
		//rlp contains: [transactions,uncles]
		blks[i].Txs.UnmarshalRLP(rlp.Bytes(bfs[i].Bodies()))
		blks[i].Receipts.UnmarshalRLP(bfs[i].Receipts())
	}
	return validate(blks)
}

func validate(blocks []eth.Block) error {
	if len(blocks) <= 1 {
		return nil
	}
	for i := 1; i < len(blocks); i++ {
		prev, curr := blocks[i-1], blocks[i]
		if !bytes.Equal(curr.Header.Parent, prev.Hash()) {
			return fmt.Errorf("invalid batch. prev=%s curr=%s", prev, curr)
		}
	}
	return nil
}
