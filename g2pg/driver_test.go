package g2pg

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"sync"
	"testing"

	"github.com/indexsupply/x/geth"
	"github.com/indexsupply/x/tc"

	"blake.io/pqx/pqxtest"
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
	chain map[string]Block
}

func newTestIntegration() *testIntegration {
	return &testIntegration{
		chain: make(map[string]Block),
	}
}

func (ti *testIntegration) blocks() []Block {
	var blks []Block
	for _, b := range ti.chain {
		blks = append(blks, b)
	}
	sort.Slice(blks, func(i, j int) bool {
		return blks[i].Num() < blks[j].Num()
	})
	return blks
}

func (ti *testIntegration) Insert(_ PG, blocks []Block) (int64, error) {
	ti.Lock()
	defer ti.Unlock()
	for _, b := range blocks {
		ti.chain[fmt.Sprintf("%x", b.Hash())] = b
	}
	return int64(len(blocks)), nil
}

func (ti *testIntegration) Delete(pg PG, h []byte) error {
	ti.Lock()
	defer ti.Unlock()
	delete(ti.chain, fmt.Sprintf("%x", h))
	return nil
}

func (ti *testIntegration) Events() [][]byte {
	return nil
}

type testGeth struct {
	blocks []Block
}

func (tg *testGeth) Hash(_ uint64) ([]byte, error) {
	return nil, nil
}

func (tg *testGeth) Latest() (uint64, []byte, error) {
	if len(tg.blocks) == 0 {
		return 0, nil, nil
	}
	b := tg.blocks[len(tg.blocks)-1]
	return b.Num(), b.Hash(), nil
}

func (tg *testGeth) LoadBlocks(filter [][]byte, bufs []geth.Buffer, blks []Block) error {
	for i := range bufs {
		for j := range tg.blocks {
			if bufs[i].Number == tg.blocks[j].Num() {
				blks[i].Header = tg.blocks[j].Header
			}
		}
	}
	return nil
}

func (tg *testGeth) add(n uint64, h, p []byte) {
	tg.blocks = append(tg.blocks, Block{
		Header: Header{
			Number: n,
			Hash:   h,
			Parent: p,
		},
	})
}

func hash(b byte) []byte {
	res := make([]byte, 32)
	res[0] = b
	return res
}

func testpg(t *testing.T) *pgxpool.Pool {
	pqxtest.CreateDB(t, `create table driver (number bigint, hash bytea)`)
	ctx := context.Background()
	pg, err := pgxpool.New(ctx, pqxtest.DSNForTest(t))
	tc.NoErr(t, err)
	return pg
}

func driverAdd(t *testing.T, pg PG, ig Integration, n uint64, h, p []byte) {
	const q = "insert into driver(number,hash) values ($1, $2)"
	_, err := pg.Exec(context.Background(), q, n, h)
	tc.NoErr(t, err)
	ig.Insert(pg, []Block{
		Block{
			Header: Header{
				Number: n,
				Hash:   h,
				Parent: p,
			},
		},
	})
}

func TestConverge_Zero(t *testing.T) {
	var (
		g  = &testGeth{}
		pg = testpg(t)
		td = NewDriver(1, 1, g, pg, newTestIntegration())
	)
	diff.Test(t, t.Errorf, td.Converge(false, 0), ErrNothingNew)
}

func TestConverge_EmptyIntegration(t *testing.T) {
	var (
		pg = testpg(t)
		tg = &testGeth{}
		ig = newTestIntegration()
		td = NewDriver(1, 1, tg, pg, ig)
	)

	tg.add(0, hash(0), hash(0))
	tg.add(1, hash(1), hash(0))
	driverAdd(t, pg, ig, 0, hash(0), hash(0))

	diff.Test(t, t.Fatalf, td.Converge(false, 0), nil)
	diff.Test(t, t.Errorf, ig.blocks(), tg.blocks)
}

func TestConverge_Reorg(t *testing.T) {
	var (
		pg = testpg(t)
		tg = &testGeth{}
		ig = newTestIntegration()
		td = NewDriver(1, 1, tg, pg, ig)
	)

	tg.add(0, hash(0), hash(0))
	tg.add(1, hash(2), hash(0))
	tg.add(2, hash(3), hash(2))
	driverAdd(t, pg, ig, 0, hash(0), hash(0))
	driverAdd(t, pg, ig, 1, hash(1), hash(0))

	diff.Test(t, t.Fatalf, td.Converge(false, 0), nil)
	diff.Test(t, t.Fatalf, td.Converge(false, 0), nil)
	diff.Test(t, t.Errorf, ig.blocks(), tg.blocks)
}

func TestConverge_DeltaBatchSize(t *testing.T) {
	const (
		batchSize = 16
		workers   = 2
	)
	var (
		pg = testpg(t)
		tg = &testGeth{}
		ig = newTestIntegration()
		td = NewDriver(batchSize, workers, tg, pg, ig)
	)

	tg.add(0, hash(0), hash(0))
	driverAdd(t, pg, ig, 0, hash(0), hash(0))

	for i := uint64(1); i <= batchSize+1; i++ {
		tg.add(i, hash(byte(i)), hash(byte(i-1)))
	}

	diff.Test(t, t.Errorf, nil, td.Converge(false, 0))
	diff.Test(t, t.Errorf, ig.blocks(), tg.blocks[:batchSize+1])

	diff.Test(t, t.Errorf, nil, td.Converge(false, 0))
	diff.Test(t, t.Errorf, ig.blocks(), tg.blocks)
}
