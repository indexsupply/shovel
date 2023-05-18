package g2pg

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/indexsupply/x/eth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

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
	Insert(context.Context, PG, []eth.Block) (int64, error)
	Delete(context.Context, PG, []byte) error
}

func NewDriver(batchSize, workers uint64, intgs ...Integration) *Driver {
	drv := &Driver{
		intgs:      intgs,
		batch:      make([]eth.Block, int(batchSize)),
		batchSize:  batchSize,
		workers:    workers,
		workerSize: batchSize / workers,
		stat: status{
			tlat:  quantile.NewTargeted(0.50, 0.90, 0.99),
			glat:  quantile.NewTargeted(0.50, 0.90, 0.99),
			pglat: quantile.NewTargeted(0.50, 0.90, 0.99),
		},
	}
	for i := 0; i < int(batchSize); i++ {
		drv.batch[i] = eth.Block{}
	}
	return drv
}

type Driver struct {
	intgs      []Integration
	batch      []eth.Block
	batchSize  uint64
	workers    uint64
	workerSize uint64
	stat       status
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

func min(x, y uint64) uint64 {
	if x < y {
		return x
	}
	return y
}

func (d *Driver) Latest(ctx context.Context, pg PG) (uint64, [32]byte, error) {
	const q = `SELECT number, hash FROM driver ORDER BY number DESC LIMIT 1`
	var n, h = uint64(0), []byte{}
	err := pg.QueryRow(ctx, q).Scan(&n, &h)
	if errors.Is(err, pgx.ErrNoRows) {
		return n, [32]byte{}, nil
	}
	return n, [32]byte(h), err
}

var (
	ErrNothingNew = errors.New("no new blocks")
	ErrReorg      = errors.New("reorg")
)

func (d *Driver) IndexBatch(ctx context.Context, g G, pg PG) error {
	totalTime := time.Now()
	localNum, localHash, err := d.Latest(ctx, pg)
	if err != nil {
		return fmt.Errorf("getting latest from driver: %w", err)
	}
	gethNum, gethHash, err := g.Latest()
	if err != nil {
		return fmt.Errorf("getting latest from eth: %w", err)
	}
	delta := min(gethNum-localNum, d.batchSize)
	if delta <= 0 {
		return ErrNothingNew
	}
	for i := uint64(0); i < delta; i++ {
		d.batch[i].Number = localNum + 1 + i
	}
	eg := errgroup.Group{}
	for i := uint64(0); i < d.workers && i*d.workerSize < delta; i++ {
		n := i * d.workerSize
		m := n + d.workerSize
		s := d.batch[n:min(m, delta)]
		eg.Go(func() error { return g.Blocks(s) })
	}
	if err := eg.Wait(); err != nil {
		return err
	}
	if localNum != 0 && d.batch[0].Header.Parent != localHash {
		fmt.Printf("reorg. deleting: %d %x\n", localNum, localHash[:4])
		const dq = "delete from driver where hash = $1"
		_, err := pg.Exec(ctx, dq, localHash[:])
		if err != nil {
			return fmt.Errorf("deleting block from driver table: %w", err)
		}
		for _, ig := range d.intgs {
			if err := ig.Delete(ctx, pg, localHash[:]); err != nil {
				return fmt.Errorf("deleting block from integration: %w", err)
			}
		}
		return ErrReorg
	}
	eg = errgroup.Group{}
	for i := uint64(0); i < d.workers && i*d.workerSize < delta; i++ {
		n := i * d.workerSize
		m := n + d.workerSize
		s := d.batch[n:min(m, delta)]
		eg.Go(func() error {
			var igeg errgroup.Group
			for _, ig := range d.intgs {
				ig := ig
				igeg.Go(func() error {
					count, err := ig.Insert(ctx, pg, s)
					if err != nil {
						return fmt.Errorf("integration indexing: %w", err)
					}
					atomic.AddInt64(&d.stat.events, count)
					return nil
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
	_, err = pg.Exec(ctx, uq, last.Number, last.Hash[:])
	if err != nil {
		return fmt.Errorf("updating driver table: %w", err)
	}

	d.stat.enum = gethNum
	d.stat.ehash = gethHash
	d.stat.inum = last.Number
	d.stat.ihash = last.Hash
	d.stat.blocks += int64(delta)
	d.stat.tlat.Insert(float64(time.Since(totalTime)))
	return nil
}
