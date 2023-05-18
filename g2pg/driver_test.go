package g2pg

import (
	"context"
	"database/sql"
	"sort"
	"sync"
	"testing"

	"blake.io/pqx/pqxtest"
	"github.com/indexsupply/x/eth"
	"github.com/indexsupply/x/tc"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"kr.dev/diff"
)

func TestMain(m *testing.M) {
	sql.Register("postgres", stdlib.GetDefaultDriver())
	pqxtest.TestMain(m)
}

type testIntegration struct {
	sync.Mutex
	chain map[[32]byte]eth.Block
}

func newTestIntegration() *testIntegration {
	return &testIntegration{
		chain: make(map[[32]byte]eth.Block),
	}
}

func (ti *testIntegration) blocks() []eth.Block {
	var blks []eth.Block
	for _, b := range ti.chain {
		blks = append(blks, b)
	}
	sort.Slice(blks, func(i, j int) bool {
		return blks[i].Number < blks[j].Number
	})
	return blks
}

func (ti *testIntegration) Insert(ctx context.Context, _ PG, blocks []eth.Block) (int64, error) {
	ti.Lock()
	defer ti.Unlock()
	for _, b := range blocks {
		ti.chain[b.Hash] = b
	}
	return int64(len(blocks)), nil
}

func (ti *testIntegration) Delete(ctx context.Context, pg PG, h []byte) error {
	ti.Lock()
	defer ti.Unlock()
	delete(ti.chain, [32]byte(h))
	return nil
}

type testGeth struct {
	blocks []eth.Block
}

func (tg *testGeth) Latest() (uint64, [32]byte, error) {
	if len(tg.blocks) == 0 {
		return 0, [32]byte{}, nil
	}
	b := tg.blocks[len(tg.blocks)-1]
	return b.Number, b.Hash, nil
}

func (tg *testGeth) Blocks(blks []eth.Block) error {
	for i := range blks {
		for j := range tg.blocks {
			if blks[i].Number == tg.blocks[j].Number {
				blks[i].Hash = tg.blocks[j].Hash
				blks[i].Header = tg.blocks[j].Header
			}
		}
	}
	return nil
}

func hash(b byte) (res [32]byte) {
	res[0] = b
	return
}

func testpg(t *testing.T) *pgxpool.Pool {
	pqxtest.CreateDB(t, `create table driver (number bigint, hash bytea)`)
	ctx := context.Background()
	pg, err := pgxpool.New(ctx, pqxtest.DSNForTest(t))
	tc.NoErr(t, err)
	return pg
}

func driverAdd(t *testing.T, pg PG, ig Integration, number uint64, h [32]byte) {
	const q = "insert into driver(number,hash) values ($1, $2)"
	_, err := pg.Exec(context.Background(), q, number, h[:])
	tc.NoErr(t, err)
	ig.Insert(context.Background(), pg, []eth.Block{
		eth.Block{
			Number: number,
			Hash:   h,
		},
	})
}

func TestIndexBatch_Zero(t *testing.T) {
	var (
		ctx = context.Background()
		g   = &testGeth{}
		pg  = testpg(t)
		td  = NewDriver(1, 1, newTestIntegration())
	)
	diff.Test(t, t.Errorf, td.IndexBatch(ctx, g, pg), ErrNothingNew)
}

func TestIndexBatch_EmptyIntegration(t *testing.T) {
	var (
		ctx = context.Background()
		g   = &testGeth{
			blocks: []eth.Block{
				eth.Block{
					Number: 0,
					Hash:   hash('a'),
				},
				eth.Block{
					Number: 1,
					Hash:   hash('b'),
					Header: eth.Header{Parent: hash('a')},
				},
			},
		}
		pg = testpg(t)
		ig = newTestIntegration()
		td = NewDriver(1, 1, ig)
	)
	tc.NoErr(t, td.IndexBatch(ctx, g, pg))
	diff.Test(t, t.Errorf, ig.blocks(), g.blocks[1:])
}

func TestIndexBatch_Reorg(t *testing.T) {
	var (
		ctx = context.Background()
		g   = &testGeth{
			blocks: []eth.Block{
				eth.Block{
					Number: 0,
					Hash:   hash(0),
				},
				eth.Block{
					Number: 1,
					Hash:   hash(2),
					Header: eth.Header{Parent: hash(0)},
				},
				eth.Block{
					Number: 2,
					Hash:   hash(3),
					Header: eth.Header{Parent: hash(2)},
				},
			},
		}
		pg = testpg(t)
		ig = newTestIntegration()
		td = NewDriver(1, 1, ig)
	)

	driverAdd(t, pg, ig, 0, hash(0))
	driverAdd(t, pg, ig, 1, hash(1))

	diff.Test(t, t.Errorf, ErrReorg, td.IndexBatch(ctx, g, pg))
	diff.Test(t, t.Errorf, nil, td.IndexBatch(ctx, g, pg))
	diff.Test(t, t.Errorf, nil, td.IndexBatch(ctx, g, pg))
	diff.Test(t, t.Errorf, ig.blocks(), g.blocks)
}

func TestIndexBatch_DeltaBatchSize(t *testing.T) {
	const (
		batchSize = 16
		workers   = 2
	)
	var (
		ctx = context.Background()
		g   = &testGeth{
			blocks: []eth.Block{
				eth.Block{
					Number: 0,
					Hash:   hash(0),
				},
			},
		}
		pg = testpg(t)
		ig = newTestIntegration()
		td = NewDriver(batchSize, workers, ig)
	)
	driverAdd(t, pg, ig, 0, hash(0))
	for i := uint64(1); i <= batchSize+1; i++ {
		g.blocks = append(g.blocks, eth.Block{
			Number: i,
			Hash:   hash(byte(i)),
			Header: eth.Header{Parent: hash(byte(i - 1))},
		})
	}
	diff.Test(t, t.Errorf, nil, td.IndexBatch(ctx, g, pg))
	diff.Test(t, t.Errorf, ig.blocks(), g.blocks[:batchSize+1])

	diff.Test(t, t.Errorf, nil, td.IndexBatch(ctx, g, pg))
	diff.Test(t, t.Errorf, ig.blocks(), g.blocks)
}
