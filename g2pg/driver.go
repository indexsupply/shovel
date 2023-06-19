package g2pg

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

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

//go:embed schema.sql
var Schema string

type G interface {
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
	Insert(PG, []Block) (int64, error)
	Delete(PG, []byte) error
	Events() [][]byte
}

func NewDriver(
	batchSize int,
	workers int,
	g G,
	pgp *pgxpool.Pool,
	intgs ...Integration,
) *Driver {
	var filter [][]byte
	for i := range intgs {
		e := intgs[i].Events()
		// if one integration has no filter
		// then the driver must consider all data
		if len(e) == 0 {
			filter = filter[:0]
			break
		}
		filter = append(filter, e...)
	}
	return &Driver{
		batch:     make([]Block, batchSize),
		buffs:     make([]geth.Buffer, batchSize),
		batchSize: batchSize,
		workers:   workers,
		geth:      g,
		pgp:       pgp,
		intgs:     intgs,
		filter:    filter,
		stat: status{
			tlat:  quantile.NewTargeted(0.50, 0.90, 0.99),
			glat:  quantile.NewTargeted(0.50, 0.90, 0.99),
			pglat: quantile.NewTargeted(0.50, 0.90, 0.99),
		},
	}
}

type Driver struct {
	intgs     []Integration
	filter    [][]byte
	batch     []Block
	buffs     []geth.Buffer
	batchSize int
	workers   int
	stat      status
	geth      G
	pgp       *pgxpool.Pool
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

func (d *Driver) Status() StatusSnapshot {
	printer := message.NewPrinter(message.MatchLanguage("en"))
	snap := StatusSnapshot{}
	snap.EthHash = fmt.Sprintf("%x", d.stat.ehash[:4])
	snap.EthNum = fmt.Sprintf("%d", d.stat.enum)
	snap.Hash = fmt.Sprintf("%x", d.stat.ihash[:4])
	snap.Num = printer.Sprintf("%d", d.stat.inum)
	snap.BlockCount = fmt.Sprintf("%d", atomic.SwapInt64(&d.stat.blocks, 0))
	snap.EventCount = fmt.Sprintf("%d", atomic.SwapInt64(&d.stat.events, 0))
	snap.TotalLatencyP50 = fmt.Sprintf("%s", time.Duration(d.stat.tlat.Query(0.50)).Round(time.Millisecond))
	snap.TotalLatencyP95 = fmt.Sprintf("%.2f", d.stat.tlat.Query(0.95))
	snap.TotalLatencyP99 = fmt.Sprintf("%.2f", d.stat.tlat.Query(0.99))
	snap.Error = fmt.Sprintf("%v", d.stat.err)
	d.stat.tlat.Reset()
	return snap
}

func min(x, y int) int {
	if x < y {
		return x
	}
	return y
}

func Insert(pg PG, n uint64, h []byte) error {
	const q = `insert into driver (number, hash) values ($1, $2)`
	_, err := pg.Exec(context.Background(), q, n, h)
	return err
}

func Latest(pg PG) (uint64, []byte, error) {
	const q = `SELECT number, hash FROM driver ORDER BY number DESC LIMIT 1`
	var n, h = uint64(0), []byte{}
	err := pg.QueryRow(context.Background(), q).Scan(&n, &h)
	if errors.Is(err, pgx.ErrNoRows) {
		return n, nil, nil
	}
	return n, h, err
}

var (
	ErrNothingNew = errors.New("no new blocks")
	ErrReorg      = errors.New("reorg")
	ErrDone       = errors.New("this is the end")
)

// Indexes at most d.batchSize of the delta between min(g, limit) and pg.
// If pg contains an invalid latest block (ie reorg) then [ErrReorg]
// is returned and the caller may rollback the transaction resulting
// in no side-effects.
func (d *Driver) Converge(usetx bool, limit uint64) error {
	var (
		err      error
		start       = time.Now()
		ctx         = context.Background()
		pg       PG = d.pgp
		pgTx     pgx.Tx
		commit   = func() error { return nil }
		rollback = func() error { return nil }
	)
	if usetx {
		pgTx, err = d.pgp.Begin(ctx)
		if err != nil {
			return err
		}
		pg = txlocker.NewTx(pgTx)
		commit = func() error { return pgTx.Commit(ctx) }
		rollback = func() error { return pgTx.Rollback(ctx) }
		defer rollback()
	}
	for reorgs := 0; reorgs <= 10; {
		localNum, localHash, err := Latest(pg)
		if err != nil {
			return fmt.Errorf("getting latest from driver: %w", err)
		}
		if limit > 0 && localNum >= limit {
			return ErrDone
		}
		gethNum, gethHash, err := d.geth.Latest()
		if err != nil {
			return fmt.Errorf("getting latest from eth: %w", err)
		}
		if limit > 0 && gethNum > limit {
			gethNum = limit
		}
		delta := min(int(gethNum-localNum), d.batchSize)
		if delta <= 0 {
			return ErrNothingNew
		}
		for i := 0; i < delta; i++ {
			d.buffs[i].Number = localNum + 1 + uint64(i)
			d.batch[i].Transactions.Reset()
			d.batch[i].Receipts.Reset()
		}
		switch err := d.writeIndex(localHash, pg, delta); {
		case errors.Is(err, ErrReorg):
			reorgs++
			fmt.Printf("reorg. deleting %d %x\n", localNum, localHash)
			const dq = "delete from driver where hash = $1"
			_, err := pg.Exec(ctx, dq, localHash)
			if err != nil {
				return fmt.Errorf("deleting block from driver table: %w", err)
			}
			for _, ig := range d.intgs {
				if err := ig.Delete(pg, localHash); err != nil {
					return fmt.Errorf("deleting block from integration: %w", err)
				}
			}
		case err != nil:
			err = errors.Join(rollback(), err)
			d.stat.err = err
			return err
		default:
			d.stat.enum = gethNum
			d.stat.ehash = gethHash
			d.stat.blocks += int64(delta)
			d.stat.tlat.Insert(float64(time.Since(start)))
			return commit()
		}
	}
	return errors.Join(ErrReorg, rollback())
}

func (d *Driver) skip(bf bloom.Filter) bool {
	if len(d.filter) == 0 {
		return false
	}
	for _, sig := range d.filter {
		if !bf.Missing(sig) {
			return false
		}
	}
	return true
}

// Fills in d.batch with block data (headers, bodies, receipts) from
// geth and then calls Index on the driver's integrations with the block
// data.
//
// Once the block data has been read, it is checked against the parent
// hash to ensure consistency in the local chain. If the newly read data
// doesn't match [ErrReorg] is returned.
//
// The reading of block data and indexing of integrations happens concurrently
// with the number of go routines controlled by c.
func (d *Driver) writeIndex(localHash []byte, pg PG, delta int) error {
	var (
		eg    = errgroup.Group{}
		wsize = d.batchSize / d.workers
	)
	for i := 0; i < d.workers && i*wsize < delta; i++ {
		n := i * wsize
		m := n + wsize
		bfs := d.buffs[n:min(m, delta)]
		blks := d.batch[n:min(m, delta)]
		if len(blks) == 0 {
			continue
		}
		eg.Go(func() error { return d.geth.LoadBlocks(d.filter, bfs, blks) })
	}
	if err := eg.Wait(); err != nil {
		return err
	}
	if len(d.batch[0].Header.Parent) != 32 {
		return fmt.Errorf("corrupt parent: %x\n", d.batch[0].Header.Parent)
	}
	if !bytes.Equal(localHash, d.batch[0].Header.Parent) {
		fmt.Printf("local: %x new: %x\n", localHash, d.batch[0].Header.Parent)
		return ErrReorg
	}
	eg = errgroup.Group{}
	for i := 0; i < d.workers && i*wsize < delta; i++ {
		n := i * wsize
		m := n + wsize
		blks := d.batch[n:min(m, delta)]
		if len(blks) == 0 {
			continue
		}
		eg.Go(func() error {
			var igeg errgroup.Group
			for _, ig := range d.intgs {
				ig := ig
				igeg.Go(func() error {
					count, err := ig.Insert(pg, blks)
					atomic.AddInt64(&d.stat.events, count)
					return err
				})
			}
			return igeg.Wait()
		})
	}
	if err := eg.Wait(); err != nil {
		return fmt.Errorf("writing indexed data: %w", err)
	}
	var last = d.batch[delta-1]
	const uq = "insert into driver (number, hash) values ($1, $2)"
	_, err := pg.Exec(context.Background(), uq, last.Num(), last.Hash())
	if err != nil {
		return fmt.Errorf("updating driver table: %w", err)
	}
	d.stat.inum = last.Num()
	d.stat.ihash = last.Hash()
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
	sp := b.Header.Parent
	if len(sp) > 4 {
		sp = sp[:4]
	}
	sh := b.Header.Hash
	if len(sh) > 4 {
		sh = sh[:4]
	}
	return fmt.Sprintf("{%d %x %x}", b.Header.Number, sp, sh)
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

func (h *Header) Unmarshal(input []byte) {
	h.Hash = isxhash.Keccak(input)
	for i, itr := 0, rlp.Iter(input); itr.HasNext(); i++ {
		d := itr.Bytes()
		switch i {
		case 0:
			h.Parent = d
		case 6:
			h.LogsBloom = d
		case 8:
			h.Number = bint.Decode(d)
		case 11:
			h.Time = bint.Decode(d)
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
	r.Status = iter.Bytes()
	r.GasUsed = bint.Decode(iter.Bytes())
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
	ts.d[i] = b
}

type Log struct {
	Address []byte
	Topics  Topics
	Data    []byte
}

func (l *Log) Unmarshal(input []byte) {
	iter := rlp.Iter(input)
	l.Address = iter.Bytes()
	l.Topics.Reset()
	for i, t := 0, rlp.Iter(iter.Bytes()); t.HasNext(); i++ {
		l.Topics.Insert(i, t.Bytes())
	}
	l.Data = iter.Bytes()
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

func (sk *StorageKeys) Reset()            { sk.n = 0 }
func (sk *StorageKeys) Len() int          { return sk.n }
func (sk *StorageKeys) At(i int) [32]byte { return sk.d[i] }

func (sk *StorageKeys) Insert(i int, b []byte) {
	sk.n++
	switch {
	case i < len(sk.d):
		sk.d[i] = [32]byte(b)
	case len(sk.d) < cap(sk.d):
		sk.d = append(sk.d, [32]byte(b))
	default:
		sk.d = append(sk.d, make([][32]byte, 8)...)
		sk.d[i] = [32]byte(b)
	}
}

type AccessTuple struct {
	Address     [20]byte
	StorageKeys StorageKeys
}

func (at *AccessTuple) Reset() {
	at.StorageKeys.Reset()
}

func (at *AccessTuple) Unmarshal(b []byte) error {
	iter := rlp.Iter(b)
	at.Address = [20]byte(iter.Bytes())
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
	hash, rbuf []byte
}

func (t *Transaction) Hash() []byte {
	if t.hash == nil {
		t.hash = isxhash.Keccak(t.rbuf)
	}
	return t.hash
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
		txs.d[i].rbuf = b
		txs.d[i].hash = nil // reset hash cache on reuse
	case len(txs.d) < cap(txs.d):
		t := Transaction{}
		t.rbuf = b
		t.Unmarshal(b)
		txs.d = append(txs.d, t)
	default:
		txs.d = append(txs.d, make([]Transaction, 512)...)
		t := Transaction{}
		t.rbuf = b
		t.Unmarshal(b)
		txs.d[i] = t
	}
}

func (tx *Transaction) Unmarshal(b []byte) error {
	if len(b) < 1 {
		return fmt.Errorf("decoding empty transaction bytes")
	}
	// Legacy Transaction
	if iter := rlp.Iter(b); iter.HasNext() {
		tx.Nonce = bint.Decode(iter.Bytes())
		tx.GasPrice.SetBytes(iter.Bytes())
		tx.GasLimit = bint.Decode(iter.Bytes())
		tx.To = iter.Bytes()
		tx.Value.SetBytes(iter.Bytes())
		tx.Data = iter.Bytes()
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
		tx.ChainID.SetBytes(iter.Bytes())
		tx.Nonce = bint.Decode(iter.Bytes())
		tx.GasPrice.SetBytes(iter.Bytes())
		tx.GasLimit = bint.Decode(iter.Bytes())
		tx.To = iter.Bytes()
		tx.Value.SetBytes(iter.Bytes())
		tx.Data = iter.Bytes()
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
		tx.ChainID.SetBytes(iter.Bytes())
		tx.Nonce = bint.Decode(iter.Bytes())
		tx.MaxPriorityFeePerGas.SetBytes(iter.Bytes())
		tx.MaxFeePerGas.SetBytes(iter.Bytes())
		tx.GasLimit = bint.Decode(iter.Bytes())
		tx.To = iter.Bytes()
		tx.Value.SetBytes(iter.Bytes())
		tx.Data = iter.Bytes()
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
