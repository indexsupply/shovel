package e2pg

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"

	"github.com/indexsupply/x/abi2"
	"github.com/indexsupply/x/eth"
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
	name  string
	chain map[uint64]eth.Block
}

func newTestDestination(name string) *testDestination {
	return &testDestination{
		name:  name,
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

func (dest *testDestination) Name() string {
	return dest.name
}

type testGeth struct {
	blocks []eth.Block
}

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

func (tg *testGeth) LoadBlocks(filter [][]byte, blks []eth.Block) error {
	for i := range blks {
		for j := range tg.blocks {
			if blks[i].Num() == tg.blocks[j].Num() {
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
			WithSourceConfig(SourceConfig{Name: "foo"}),
			WithSourceFactory(func(SourceConfig) Source { return tg }),
			WithPG(pg),
			WithDestinations(newTestDestination("foo")),
		)
	)
	tg.add(0, hash(0), hash(0))
	tg.add(1, hash(1), hash(0))
	tg.add(2, hash(2), hash(1))
	diff.Test(t, t.Errorf, task.Setup(), nil)

	checkQuery(t, pg, `
		select true
		from e2pg.task
		where src_name = 'foo'
		and hash = $1
		and num = $2
	`, hash(1), uint64(1))
}

func TestConverge_Zero(t *testing.T) {
	var (
		pg   = testpg(t)
		task = NewTask(
			WithSourceConfig(SourceConfig{Name: "foo"}),
			WithSourceFactory(func(SourceConfig) Source { return &testGeth{} }),
			WithPG(pg),
			WithDestinations(newTestDestination("foo")),
		)
	)
	diff.Test(t, t.Errorf, task.Converge(false), ErrNothingNew)
}

func TestConverge_EmptyDestination(t *testing.T) {
	var (
		pg   = testpg(t)
		tg   = &testGeth{}
		dest = newTestDestination("foo")
		task = NewTask(
			WithSourceConfig(SourceConfig{Name: "foo"}),
			WithSourceFactory(func(SourceConfig) Source { return tg }),
			WithPG(pg),
			WithDestinations(dest),
		)
	)
	tg.add(0, hash(0), hash(0))
	tg.add(1, hash(1), hash(0))
	dest.add(0, hash(0), hash(0))
	taskAdd(t, pg, "foo", 0, hash(0))
	diff.Test(t, t.Fatalf, task.Converge(true), nil)
	diff.Test(t, t.Errorf, dest.blocks(), tg.blocks)
}

func TestConverge_Reorg(t *testing.T) {
	var (
		pg   = testpg(t)
		tg   = &testGeth{}
		dest = newTestDestination("foo")
		task = NewTask(
			WithSourceConfig(SourceConfig{Name: "foo"}),
			WithSourceFactory(func(SourceConfig) Source { return tg }),
			WithPG(pg),
			WithDestinations(dest),
		)
	)

	tg.add(0, hash(0), hash(0))
	tg.add(1, hash(2), hash(0))
	tg.add(2, hash(3), hash(2))

	dest.add(0, hash(0), hash(0))
	dest.add(1, hash(2), hash(0))

	taskAdd(t, pg, "foo", 0, hash(0), dest.Name())
	taskAdd(t, pg, "foo", 1, hash(1), dest.Name())

	diff.Test(t, t.Fatalf, task.Converge(false), nil)
	diff.Test(t, t.Fatalf, task.Converge(false), nil)
	diff.Test(t, t.Errorf, dest.blocks(), tg.blocks)
}

func taskAdd(
	tb testing.TB,
	pg wpg.Conn,
	srcName string,
	n uint64,
	h []byte,
	intgs ...string,
) {
	ctx := context.Background()
	const q1 = `
		insert into e2pg.task(src_name, backfill, num, hash)
		values ($1, false, $2, $3)
	`
	_, err := pg.Exec(ctx, q1, srcName, n, h)
	if err != nil {
		tb.Fatalf("inserting task %d %.4x %s", n, h, err)
	}
	for i := range intgs {
		const q1 = `
			insert into e2pg.intg(name, src_name, backfill, num)
			values ($1, $2, false, $3)
		`
		_, err := pg.Exec(ctx, q1, intgs[i], srcName, n)
		if err != nil {
			tb.Fatalf("inserting task %d %.4x %s", n, h, err)
		}
	}
}

func TestConverge_DeltaBatchSize(t *testing.T) {
	const (
		batchSize = 16
		workers   = 2
	)
	var (
		pg   = testpg(t)
		tg   = &testGeth{}
		dest = newTestDestination("foo")
		task = NewTask(
			WithSourceConfig(SourceConfig{Name: "foo"}),
			WithSourceFactory(func(SourceConfig) Source { return tg }),
			WithPG(pg),
			WithConcurrency(workers, batchSize),
			WithDestinations(dest),
		)
	)

	tg.add(0, hash(0), hash(0))
	dest.add(0, hash(0), hash(0))
	taskAdd(t, pg, "foo", 0, hash(0))

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
		dest1 = newTestDestination("foo")
		dest2 = newTestDestination("bar")
		task1 = NewTask(
			WithSourceConfig(SourceConfig{Name: "a"}),
			WithSourceFactory(func(SourceConfig) Source { return tg }),
			WithPG(pg),
			WithConcurrency(1, 3),
			WithDestinations(dest1),
		)
		task2 = NewTask(
			WithSourceConfig(SourceConfig{Name: "b"}),
			WithSourceFactory(func(SourceConfig) Source { return tg }),
			WithPG(pg),
			WithConcurrency(1, 3),
			WithDestinations(dest2),
		)
	)
	tg.add(1, hash(1), hash(0))
	tg.add(2, hash(2), hash(1))

	taskAdd(t, pg, "a", 0, hash(0))
	taskAdd(t, pg, "b", 0, hash(0))

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
		dest = newTestDestination("foo")
		task = NewTask(
			WithSourceConfig(SourceConfig{Name: "foo"}),
			WithSourceFactory(func(SourceConfig) Source { return tg }),
			WithPG(pg),
			WithConcurrency(1, 3),
			WithDestinations(dest),
		)
	)
	tg.add(1, hash(1), hash(0))

	taskAdd(t, pg, "foo", 0, hash(0))
	taskAdd(t, pg, "foo", 1, hash(1))
	taskAdd(t, pg, "foo", 2, hash(2))

	diff.Test(t, t.Errorf, task.Converge(true), ErrAhead)
}

func TestConverge_Done(t *testing.T) {
	var (
		ctx  = context.Background()
		pg   = testpg(t)
		tg   = &testGeth{}
		task = NewTask(
			WithSourceConfig(SourceConfig{Name: "foo"}),
			WithSourceFactory(func(SourceConfig) Source { return tg }),
			WithPG(pg),
			WithConcurrency(1, 1),
			WithDestinations(newTestDestination("foo")),
		)
		taskBF = NewTask(
			WithBackfill(true),
			WithSourceConfig(SourceConfig{Name: "foo"}),
			WithSourceFactory(func(SourceConfig) Source { return tg }),
			WithPG(pg),
			WithConcurrency(1, 1),
			WithDestinations(newTestDestination("foo")),
		)
	)
	diff.Test(t, t.Errorf, nil, task.initRows(0, hash(0)))

	tg.add(1, hash(1), hash(0))
	tg.add(2, hash(2), hash(1))

	diff.Test(t, t.Errorf, nil, task.Converge(true))

	// manually setup the first record.
	// this is typically done in loadTasks
	const q = `
		insert into e2pg.intg(name, src_name, backfill, num)
		values ($1, $2, $3, $4)
	`
	_, err := pg.Exec(ctx, q, "foo", "foo", true, 0)
	diff.Test(t, t.Errorf, nil, err)
	diff.Test(t, t.Errorf, nil, taskBF.destRanges[0].load(ctx, pg, "foo", "foo"))
	diff.Test(t, t.Errorf, ErrDone, taskBF.Converge(true))
}

func checkQuery(tb testing.TB, pg wpg.Conn, query string, args ...any) {
	var found bool
	err := pg.QueryRow(context.Background(), query, args...).Scan(&found)
	diff.Test(tb, tb.Fatalf, err, nil)
	if !found {
		tb.Errorf("query\n%s\nreturned false", query)
	}
}

func TestPruneTask(t *testing.T) {
	pg := testpg(t)
	it := func(n uint8) {
		_, err := pg.Exec(context.Background(), `
			insert into e2pg.task(src_name, backfill, num, hash)
			values ($1, false, $2, $3)
		`, "foo", n, hash(n))
		if err != nil {
			t.Fatalf("inserting task: %d", n)
		}
	}
	for i := uint8(0); i < 10; i++ {
		it(i)
	}
	checkQuery(t, pg, `select count(*) = 10 from e2pg.task`)
	PruneTask(context.Background(), pg, 1)
	checkQuery(t, pg, `select count(*) = 1 from e2pg.task`)
}

func TestPruneIntg(t *testing.T) {
	ctx := context.Background()

	pqxtest.CreateDB(t, Schema)
	pg, err := pgxpool.New(ctx, pqxtest.DSNForTest(t))
	diff.Test(t, t.Fatalf, err, nil)

	iub := newIUB(2)
	iub.update(0, "foo", "bar", true, 1, 0, 0, 0)
	err = iub.write(ctx, pg)
	diff.Test(t, t.Fatalf, err, nil)
	checkQuery(t, pg, `select count(*) = 1 from e2pg.intg`)

	for i := 0; i < 10; i++ {
		iub.update(0, "foo", "bar", true, uint64(i+2), 0, 0, 0)
		err := iub.write(ctx, pg)
		diff.Test(t, t.Fatalf, err, nil)
	}
	checkQuery(t, pg, `select count(*) = 11 from e2pg.intg`)
	err = PruneIntg(ctx, pg)
	diff.Test(t, t.Fatalf, err, nil)
	checkQuery(t, pg, `select count(*) = 2 from e2pg.intg`)

	iub.update(1, "foo", "baz", true, 1, 0, 0, 0)
	err = iub.write(ctx, pg)
	diff.Test(t, t.Fatalf, err, nil)
	checkQuery(t, pg, `select count(*) = 1 from e2pg.intg where src_name = 'baz'`)
	checkQuery(t, pg, `select count(*) = 3 from e2pg.intg`)

	err = PruneIntg(ctx, pg)
	diff.Test(t, t.Fatalf, err, nil)
	checkQuery(t, pg, `select count(*) = 3 from e2pg.intg`)
}

func TestInitRows(t *testing.T) {
	ctx := context.Background()
	pqxtest.CreateDB(t, Schema)
	pg, err := pgxpool.New(ctx, pqxtest.DSNForTest(t))
	diff.Test(t, t.Fatalf, err, nil)

	task := NewTask(
		WithPG(pg),
		WithSourceConfig(SourceConfig{Name: "foo"}),
		WithSourceFactory(func(SourceConfig) Source { return &testGeth{} }),
		WithDestinations(newTestDestination("bar")),
	)
	err = task.initRows(42, hash(42))
	diff.Test(t, t.Fatalf, err, nil)
	checkQuery(t, pg, `select count(*) = 1 from e2pg.intg`)
	checkQuery(t, pg, `select count(*) = 1 from e2pg.task`)

	err = task.initRows(42, hash(42))
	diff.Test(t, t.Fatalf, err, nil)
	checkQuery(t, pg, `select count(*) = 1 from e2pg.intg`)
	checkQuery(t, pg, `select count(*) = 1 from e2pg.task`)

	task = NewTask(
		WithPG(pg),
		WithSourceConfig(SourceConfig{Name: "foo"}),
		WithSourceFactory(func(SourceConfig) Source { return &testGeth{} }),
		WithDestinations(newTestDestination("bar"), newTestDestination("baz")),
	)
	err = task.initRows(42, hash(42))
	diff.Test(t, t.Fatalf, err, nil)
	checkQuery(t, pg, `select count(*) = 2 from e2pg.intg`)
	checkQuery(t, pg, `select count(*) = 1 from e2pg.task`)

	task = NewTask(
		WithPG(pg),
		WithBackfill(true),
		WithSourceConfig(SourceConfig{Name: "foo"}),
		WithSourceFactory(func(SourceConfig) Source { return &testGeth{} }),
		WithDestinations(newTestDestination("bar"), newTestDestination("baz")),
	)
	err = task.initRows(42, hash(42))
	diff.Test(t, t.Fatalf, err, nil)
	checkQuery(t, pg, `select count(*) = 4 from e2pg.intg`)
	checkQuery(t, pg, `select count(*) = 2 from e2pg.task`)
	checkQuery(t, pg, `
		select count(*) = 1
		from e2pg.task
		where src_name = 'foo'
		and backfill = false
	`)
	checkQuery(t, pg, `
		select count(*) = 1
		from e2pg.task
		where src_name = 'foo'
		and backfill = true
	`)
	checkQuery(t, pg, `
		select count(*) = 1
		from e2pg.intg
		where name = 'bar'
		and src_name = 'foo'
		and backfill = true
	`)
	checkQuery(t, pg, `
		select count(*) = 1
		from e2pg.intg
		where name = 'bar'
		and src_name = 'foo'
		and backfill = false
	`)
	checkQuery(t, pg, `
		select count(*) = 1
		from e2pg.intg
		where name = 'baz'
		and src_name = 'foo'
		and backfill = true
	`)
	checkQuery(t, pg, `
		select count(*) = 1
		from e2pg.intg
		where name = 'baz'
		and src_name = 'foo'
		and backfill = false
	`)
}

func TestDestRanges_Load(t *testing.T) {
	ctx := context.Background()
	pqxtest.CreateDB(t, Schema)
	pg, err := pgxpool.New(ctx, pqxtest.DSNForTest(t))
	diff.Test(t, t.Fatalf, err, nil)

	task1 := NewTask(
		WithPG(pg),
		WithSourceConfig(SourceConfig{Name: "foo"}),
		WithSourceFactory(func(SourceConfig) Source { return &testGeth{} }),
		WithDestinations(newTestDestination("bar")),
	)
	task2 := NewTask(
		WithPG(pg),
		WithBackfill(true),
		WithSourceConfig(SourceConfig{Name: "foo"}),
		WithSourceFactory(func(SourceConfig) Source { return &testGeth{} }),
		WithDestinations(newTestDestination("bar")),
	)
	err = task1.initRows(42, hash(42))
	diff.Test(t, t.Fatalf, err, nil)
	err = task2.initRows(10, hash(10))
	diff.Test(t, t.Fatalf, err, nil)

	diff.Test(t, t.Fatalf, len(task2.destRanges), 1)
	err = task2.destRanges[0].load(ctx, pg, "bar", "foo")
	diff.Test(t, t.Fatalf, err, nil)
	diff.Test(t, t.Errorf, task2.destRanges[0].start, uint64(10))
	diff.Test(t, t.Errorf, task2.destRanges[0].stop, uint64(42))
}

func TestDestRanges_Filter(t *testing.T) {
	br := func(l, h uint64) (res []eth.Block) {
		for i := l; i <= h; i++ {
			res = append(res, eth.Block{Header: eth.Header{Number: eth.Uint64(i)}})
		}
		return
	}
	cases := []struct {
		desc  string
		input []eth.Block
		r     destRange
		want  []eth.Block
	}{
		{
			desc:  "empty input",
			input: []eth.Block{},
			r:     destRange{},
			want:  []eth.Block{},
		},
		{
			desc: "empty range",
			input: []eth.Block{
				eth.Block{Header: eth.Header{Number: 42}},
				eth.Block{Header: eth.Header{Number: 43}},
			},
			r: destRange{},
			want: []eth.Block{
				eth.Block{Header: eth.Header{Number: 42}},
				eth.Block{Header: eth.Header{Number: 43}},
			},
		},
		{
			desc:  "[0, 10] -> [1,9]",
			input: br(0, 10),
			r:     destRange{start: 1, stop: 9},
			want:  br(1, 9),
		},
		{
			desc:  "[0, 10] -> [0,5]",
			input: br(0, 10),
			r:     destRange{start: 0, stop: 5},
			want:  br(0, 5),
		},
		{
			desc:  "[0, 10] -> [5,10]",
			input: br(0, 10),
			r:     destRange{start: 5, stop: 10},
			want:  br(5, 10),
		},
		{
			desc:  "[0, 10] -> [10, 15]",
			input: br(0, 10),
			r:     destRange{start: 10, stop: 15},
			want:  br(10, 10),
		},
		{
			desc:  "[0, 10] -> [10, 10]",
			input: br(0, 10),
			r:     destRange{start: 10, stop: 10},
			want:  br(10, 10),
		},
		{
			desc:  "[0, 10] -> [15, 10]",
			input: br(0, 10),
			r:     destRange{start: 15, stop: 10},
			want:  []eth.Block(nil),
		},
	}
	for _, tc := range cases {
		got := tc.r.filter(tc.input)
		diff.Test(t, t.Errorf, got, tc.want)
	}
}

func TestLoadTasks(t *testing.T) {
	ctx := context.Background()
	pqxtest.CreateDB(t, Schema)
	pg, err := pgxpool.New(ctx, pqxtest.DSNForTest(t))
	diff.Test(t, t.Fatalf, err, nil)

	conf := Config{
		PGURL: pqxtest.DSNForTest(t),
		SourceConfigs: []SourceConfig{
			SourceConfig{
				Name:    "foo",
				ChainID: 888,
				URL:     "http://foo",
			},
		},
		Integrations: []Integration{
			Integration{
				Enabled: true,
				Name:    "bar",
				Table: abi2.Table{
					Name: "bar",
					Cols: []abi2.Column{
						abi2.Column{Name: "block_hash", Type: "bytea"},
					},
				},
				Block: []abi2.BlockData{
					abi2.BlockData{
						Name:   "block_hash",
						Column: "block_hash",
					},
				},
				SourceConfigs: []SourceConfig{
					SourceConfig{Name: "foo"},
				},
			},
		},
	}
	tasks, err := loadTasks(ctx, pg, conf)
	diff.Test(t, t.Fatalf, err, nil)
	diff.Test(t, t.Fatalf, len(tasks), 1)

	diff.Test(t, t.Fatalf, tasks[0].backfill, false)
	diff.Test(t, t.Fatalf, tasks[0].start, uint64(0))
}

func TestLoadTasks_Backfill(t *testing.T) {
	ctx := context.Background()
	pqxtest.CreateDB(t, Schema)
	pg, err := pgxpool.New(ctx, pqxtest.DSNForTest(t))
	diff.Test(t, t.Fatalf, err, nil)

	conf := Config{
		PGURL: pqxtest.DSNForTest(t),
		SourceConfigs: []SourceConfig{
			SourceConfig{
				Name:    "foo",
				ChainID: 888,
				URL:     "http://foo",
			},
		},
		Integrations: []Integration{
			Integration{
				Enabled: true,
				Name:    "bar",
				Table: abi2.Table{
					Name: "bar",
					Cols: []abi2.Column{
						abi2.Column{Name: "block_hash", Type: "bytea"},
					},
				},
				Block: []abi2.BlockData{
					abi2.BlockData{
						Name:   "block_hash",
						Column: "block_hash",
					},
				},
				SourceConfigs: []SourceConfig{
					SourceConfig{Name: "foo", Start: 42},
				},
			},
			Integration{
				Enabled: true,
				Name:    "baz",
				Table: abi2.Table{
					Name: "baz",
					Cols: []abi2.Column{
						abi2.Column{Name: "block_hash", Type: "bytea"},
					},
				},
				Block: []abi2.BlockData{
					abi2.BlockData{
						Name:   "block_hash",
						Column: "block_hash",
					},
				},
				SourceConfigs: []SourceConfig{
					SourceConfig{Name: "foo", Start: 41},
				},
			},
		},
	}
	tasks, err := loadTasks(ctx, pg, conf)
	diff.Test(t, t.Fatalf, err, nil)
	diff.Test(t, t.Fatalf, len(tasks), 2)

	diff.Test(t, t.Fatalf, tasks[0].backfill, false)
	diff.Test(t, t.Fatalf, tasks[0].start, uint64(0))

	diff.Test(t, t.Fatalf, tasks[1].backfill, true)
	diff.Test(t, t.Fatalf, tasks[1].start, uint64(41))
}

func TestValidateChain(t *testing.T) {
	cases := []struct {
		parent []byte
		blks   []eth.Block
		want   error
	}{
		{
			[]byte{},
			[]eth.Block{
				{Header: eth.Header{}},
			},
			errors.New("corrupt parent: "),
		},
		{
			hash(0),
			[]eth.Block{
				{Header: eth.Header{Hash: hash(1), Parent: hash(0)}},
			},
			nil,
		},
		{
			hash(0),
			[]eth.Block{
				{Header: eth.Header{Hash: hash(2), Parent: hash(1)}},
			},
			ErrReorg,
		},
		{
			hash(0),
			[]eth.Block{
				{Header: eth.Header{Hash: hash(1), Parent: hash(0)}},
				{Header: eth.Header{Hash: hash(2), Parent: hash(1)}},
			},
			nil,
		},
		{
			hash(0),
			[]eth.Block{
				{Header: eth.Header{Hash: hash(1), Parent: hash(0)}},
				{Header: eth.Header{Hash: hash(2), Parent: hash(1)}},
				{Header: eth.Header{Hash: hash(3), Parent: hash(2)}},
			},
			nil,
		},
		{
			hash(0),
			[]eth.Block{
				{Header: eth.Header{Hash: hash(1), Parent: hash(0)}},
				{Header: eth.Header{Hash: hash(4), Parent: hash(3)}},
				{Header: eth.Header{Hash: hash(3), Parent: hash(2)}},
			},
			errors.New("corrupt chain segment"),
		},
	}
	for _, tc := range cases {
		diff.Test(t, t.Errorf, tc.want, validateChain(context.Background(), tc.parent, tc.blks))
	}
}

func TestParts(t *testing.T) {
	cases := []struct {
		numParts  int
		batchSize int
		want      []part
	}{
		{
			1,
			1,
			[]part{part{m: 0, n: 1}},
		},
		{
			1,
			3,
			[]part{part{m: 0, n: 3}},
		},
		{
			3,
			3,
			[]part{
				part{m: 0, n: 1},
				part{m: 1, n: 2},
				part{m: 2, n: 3},
			},
		},
		{
			3,
			10,
			[]part{
				part{m: 0, n: 3},
				part{m: 3, n: 6},
				part{m: 6, n: 10},
			},
		},
		{
			5,
			42,
			[]part{
				part{m: 0, n: 8},
				part{m: 8, n: 16},
				part{m: 16, n: 24},
				part{m: 24, n: 32},
				part{m: 32, n: 42},
			},
		},
	}
	for _, tc := range cases {
		got := parts(tc.numParts, tc.batchSize)
		diff.Test(t, t.Errorf, tc.want, got)
	}
}

func TestLockID(t *testing.T) {
	cases := []struct {
		chid     uint64
		backfill bool
		want     uint32
	}{
		{1, true, 0b00000000000000000000000000000011},
		{1, false, 0b00000000000000000000000000000010},
		{1024, true, 0b00000000000000000000100000000001},
		{1024, false, 0b00000000000000000000100000000000},
	}
	for _, tc := range cases {
		got := lockid(tc.chid, tc.backfill)
		diff.Test(t, t.Errorf, tc.want, got)
	}
}
