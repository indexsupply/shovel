package e2pg

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/indexsupply/x/bint"
	"github.com/indexsupply/x/bloom"
	"github.com/indexsupply/x/freezer"
	"github.com/indexsupply/x/geth"
	"github.com/indexsupply/x/isxhash"
	"github.com/indexsupply/x/jrpc"
	"github.com/indexsupply/x/rlp"
	"github.com/indexsupply/x/txlocker"

	"github.com/bmizerany/perks/quantile"
	"github.com/holiman/uint256"
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
func WithTaskID(ctx context.Context, id uint64) context.Context {
	return context.WithValue(ctx, taskIDKey, id)
}

func TaskID(ctx context.Context) uint64 {
	id, _ := ctx.Value(taskIDKey).(uint64)
	return id
}

//go:embed schema.sql
var Schema string

type Node interface {
	LoadBlocks([][]byte, []geth.Buffer, []Block) error
	Latest() (uint64, []byte, error)
	Hash(uint64) ([]byte, error)
}

type PG interface {
	CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error)
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type Integration interface {
	Insert(context.Context, PG, []Block) (int64, error)
	Delete(context.Context, PG, uint64) error
	Events(context.Context) [][]byte
}

func NewTask(
	id uint64,
	chainID uint64,
	name string,
	batchSize uint64,
	workers uint64,
	node Node,
	pgp *pgxpool.Pool,
	begin, end uint64,
	intgs ...Integration,
) *Task {
	ctx := context.Background()
	ctx = WithChainID(ctx, chainID)
	ctx = WithTaskID(ctx, id)

	var filter [][]byte
	for i := range intgs {
		e := intgs[i].Events(ctx)
		// if one integration has no filter
		// then the task must consider all data
		if len(e) == 0 {
			filter = filter[:0]
			break
		}
		filter = append(filter, e...)
	}
	return &Task{
		ctx:       ctx,
		ID:        id,
		Name:      name,
		ChainID:   chainID,
		batch:     make([]Block, batchSize),
		buffs:     make([]geth.Buffer, batchSize),
		batchSize: batchSize,
		workers:   workers,
		node:      node,
		pgp:       pgp,
		intgs:     intgs,
		filter:    filter,
		begin:     begin,
		end:       end,
		stat: status{
			tlat:  quantile.NewTargeted(0.50, 0.90, 0.99),
			glat:  quantile.NewTargeted(0.50, 0.90, 0.99),
			pglat: quantile.NewTargeted(0.50, 0.90, 0.99),
		},
	}
}

type Task struct {
	ctx     context.Context
	ID      uint64
	Name    string
	ChainID uint64

	node       Node
	pgp        *pgxpool.Pool
	intgs      []Integration
	begin, end uint64

	filter    [][]byte
	batch     []Block
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
	Name            string `json:"name"`
	ChainID         string `json:"chainID"`
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
	snap.Name = task.Name
	snap.ChainID = fmt.Sprintf("%d", task.ChainID)
	snap.EthHash = fmt.Sprintf("%.4x", task.stat.ehash)
	snap.EthNum = fmt.Sprintf("%d", task.stat.enum)
	snap.Hash = fmt.Sprintf("%.4x", task.stat.ihash)
	snap.Num = printer.Sprintf("%.9d", task.stat.inum)
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
	const q = `insert into task (id, number, hash) values ($1, $2, $3)`
	_, err := task.pgp.Exec(context.Background(), q, task.ID, n, h)
	return err
}

func (task *Task) Latest() (uint64, []byte, error) {
	const q = `SELECT number, hash FROM task WHERE id = $1 ORDER BY number DESC LIMIT 1`
	var n, h = uint64(0), []byte{}
	err := task.pgp.QueryRow(context.Background(), q, task.ID).Scan(&n, &h)
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
	if task.begin > 0 {
		h, err := task.node.Hash(task.begin - 1)
		if err != nil {
			return err
		}
		return task.Insert(task.begin-1, h)
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
		_, err = pg.Exec(task.ctx, lockq, task.ID)
		if err != nil {
			return fmt.Errorf("task lock %d: %w", task.ID, err)
		}
	}
	for reorgs := 0; reorgs <= 10; {
		localNum, localHash := uint64(0), []byte{}
		const q = `SELECT number, hash FROM task WHERE id = $1 ORDER BY number DESC LIMIT 1`
		err := pg.QueryRow(task.ctx, q, task.ID).Scan(&localNum, &localHash)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("getting latest from task: %w", err)
		}
		if task.end > 0 && localNum >= task.end { //don't sync past task.end
			return ErrDone
		}
		gethNum, gethHash, err := task.node.Latest()
		if err != nil {
			return fmt.Errorf("getting latest from eth: %w", err)
		}
		if task.end > 0 && gethNum > task.end {
			gethNum = task.end
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
			task.buffs[i].Number = localNum + i + 1
			task.batch[i].Transactions.Reset()
			task.batch[i].Receipts.Reset()
		}
		switch err := task.writeIndex(localHash, pg, delta); {
		case errors.Is(err, ErrReorg):
			reorgs++
			slog.ErrorContext(task.ctx, "reorg", "n", localNum, "h", fmt.Sprintf("%.4x", localHash))
			const dq = "delete from task where id = $1 AND number >= $2"
			_, err := pg.Exec(task.ctx, dq, task.ID, localNum)
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
	const uq = "insert into task (id, number, hash) values ($1, $2, $3)"
	_, err := pg.Exec(context.Background(), uq, task.ID, last.Num(), last.Hash())
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

func Skip(filter [][]byte, bf bloom.Filter) bool {
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

func (g *Geth) LoadBlocks(filter [][]byte, bfs []geth.Buffer, blks []Block) error {
	err := geth.Load(filter, bfs, g.fc, g.rc)
	if err != nil {
		return fmt.Errorf("loading data: %w", err)
	}
	for i := range blks {
		blks[i].Header.Unmarshal(bfs[i].Header())
		if Skip(filter, bloom.Filter(blks[i].Header.LogsBloom)) {
			continue
		}
		//rlp contains: [transactions,uncles]
		txs := rlp.Bytes(bfs[i].Bodies())
		for j, it := 0, rlp.Iter(txs); it.HasNext(); j++ {
			blks[i].Transactions.Insert(j, it.Bytes())
		}
		for j, it := 0, rlp.Iter(bfs[i].Receipts()); it.HasNext(); j++ {
			blks[i].Receipts.Insert(j, it.Bytes())
		}
	}
	return validate(blks)
}

func validate(blocks []Block) error {
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

type Block struct {
	Header       Header
	Transactions Transactions
	Receipts     Receipts
	Uncles       []Header
}

func (b Block) Num() uint64  { return b.Header.Number }
func (b Block) Hash() []byte { return b.Header.Hash }

func (b Block) String() string {
	return fmt.Sprintf("{%d %.4x %.4x}",
		b.Header.Number,
		b.Header.Parent,
		b.Header.Hash,
	)
}

type Header struct {
	Hash        []byte
	Parent      []byte
	Uncle       []byte
	Coinbase    []byte
	StateRoot   []byte
	TxRoot      []byte
	ReceiptRoot []byte
	LogsBloom   []byte
	Difficulty  []byte
	Number      uint64
	GasLimit    uint64
	GasUsed     uint64
	Time        uint64
	Extra       []byte
	MixHash     []byte
	Nonce       uint64
	BaseFee     []byte
	Withdraw    []byte
	ExcessData  []byte
}

func grow(s []byte, n int) []byte {
	if len(s) < n {
		s = append(s, make([]byte, n-len(s))...)
	}
	return s[:n]
}

func growcopy(dst, src []byte) []byte {
	dst = grow(dst, len(src))
	copy(dst, src)
	return dst
}

func (h *Header) Unmarshal(input []byte) {
	h.Hash = isxhash.Keccak(input)
	for i, itr := 0, rlp.Iter(input); itr.HasNext(); i++ {
		d := itr.Bytes()
		switch i {
		case 0:
			h.Parent = growcopy(h.Parent, d)
		case 6:
			h.LogsBloom = growcopy(h.LogsBloom, d)
		case 8:
			h.Number = bint.Uint64(d)
		case 11:
			h.Time = bint.Uint64(d)
		}
	}
}

type Receipt struct {
	Status  []byte
	GasUsed uint64
	Logs    Logs
}

func (r *Receipt) Unmarshal(input []byte) {
	iter := rlp.Iter(input)
	r.Status = growcopy(r.Status, iter.Bytes())
	r.GasUsed = bint.Uint64(iter.Bytes())
	r.Logs.Reset()
	for i, l := 0, rlp.Iter(iter.Bytes()); l.HasNext(); i++ {
		r.Logs.Insert(i, l.Bytes())
	}
}

type Receipts struct {
	d []Receipt
	n int
}

func (rs *Receipts) Reset()            { rs.n = 0 }
func (rs *Receipts) Len() int          { return rs.n }
func (rs *Receipts) At(i int) *Receipt { return &rs.d[i] }

func (rs *Receipts) Insert(i int, b []byte) {
	rs.n++
	switch {
	case i < len(rs.d):
		rs.d[i].Unmarshal(b)
	case len(rs.d) < cap(rs.d):
		r := Receipt{}
		r.Unmarshal(b)
		rs.d = append(rs.d, r)
	default:
		rs.d = append(rs.d, make([]Receipt, 512)...)
		r := Receipt{}
		r.Unmarshal(b)
		rs.d[i] = r
	}
}

type Topics struct {
	d [4][]byte
	n int
}

func (ts *Topics) Reset()          { ts.n = 0 }
func (ts *Topics) Len() int        { return ts.n }
func (ts *Topics) At(i int) []byte { return ts.d[i] }

func (ts *Topics) Insert(i int, b []byte) {
	ts.n++
	ts.d[i] = growcopy(ts.d[i], b)
}

type Log struct {
	Address []byte
	Topics  Topics
	Data    []byte
}

func (l *Log) Unmarshal(input []byte) {
	iter := rlp.Iter(input)
	l.Address = growcopy(l.Address, iter.Bytes())
	l.Topics.Reset()
	for i, t := 0, rlp.Iter(iter.Bytes()); t.HasNext(); i++ {
		l.Topics.Insert(i, t.Bytes())
	}
	l.Data = growcopy(l.Data, iter.Bytes())
}

type Logs struct {
	d []Log
	n int
}

func (ls *Logs) Reset()        { ls.n = 0 }
func (ls *Logs) Len() int      { return ls.n }
func (ls *Logs) At(i int) *Log { return &ls.d[i] }

func (ls *Logs) Insert(i int, b []byte) {
	ls.n++
	switch {
	case i < len(ls.d):
		ls.d[i].Unmarshal(b)
	case len(ls.d) < cap(ls.d):
		l := Log{}
		l.Unmarshal(b)
		ls.d = append(ls.d, l)
	default:
		ls.d = append(ls.d, make([]Log, 8)...)
		l := Log{}
		l.Unmarshal(b)
		ls.d[i] = l
	}
}

type StorageKeys struct {
	d [][32]byte
	n int
}

func (sk *StorageKeys) MarshalRLP() []byte {
	var res [][]byte
	for i := 0; i < sk.n; i++ {
		res = append(res, rlp.Encode(sk.d[i][:]))
	}
	return rlp.List(res...)
}

func (sk *StorageKeys) Reset()            { sk.n = 0 }
func (sk *StorageKeys) Len() int          { return sk.n }
func (sk *StorageKeys) At(i int) [32]byte { return sk.d[i] }

func (sk *StorageKeys) Insert(i int, b []byte) {
	sk.n++
	switch {
	case i < len(sk.d):
		copy(sk.d[i][:], b)
	case len(sk.d) < cap(sk.d):
		sk.d = append(sk.d, [32]byte(b))
	default:
		sk.d = append(sk.d, make([][32]byte, 8)...)
		copy(sk.d[i][:], b)
	}
}

type AccessTuple struct {
	Address     [20]byte
	StorageKeys StorageKeys
}

func (at *AccessTuple) MarshalRLP() []byte {
	return rlp.List(
		rlp.Encode(at.Address[:]),
		at.StorageKeys.MarshalRLP(),
	)
}

func (at *AccessTuple) Reset() {
	at.StorageKeys.Reset()
}

func (at *AccessTuple) Unmarshal(b []byte) error {
	iter := rlp.Iter(b)
	copy(at.Address[:], iter.Bytes())
	at.StorageKeys.Reset()
	for i, it := 0, rlp.Iter(iter.Bytes()); it.HasNext(); i++ {
		at.StorageKeys.Insert(i, it.Bytes())
	}
	return nil
}

type AccessList struct {
	d []AccessTuple
	n int
}

func (al AccessList) MarshalRLP() []byte {
	var res [][]byte
	for i := 0; i < al.n; i++ {
		res = append(res, al.d[i].MarshalRLP())
	}
	return rlp.List(res...)
}

func (al *AccessList) Reset()               { al.n = 0 }
func (al *AccessList) Len() int             { return al.n }
func (al *AccessList) At(i int) AccessTuple { return al.d[i] }

func (al *AccessList) Insert(i int, b []byte) {
	al.n++
	switch {
	case i < len(al.d):
		al.d[i].Unmarshal(b)
	case len(al.d) < cap(al.d):
		t := AccessTuple{}
		t.Unmarshal(b)
		al.d = append(al.d, t)
	default:
		al.d = append(al.d, make([]AccessTuple, 4)...)
		t := AccessTuple{}
		t.Unmarshal(b)
		al.d[i] = t
	}
}

type Transaction struct {
	Type     byte
	ChainID  uint256.Int
	Nonce    uint64
	GasPrice uint256.Int
	GasLimit uint64
	To       []byte
	Value    uint256.Int
	Data     []byte
	V, R, S  uint256.Int

	// EIP-2930
	AccessList AccessList

	// EIP-1559
	MaxPriorityFeePerGas uint256.Int
	MaxFeePerGas         uint256.Int

	// [Transactions] has a rlp buffer which
	// holds a pointer to the entire list of
	// transactions for a block and each [Transaction]
	// has an rlp buffer for its subslice.
	// For now this is only used to calculate
	// the [Transaction.Hash] on demand.
	// The hash is cached since it can be expensive.
	cacheMut           sync.Mutex
	hash, rbuf, signer []byte
}

func (tx *Transaction) Reset() {
	tx.cacheMut.Lock()
	tx.hash = nil
	tx.signer = nil
	tx.cacheMut.Unlock()
}

func (tx *Transaction) Hash() []byte {
	tx.cacheMut.Lock()
	defer tx.cacheMut.Unlock()
	if tx.hash == nil {
		tx.hash = isxhash.Keccak(tx.rbuf)
	}
	return tx.hash
}

func (tx *Transaction) Signer() ([]byte, error) {
	tx.cacheMut.Lock()
	defer tx.cacheMut.Unlock()
	if len(tx.signer) > 0 {
		return tx.signer, nil
	}
	var sig [65]byte
	sig[0] = tx.v()
	copy(sig[33-len(tx.R.Bytes()):33], tx.R.Bytes())
	copy(sig[65-len(tx.S.Bytes()):65], tx.S.Bytes())
	pubk, _, err := ecdsa.RecoverCompact(sig[:], tx.SigHash())
	if err != nil {
		return nil, fmt.Errorf("recovering pubkey: %w", err)
	}
	var (
		cpk  = pubk.SerializeUncompressed()
		addr = isxhash.Keccak(cpk[1:])
	)
	tx.signer = addr[12:]
	return tx.signer, nil
}

func (tx *Transaction) v() byte {
	switch v := tx.V.Uint64(); {
	case v >= 35:
		return byte(27 + ((v - 35) % 2))
	case v == 27 || v == 28:
		return byte(v)
	case v == 0 || v == 1:
		return byte(27 + v)
	default:
		panic(fmt.Sprintf("unkown v: %d", v))
	}
}

func (tx *Transaction) eip155() bool {
	switch v := tx.V.Uint64(); {
	case v == 27 || v == 28:
		return false
	default:
		return true
	}
}

func (tx *Transaction) chainid() uint64 {
	switch v := tx.V.Uint64(); {
	case v >= 35:
		return (v - 35) >> 1
	default:
		return 0
	}
}

func (t *Transaction) SigHash() []byte {
	switch t.Type {
	case 0x00:
		switch {
		case t.eip155():
			return isxhash.Keccak(rlp.List(
				rlp.Encode(bint.Encode(nil, t.Nonce)),
				rlp.Encode(t.GasPrice.Bytes()),
				rlp.Encode(bint.Encode(nil, t.GasLimit)),
				rlp.Encode(t.To),
				rlp.Encode(t.Value.Bytes()),
				rlp.Encode(t.Data),
				rlp.Encode(bint.Encode(nil, t.chainid())),
				rlp.Encode([]byte{0}),
				rlp.Encode([]byte{0}),
			))
		default:
			return isxhash.Keccak(rlp.List(
				rlp.Encode(bint.Encode(nil, t.Nonce)),
				rlp.Encode(t.GasPrice.Bytes()),
				rlp.Encode(bint.Encode(nil, t.GasLimit)),
				rlp.Encode(t.To),
				rlp.Encode(t.Value.Bytes()),
				rlp.Encode(t.Data),
			))
		}
	case 0x01:
		return isxhash.Keccak(append([]byte{0x01}, rlp.List(
			rlp.Encode(t.ChainID.Bytes()),
			rlp.Encode(bint.Encode(nil, t.Nonce)),
			rlp.Encode(t.GasPrice.Bytes()),
			rlp.Encode(bint.Encode(nil, t.GasLimit)),
			rlp.Encode(t.To),
			rlp.Encode(t.Value.Bytes()),
			rlp.Encode(t.Data),
			t.AccessList.MarshalRLP(),
		)...))
	case 0x02:
		return isxhash.Keccak(append([]byte{0x02}, rlp.List(
			rlp.Encode(t.ChainID.Bytes()),
			rlp.Encode(bint.Encode(nil, t.Nonce)),
			rlp.Encode(t.MaxPriorityFeePerGas.Bytes()),
			rlp.Encode(t.MaxFeePerGas.Bytes()),
			rlp.Encode(bint.Encode(nil, t.GasLimit)),
			rlp.Encode(t.To),
			rlp.Encode(t.Value.Bytes()),
			rlp.Encode(t.Data),
			t.AccessList.MarshalRLP(),
		)...))
	default:
		return nil
	}
}

type Transactions struct {
	d []Transaction
	n int
}

func (txs *Transactions) Reset()                { txs.n = 0 }
func (txs *Transactions) Len() int              { return txs.n }
func (txs *Transactions) At(i int) *Transaction { return &txs.d[i] }

func (txs *Transactions) Insert(i int, b []byte) {
	txs.n++
	switch {
	case i < len(txs.d):
		txs.d[i].Unmarshal(b)
	case len(txs.d) < cap(txs.d):
		t := Transaction{}
		t.Unmarshal(b)
		txs.d = append(txs.d, t)
	default:
		txs.d = append(txs.d, make([]Transaction, 512)...)
		t := Transaction{}
		t.Unmarshal(b)
		txs.d[i] = t
	}
}

func (tx *Transaction) Unmarshal(b []byte) error {
	tx.Reset()
	tx.rbuf = growcopy(tx.rbuf, b)
	if len(b) < 1 {
		return fmt.Errorf("decoding empty transaction bytes")
	}
	// Legacy Transaction
	if iter := rlp.Iter(b); iter.HasNext() {
		tx.Type = 0x00
		tx.Nonce = bint.Decode(iter.Bytes())
		tx.GasPrice.SetBytes(iter.Bytes())
		tx.GasLimit = bint.Decode(iter.Bytes())
		tx.To = growcopy(tx.To, iter.Bytes())
		tx.Value.SetBytes(iter.Bytes())
		tx.Data = growcopy(tx.Data, iter.Bytes())
		tx.V.SetBytes(iter.Bytes())
		tx.R.SetBytes(iter.Bytes())
		tx.S.SetBytes(iter.Bytes())
		return nil
	}
	// EIP-2718: Typed Transaction
	// https://eips.ethereum.org/EIPS/eip-2718
	switch iter := rlp.Iter(b[1:]); b[0] {
	case 0x01:
		// EIP-2930: Access List
		// https://eips.ethereum.org/EIPS/eip-2930
		tx.Type = 0x01
		tx.ChainID.SetBytes(iter.Bytes())
		tx.Nonce = bint.Decode(iter.Bytes())
		tx.GasPrice.SetBytes(iter.Bytes())
		tx.GasLimit = bint.Decode(iter.Bytes())
		tx.To = growcopy(tx.To, iter.Bytes())
		tx.Value.SetBytes(iter.Bytes())
		tx.Data = growcopy(tx.Data, iter.Bytes())
		tx.AccessList.Reset()
		for i, it := 0, rlp.Iter(iter.Bytes()); it.HasNext(); i++ {
			tx.AccessList.Insert(i, it.Bytes())
		}
		tx.V.SetBytes(iter.Bytes())
		tx.R.SetBytes(iter.Bytes())
		tx.S.SetBytes(iter.Bytes())
		return nil
	case 0x02:
		// EIP-1559: Dynamic Fee
		// https://eips.ethereum.org/EIPS/eip-1559
		tx.Type = 0x02
		tx.ChainID.SetBytes(iter.Bytes())
		tx.Nonce = bint.Decode(iter.Bytes())
		tx.MaxPriorityFeePerGas.SetBytes(iter.Bytes())
		tx.MaxFeePerGas.SetBytes(iter.Bytes())
		tx.GasLimit = bint.Decode(iter.Bytes())
		tx.To = growcopy(tx.To, iter.Bytes())
		tx.Value.SetBytes(iter.Bytes())
		tx.Data = growcopy(tx.Data, iter.Bytes())
		tx.AccessList.Reset()
		for i, it := 0, rlp.Iter(iter.Bytes()); it.HasNext(); i++ {
			tx.AccessList.Insert(i, it.Bytes())
		}
		tx.V.SetBytes(iter.Bytes())
		tx.R.SetBytes(iter.Bytes())
		tx.S.SetBytes(iter.Bytes())
		return nil
	default:
		return fmt.Errorf("unsupported tx type: 0x%X", b[0])
	}
}
