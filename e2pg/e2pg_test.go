package e2pg

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
	chain map[uint64]Block
}

func newTestIntegration() *testIntegration {
	return &testIntegration{
		chain: make(map[uint64]Block),
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

func (ti *testIntegration) Insert(_ context.Context, _ PG, blocks []Block) (int64, error) {
	ti.Lock()
	defer ti.Unlock()
	for _, b := range blocks {
		ti.chain[b.Header.Number] = b
	}
	return int64(len(blocks)), nil
}

func (ti *testIntegration) add(n uint64, hash, parent []byte) {
	ti.Insert(context.Background(), nil, []Block{
		Block{
			Header: Header{
				Number: n,
				Hash:   hash,
				Parent: parent,
			},
		},
	})
}

func (ti *testIntegration) Delete(_ context.Context, pg PG, n uint64) error {
	ti.Lock()
	defer ti.Unlock()
	delete(ti.chain, n)
	return nil
}

func (ti *testIntegration) Events(_ context.Context) [][]byte {
	return nil
}

type testGeth struct {
	blocks []Block
}

func (tg *testGeth) Hash(n uint64) ([]byte, error) {
	for j := range tg.blocks {
		if tg.blocks[j].Header.Number == n {
			return tg.blocks[j].Header.Hash, nil
		}
	}
	return nil, fmt.Errorf("not found: %d", n)
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
	pqxtest.CreateDB(t, Schema)
	pg, err := pgxpool.New(context.Background(), pqxtest.DSNForTest(t))
	tc.NoErr(t, err)
	return pg
}

func TestSetup(t *testing.T) {
	var (
		tg   = &testGeth{}
		pg   = testpg(t)
		task = NewTask(0, 0, "main", 1, 1, tg, pg, 0, 0, newTestIntegration())
	)
	tg.add(0, hash(0), hash(0))
	tg.add(1, hash(1), hash(0))
	tg.add(2, hash(2), hash(1))
	diff.Test(t, t.Errorf, task.Setup(), nil)

	n, h, err := task.Latest()
	diff.Test(t, t.Errorf, err, nil)
	diff.Test(t, t.Errorf, n, uint64(1))
	diff.Test(t, t.Errorf, h, hash(1))
}

func TestConverge_Zero(t *testing.T) {
	var (
		g    = &testGeth{}
		pg   = testpg(t)
		task = NewTask(0, 0, "main", 1, 1, g, pg, 0, 0, newTestIntegration())
	)
	diff.Test(t, t.Errorf, task.Converge(false), ErrNothingNew)
}

func TestConverge_EmptyIntegration(t *testing.T) {
	var (
		pg   = testpg(t)
		tg   = &testGeth{}
		ig   = newTestIntegration()
		task = NewTask(0, 0, "main", 1, 1, tg, pg, 0, 0, ig)
	)
	tg.add(0, hash(0), hash(0))
	tg.add(1, hash(1), hash(0))
	ig.add(0, hash(0), hash(0))
	task.Insert(0, hash(0))
	diff.Test(t, t.Fatalf, task.Converge(true), nil)
	diff.Test(t, t.Errorf, ig.blocks(), tg.blocks)
}

func TestConverge_Reorg(t *testing.T) {
	var (
		pg   = testpg(t)
		tg   = &testGeth{}
		ig   = newTestIntegration()
		task = NewTask(0, 0, "main", 1, 1, tg, pg, 0, 0, ig)
	)

	tg.add(0, hash(0), hash(0))
	tg.add(1, hash(2), hash(0))
	tg.add(2, hash(3), hash(2))

	ig.add(0, hash(0), hash(0))
	ig.add(1, hash(2), hash(0))

	task.Insert(0, hash(0))
	task.Insert(1, hash(1))

	diff.Test(t, t.Fatalf, task.Converge(false), nil)
	diff.Test(t, t.Fatalf, task.Converge(false), nil)
	diff.Test(t, t.Errorf, ig.blocks(), tg.blocks)
}

func TestConverge_DeltaBatchSize(t *testing.T) {
	const (
		batchSize = 16
		workers   = 2
	)
	var (
		pg   = testpg(t)
		tg   = &testGeth{}
		ig   = newTestIntegration()
		task = NewTask(0, 0, "main", batchSize, workers, tg, pg, 0, 0, ig)
	)

	tg.add(0, hash(0), hash(0))
	ig.add(0, hash(0), hash(0))
	task.Insert(0, hash(0))

	for i := uint64(1); i <= batchSize+1; i++ {
		tg.add(i, hash(byte(i)), hash(byte(i-1)))
	}

	diff.Test(t, t.Errorf, nil, task.Converge(false))
	diff.Test(t, t.Errorf, ig.blocks(), tg.blocks[:batchSize+1])

	diff.Test(t, t.Errorf, nil, task.Converge(false))
	diff.Test(t, t.Errorf, ig.blocks(), tg.blocks)
}

func TestConverge_MultipleTasks(t *testing.T) {
	var (
		tg    = &testGeth{}
		pg    = testpg(t)
		ig1   = newTestIntegration()
		ig2   = newTestIntegration()
		task1 = NewTask(0, 0, "one", 3, 1, tg, pg, 0, 0, ig1)
		task2 = NewTask(1, 0, "two", 3, 1, tg, pg, 0, 0, ig2)
	)
	tg.add(1, hash(1), hash(0))
	tg.add(2, hash(2), hash(1))

	task1.Insert(0, hash(0))
	task2.Insert(0, hash(0))

	diff.Test(t, t.Errorf, task1.Converge(true), nil)
	diff.Test(t, t.Errorf, ig1.blocks(), tg.blocks)
	diff.Test(t, t.Errorf, len(ig2.blocks()), 0)
	diff.Test(t, t.Errorf, task2.Converge(true), nil)
	diff.Test(t, t.Errorf, ig2.blocks(), tg.blocks)
}

func TestConverge_LocalAhead(t *testing.T) {
	var (
		tg   = &testGeth{}
		pg   = testpg(t)
		ig   = newTestIntegration()
		task = NewTask(0, 0, "one", 3, 1, tg, pg, 0, 0, ig)
	)
	tg.add(1, hash(1), hash(0))

	task.Insert(0, hash(0))
	task.Insert(1, hash(1))
	task.Insert(2, hash(2))

	diff.Test(t, t.Errorf, task.Converge(true), ErrAhead)
}
