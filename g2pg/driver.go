package g2pg

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/indexsupply/x/eth"
	"github.com/indexsupply/x/txlocker"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/bmizerany/perks/quantile"
	"golang.org/x/sync/errgroup"
	"golang.org/x/text/message"
)

type G interface {
	Latest() (uint64, [32]byte, error)
	Blocks([]eth.Block) error
}

type PG interface {
	CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error)
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type Integration interface {
	Insert(PG, []eth.Block) (int64, error)
	Delete(PG, []byte) error
}

func NewDriver(batchSize, workers int, intgs ...Integration) *Driver {
	drv := &Driver{
		intgs:     intgs,
		batch:     make([]eth.Block, batchSize),
		batchSize: batchSize,
		workers:   workers,
		stat: status{
			tlat:  quantile.NewTargeted(0.50, 0.90, 0.99),
			glat:  quantile.NewTargeted(0.50, 0.90, 0.99),
			pglat: quantile.NewTargeted(0.50, 0.90, 0.99),
		},
	}
	return drv
}

type Driver struct {
	intgs     []Integration
	batch     []eth.Block
	batchSize int
	workers   int
	stat      status
}

type status struct {
	ehash, ihash      [32]byte
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

func (d *Driver) Insert(pg PG, n uint64, h [32]byte) error {
	const q = `insert into driver (number, hash) values ($1, $2)`
	_, err := pg.Exec(context.Background(), q, n, h[:])
	return err
}

func (d *Driver) Latest(pg PG) (uint64, [32]byte, error) {
	const q = `SELECT number, hash FROM driver ORDER BY number DESC LIMIT 1`
	var n, h = uint64(0), []byte{}
	err := pg.QueryRow(context.Background(), q).Scan(&n, &h)
	if errors.Is(err, pgx.ErrNoRows) {
		return n, [32]byte{}, nil
	}
	return n, [32]byte(h), err
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
func (d *Driver) Converge(g G, pgp *pgxpool.Pool, tx bool, limit uint64) error {
	var (
		start       = time.Now()
		ctx         = context.Background()
		pg       PG = pgp
		pgTx     pgx.Tx
		commit   = func() error { return nil }
		rollback = func() error { return nil }
	)
	for reorgs := 0; reorgs <= 10; {
		localNum, localHash, err := d.Latest(pg)
		if err != nil {
			return fmt.Errorf("getting latest from driver: %w", err)
		}
		if limit > 0 && localNum >= limit {
			return ErrDone
		}
		gethNum, gethHash, err := g.Latest()
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
			d.batch[i].Number = localNum + 1 + uint64(i)
		}
		if tx {
			pgTx, err = pgp.Begin(ctx)
			if err != nil {
				return err
			}
			pg = txlocker.NewTx(pgTx)
			commit = func() error { return pgTx.Commit(ctx) }
			rollback = func() error { return pgTx.Rollback(ctx) }
		}
		switch err := d.writeIndex(localHash, g, pg, delta); {
		case errors.Is(err, ErrReorg):
			reorgs++
			fmt.Printf("reorg. deleting %d %x\n", localNum, localHash[:4])
			const dq = "delete from driver where hash = $1"
			_, err := pg.Exec(ctx, dq, localHash[:])
			if err != nil {
				return fmt.Errorf("deleting block from driver table: %w", err)
			}
			for _, ig := range d.intgs {
				if err := ig.Delete(pg, localHash[:]); err != nil {
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
	return ErrReorg
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
func (d *Driver) writeIndex(parent [32]byte, g G, pg PG, delta int) error {
	var (
		eg    = errgroup.Group{}
		wsize = d.batchSize / d.workers
	)
	for i := 0; i < d.workers && i*wsize < delta; i++ {
		n := i * wsize
		m := n + wsize
		s := d.batch[n:min(m, delta)]
		eg.Go(func() error { return g.Blocks(s) })
	}
	if err := eg.Wait(); err != nil {
		return err
	}
	if parent != d.batch[0].Header.Parent {
		fmt.Printf("parent: %x batch: %x\n", parent, d.batch[0].Header.Parent)
		return ErrReorg
	}
	eg = errgroup.Group{}
	for i := 0; i < d.workers && i*wsize < delta; i++ {
		n := i * wsize
		m := n + wsize
		s := d.batch[n:min(m, delta)]
		eg.Go(func() error {
			var igeg errgroup.Group
			for _, ig := range d.intgs {
				ig := ig
				igeg.Go(func() error {
					count, err := ig.Insert(pg, s)
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
	_, err := pg.Exec(context.Background(), uq, last.Number, last.Hash[:])
	if err != nil {
		return fmt.Errorf("updating driver table: %w", err)
	}
	d.stat.inum = last.Number
	d.stat.ihash = last.Hash
	return nil
}
