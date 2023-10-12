package e2pg

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"sync"
	"testing"

	"github.com/indexsupply/x/eth"
	"github.com/indexsupply/x/geth"
	"github.com/indexsupply/x/tc"
	"github.com/indexsupply/x/wpg"

	"blake.io/pqx/pqxtest"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"kr.dev/diff"
)

func TestMain(m *testing.M) {
	sql.Register("postgres", stdlib.GetDefaultDriver())
	pqxtest.TestMain(m)
}

type testDestination struct {
	sync.Mutex
	chain map[uint64]eth.Block
}

func newTestDestination() *testDestination {
	return &testDestination{
		chain: make(map[uint64]eth.Block),
	}
}

func (dest *testDestination) blocks() []eth.Block {
	var blks []eth.Block
	for _, b := range dest.chain {
		blks = append(blks, b)
	}
	sort.Slice(blks, func(i, j int) bool {
		return blks[i].Num() < blks[j].Num()
	})
	return blks
}

func (dest *testDestination) Insert(_ context.Context, _ wpg.Conn, blocks []eth.Block) (int64, error) {
	dest.Lock()
	defer dest.Unlock()
	for _, b := range blocks {
		dest.chain[uint64(b.Header.Number)] = b
	}
	return int64(len(blocks)), nil
}

func (dest *testDestination) add(n uint64, hash, parent []byte) {
	dest.Insert(context.Background(), nil, []eth.Block{
		eth.Block{
			Header: eth.Header{
				Number: eth.Uint64(n),
				Hash:   hash,
				Parent: parent,
			},
		},
	})
}

func (dest *testDestination) Delete(_ context.Context, pg wpg.Conn, n uint64) error {
	dest.Lock()
	defer dest.Unlock()
	delete(dest.chain, n)
	return nil
}

func (dest *testDestination) Events(_ context.Context) [][]byte {
	return nil
}

type testGeth struct {
	blocks []eth.Block
}

func (tg *testGeth) ChainID() uint64 { return 0 }

func (tg *testGeth) Hash(n uint64) ([]byte, error) {
	for j := range tg.blocks {
		if uint64(tg.blocks[j].Header.Number) == n {
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

func (tg *testGeth) LoadBlocks(filter [][]byte, bufs []geth.Buffer, blks []eth.Block) error {
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
	tg.blocks = append(tg.blocks, eth.Block{
		Header: eth.Header{
			Number: eth.Uint64(n),
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
		task = NewTask(
			WithSource(tg),
			WithPG(pg),
			WithDestinations(newTestDestination()),
		)
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
		tg   = &testGeth{}
		pg   = testpg(t)
		task = NewTask(
			WithSource(tg),
			WithPG(pg),
			WithDestinations(newTestDestination()),
		)
	)
	diff.Test(t, t.Errorf, task.Converge(false), ErrNothingNew)
}

func TestConverge_EmptyDestination(t *testing.T) {
	var (
		pg   = testpg(t)
		tg   = &testGeth{}
		dest = newTestDestination()
		task = NewTask(
			WithSource(tg),
			WithPG(pg),
			WithDestinations(dest),
		)
	)
	tg.add(0, hash(0), hash(0))
	tg.add(1, hash(1), hash(0))
	dest.add(0, hash(0), hash(0))
	task.Insert(0, hash(0))
	diff.Test(t, t.Fatalf, task.Converge(true), nil)
	diff.Test(t, t.Errorf, dest.blocks(), tg.blocks)
}

func TestConverge_Reorg(t *testing.T) {
	var (
		pg   = testpg(t)
		tg   = &testGeth{}
		dest = newTestDestination()
		task = NewTask(
			WithSource(tg),
			WithPG(pg),
			WithDestinations(dest),
		)
	)

	tg.add(0, hash(0), hash(0))
	tg.add(1, hash(2), hash(0))
	tg.add(2, hash(3), hash(2))

	dest.add(0, hash(0), hash(0))
	dest.add(1, hash(2), hash(0))

	task.Insert(0, hash(0))
	task.Insert(1, hash(1))

	diff.Test(t, t.Fatalf, task.Converge(false), nil)
	diff.Test(t, t.Fatalf, task.Converge(false), nil)
	diff.Test(t, t.Errorf, dest.blocks(), tg.blocks)
}

func TestConverge_DeltaBatchSize(t *testing.T) {
	const (
		batchSize = 16
		workers   = 2
	)
	var (
		pg   = testpg(t)
		tg   = &testGeth{}
		dest = newTestDestination()
		task = NewTask(
			WithSource(tg),
			WithPG(pg),
			WithConcurrency(workers, batchSize),
			WithDestinations(dest),
		)
	)

	tg.add(0, hash(0), hash(0))
	dest.add(0, hash(0), hash(0))
	task.Insert(0, hash(0))

	for i := uint64(1); i <= batchSize+1; i++ {
		tg.add(i, hash(byte(i)), hash(byte(i-1)))
	}

	diff.Test(t, t.Errorf, nil, task.Converge(false))
	diff.Test(t, t.Errorf, dest.blocks(), tg.blocks[:batchSize+1])

	diff.Test(t, t.Errorf, nil, task.Converge(false))
	diff.Test(t, t.Errorf, dest.blocks(), tg.blocks)
}

func TestConverge_MultipleTasks(t *testing.T) {
	var (
		tg    = &testGeth{}
		pg    = testpg(t)
		dest1 = newTestDestination()
		dest2 = newTestDestination()
		task1 = NewTask(
			WithSource(tg),
			WithPG(pg),
			WithConcurrency(1, 3),
			WithDestinations(dest1),
		)
		task2 = NewTask(
			WithBackfillSource(tg, "foo"),
			WithPG(pg),
			WithConcurrency(1, 3),
			WithDestinations(dest2),
		)
	)
	tg.add(1, hash(1), hash(0))
	tg.add(2, hash(2), hash(1))

	task1.Insert(0, hash(0))
	task2.Insert(0, hash(0))

	diff.Test(t, t.Errorf, task1.Converge(true), nil)
	diff.Test(t, t.Errorf, dest1.blocks(), tg.blocks)
	diff.Test(t, t.Errorf, len(dest2.blocks()), 0)
	diff.Test(t, t.Errorf, task2.Converge(true), nil)
	diff.Test(t, t.Errorf, dest2.blocks(), tg.blocks)
}

func TestConverge_LocalAhead(t *testing.T) {
	var (
		tg   = &testGeth{}
		pg   = testpg(t)
		dest = newTestDestination()
		task = NewTask(
			WithSource(tg),
			WithPG(pg),
			WithConcurrency(1, 3),
			WithDestinations(dest),
		)
	)
	tg.add(1, hash(1), hash(0))

	task.Insert(0, hash(0))
	task.Insert(1, hash(1))
	task.Insert(2, hash(2))

	diff.Test(t, t.Errorf, task.Converge(true), ErrAhead)
}
