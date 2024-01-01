package shovel

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"

	"github.com/indexsupply/x/dig"
	"github.com/indexsupply/x/eth"
	"github.com/indexsupply/x/shovel/config"
	"github.com/indexsupply/x/shovel/glf"
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

func (dest *testDestination) factory(ig config.Integration) (Destination, error) {
	return dest, nil
}

func (dest *testDestination) ig() config.Integration {
	return config.Integration{Name: dest.name, Enabled: true}
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

func (dest *testDestination) Filter() glf.Filter {
	return glf.Filter{UseBlocks: true, UseLogs: true}
}

func (dest *testDestination) Name() string {
	return dest.name
}

type testGeth struct {
	blocks []eth.Block
}

func (tg *testGeth) factory(config.Source, glf.Filter) Source {
	return tg
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

func checkQuery(tb testing.TB, pg wpg.Conn, query string, args ...any) {
	var found bool
	err := pg.QueryRow(context.Background(), query, args...).Scan(&found)
	diff.Test(tb, tb.Fatalf, err, nil)
	if !found {
		tb.Errorf("query\n%s\nreturned false", query)
	}
}

func taskAdd(
	tb testing.TB,
	pg wpg.Conn,
	srcName string,
	n uint64,
	h []byte,
	igs ...string,
) {
	ctx := context.Background()
	const q1 = `
		insert into shovel.task_updates(src_name, backfill, num, hash)
		values ($1, false, $2, $3)
	`
	_, err := pg.Exec(ctx, q1, srcName, n, h)
	if err != nil {
		tb.Fatalf("inserting task %d %.4x %s", n, h, err)
	}
	for i := range igs {
		const q1 = `
			insert into shovel.ig_updates(name, src_name, backfill, num)
			values ($1, $2, false, $3)
		`
		_, err := pg.Exec(ctx, q1, igs[i], srcName, n)
		if err != nil {
			tb.Fatalf("inserting task %d %.4x %s", n, h, err)
		}
	}
}

func TestSetup(t *testing.T) {
	var (
		tg        = &testGeth{}
		pg        = testpg(t)
		dest      = newTestDestination("foo")
		task, err = NewTask(
			WithPG(pg),
			WithSourceConfig(config.Source{Name: "foo"}),
			WithSourceFactory(tg.factory),
			WithIntegrations(dest.ig()),
			WithIntegrationFactory(dest.factory),
		)
	)
	diff.Test(t, t.Errorf, err, nil)
	tg.add(0, hash(0), hash(0))
	tg.add(1, hash(1), hash(0))
	tg.add(2, hash(2), hash(1))
	diff.Test(t, t.Errorf, task.Setup(), nil)

	checkQuery(t, pg, `
		select true
		from shovel.task_updates
		where src_name = 'foo'
		and hash = $1
		and num = $2
	`, hash(1), uint64(1))
}

func TestConverge_Zero(t *testing.T) {
	t.Skip()
	var (
		tg        = &testGeth{}
		pg        = testpg(t)
		dest      = newTestDestination("foo")
		task, err = NewTask(
			WithPG(pg),
			WithSourceConfig(config.Source{Name: "foo"}),
			WithSourceFactory(tg.factory),
			WithIntegrations(dest.ig()),
			WithIntegrationFactory(dest.factory),
		)
	)
	diff.Test(t, t.Errorf, err, nil)
	diff.Test(t, t.Errorf, task.Converge(false), ErrNothingNew)
}

func TestConverge_EmptyDestination(t *testing.T) {
	var (
		pg        = testpg(t)
		tg        = &testGeth{}
		dest      = newTestDestination("foo")
		task, err = NewTask(
			WithPG(pg),
			WithSourceConfig(config.Source{Name: "foo"}),
			WithSourceFactory(tg.factory),
			WithIntegrations(dest.ig()),
			WithIntegrationFactory(dest.factory),
		)
	)
	diff.Test(t, t.Errorf, err, nil)
	tg.add(0, hash(0), hash(0))
	tg.add(1, hash(1), hash(0))
	dest.add(0, hash(0), hash(0))
	taskAdd(t, pg, "foo", 0, hash(0))
	diff.Test(t, t.Fatalf, task.Converge(true), nil)
	diff.Test(t, t.Errorf, dest.blocks(), tg.blocks)
}

func TestConverge_Reorg(t *testing.T) {
	var (
		pg        = testpg(t)
		tg        = &testGeth{}
		dest      = newTestDestination("foo")
		task, err = NewTask(
			WithPG(pg),
			WithSourceConfig(config.Source{Name: "foo"}),
			WithSourceFactory(tg.factory),
			WithIntegrations(dest.ig()),
			WithIntegrationFactory(dest.factory),
		)
	)
	diff.Test(t, t.Errorf, err, nil)
	task.filter = glf.Filter{UseBlocks: true}

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

func TestConverge_DeltaBatchSize(t *testing.T) {
	const (
		batchSize = 16
		workers   = 2
	)
	var (
		pg        = testpg(t)
		tg        = &testGeth{}
		dest      = newTestDestination("foo")
		task, err = NewTask(
			WithPG(pg),
			WithConcurrency(workers, batchSize),
			WithSourceConfig(config.Source{Name: "foo"}),
			WithSourceFactory(tg.factory),
			WithIntegrations(dest.ig()),
			WithIntegrationFactory(dest.factory),
		)
	)
	diff.Test(t, t.Errorf, err, nil)

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
		tg          = &testGeth{}
		pg          = testpg(t)
		dest1       = newTestDestination("foo")
		dest2       = newTestDestination("bar")
		task1, err1 = NewTask(
			WithPG(pg),
			WithConcurrency(1, 3),
			WithSourceConfig(config.Source{Name: "a"}),
			WithSourceFactory(tg.factory),
			WithIntegrations(dest1.ig()),
			WithIntegrationFactory(dest1.factory),
		)
		task2, err2 = NewTask(
			WithPG(pg),
			WithConcurrency(1, 3),
			WithSourceConfig(config.Source{Name: "b"}),
			WithSourceFactory(tg.factory),
			WithIntegrations(dest2.ig()),
			WithIntegrationFactory(dest2.factory),
		)
	)
	diff.Test(t, t.Errorf, err1, nil)
	diff.Test(t, t.Errorf, err2, nil)
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
		tg        = &testGeth{}
		pg        = testpg(t)
		dest      = newTestDestination("foo")
		task, err = NewTask(
			WithPG(pg),
			WithConcurrency(1, 3),
			WithSourceConfig(config.Source{Name: "foo"}),
			WithSourceFactory(tg.factory),
			WithIntegrations(dest.ig()),
			WithIntegrationFactory(dest.factory),
		)
	)
	diff.Test(t, t.Errorf, err, nil)
	tg.add(1, hash(1), hash(0))

	taskAdd(t, pg, "foo", 0, hash(0))
	taskAdd(t, pg, "foo", 1, hash(1))
	taskAdd(t, pg, "foo", 2, hash(2))

	diff.Test(t, t.Errorf, task.Converge(true), ErrAhead)
}

func TestConverge_Done(t *testing.T) {
	var (
		ctx        = context.Background()
		pg         = testpg(t)
		tg         = &testGeth{}
		dest       = newTestDestination("foo")
		task, err1 = NewTask(
			WithPG(pg),
			WithConcurrency(1, 1),
			WithSourceConfig(config.Source{Name: "foo"}),
			WithSourceFactory(tg.factory),
			WithIntegrations(dest.ig()),
			WithIntegrationFactory(dest.factory),
		)
		taskBF, err2 = NewTask(
			WithBackfill(true),
			WithPG(pg),
			WithConcurrency(1, 1),
			WithSourceConfig(config.Source{Name: "foo"}),
			WithSourceFactory(tg.factory),
			WithIntegrations(dest.ig()),
			WithIntegrationFactory(dest.factory),
		)
	)
	diff.Test(t, t.Errorf, err1, nil)
	diff.Test(t, t.Errorf, err2, nil)
	diff.Test(t, t.Errorf, nil, task.initRows(0, hash(0)))

	tg.add(1, hash(1), hash(0))
	tg.add(2, hash(2), hash(1))

	diff.Test(t, t.Errorf, nil, task.Converge(true))

	// manually setup the first record.
	// this is typically done in loadTasks
	const q = `
		insert into shovel.ig_updates(name, src_name, backfill, num)
		values ($1, $2, $3, $4)
	`
	_, err := pg.Exec(ctx, q, "foo", "foo", true, 0)
	diff.Test(t, t.Errorf, nil, err)
	diff.Test(t, t.Errorf, nil, taskBF.igRange[0].load(ctx, pg, "foo", "foo"))
	diff.Test(t, t.Errorf, ErrDone, taskBF.Converge(true))
}

func TestPruneTask(t *testing.T) {
	pg := testpg(t)
	it := func(n uint8) {
		_, err := pg.Exec(context.Background(), `
			insert into shovel.task_updates(src_name, backfill, num, hash)
			values ($1, false, $2, $3)
		`, "foo", n, hash(n))
		if err != nil {
			t.Fatalf("inserting task: %d", n)
		}
	}
	for i := uint8(0); i < 10; i++ {
		it(i)
	}
	checkQuery(t, pg, `select count(*) = 10 from shovel.task_updates`)
	PruneTask(context.Background(), pg, 1)
	checkQuery(t, pg, `select count(*) = 1 from shovel.task_updates`)
}

func TestPruneIG(t *testing.T) {
	ctx := context.Background()

	pqxtest.CreateDB(t, Schema)
	pg, err := pgxpool.New(ctx, pqxtest.DSNForTest(t))
	diff.Test(t, t.Fatalf, err, nil)

	igUpdateBuf := newIGUpdateBuf(2)
	igUpdateBuf.update(0, "foo", "bar", true, 1, 0, 0, 0)
	err = igUpdateBuf.write(ctx, pg)
	diff.Test(t, t.Fatalf, err, nil)
	checkQuery(t, pg, `select count(*) = 1 from shovel.ig_updates`)

	for i := 0; i < 10; i++ {
		igUpdateBuf.update(0, "foo", "bar", true, uint64(i+2), 0, 0, 0)
		err := igUpdateBuf.write(ctx, pg)
		diff.Test(t, t.Fatalf, err, nil)
	}
	checkQuery(t, pg, `select count(*) = 11 from shovel.ig_updates`)
	err = PruneIG(ctx, pg)
	diff.Test(t, t.Fatalf, err, nil)
	checkQuery(t, pg, `select count(*) = 2 from shovel.ig_updates`)

	igUpdateBuf.update(1, "foo", "baz", true, 1, 0, 0, 0)
	err = igUpdateBuf.write(ctx, pg)
	diff.Test(t, t.Fatalf, err, nil)
	checkQuery(t, pg, `select count(*) = 1 from shovel.ig_updates where src_name = 'baz'`)
	checkQuery(t, pg, `select count(*) = 3 from shovel.ig_updates`)

	err = PruneIG(ctx, pg)
	diff.Test(t, t.Fatalf, err, nil)
	checkQuery(t, pg, `select count(*) = 3 from shovel.ig_updates`)
}

func destFactory(dests ...*testDestination) func(config.Integration) (Destination, error) {
	return func(ig config.Integration) (Destination, error) {
		for i := range dests {
			if dests[i].name == ig.Name {
				return dests[i], nil
			}
		}
		return nil, fmt.Errorf("dest not found %s", ig.Name)
	}
}

func TestInitRows(t *testing.T) {
	ctx := context.Background()
	pqxtest.CreateDB(t, Schema)
	pg, err := pgxpool.New(ctx, pqxtest.DSNForTest(t))
	diff.Test(t, t.Fatalf, err, nil)

	var (
		tg  = &testGeth{}
		bar = newTestDestination("bar")
		baz = newTestDestination("baz")
	)
	task, err := NewTask(
		WithPG(pg),
		WithSourceConfig(config.Source{Name: "foo"}),
		WithSourceFactory(tg.factory),
		WithIntegrations(bar.ig()),
		WithIntegrationFactory(bar.factory),
	)
	diff.Test(t, t.Errorf, err, nil)
	err = task.initRows(42, hash(42))
	diff.Test(t, t.Fatalf, err, nil)
	checkQuery(t, pg, `select count(*) = 1 from shovel.ig_updates`)
	checkQuery(t, pg, `select count(*) = 1 from shovel.task_updates`)

	err = task.initRows(42, hash(42))
	diff.Test(t, t.Fatalf, err, nil)
	checkQuery(t, pg, `select count(*) = 1 from shovel.ig_updates`)
	checkQuery(t, pg, `select count(*) = 1 from shovel.task_updates`)

	task, err = NewTask(
		WithPG(pg),
		WithSourceConfig(config.Source{Name: "foo"}),
		WithSourceFactory(tg.factory),
		WithIntegrations(bar.ig(), baz.ig()),
		WithIntegrationFactory(destFactory(bar, baz)),
	)
	diff.Test(t, t.Errorf, err, nil)

	err = task.initRows(42, hash(42))
	diff.Test(t, t.Fatalf, err, nil)
	checkQuery(t, pg, `select count(*) = 2 from shovel.ig_updates`)
	checkQuery(t, pg, `select count(*) = 1 from shovel.task_updates`)

	task, err = NewTask(
		WithPG(pg),
		WithBackfill(true),
		WithSourceConfig(config.Source{Name: "foo"}),
		WithSourceFactory(tg.factory),
		WithIntegrations(bar.ig(), baz.ig()),
		WithIntegrationFactory(destFactory(bar, baz)),
	)
	diff.Test(t, t.Errorf, err, nil)

	err = task.initRows(42, hash(42))
	diff.Test(t, t.Fatalf, err, nil)
	checkQuery(t, pg, `select count(*) = 4 from shovel.ig_updates`)
	checkQuery(t, pg, `select count(*) = 2 from shovel.task_updates`)
	checkQuery(t, pg, `
		select count(*) = 1
		from shovel.task_updates
		where src_name = 'foo'
		and backfill = false
	`)
	checkQuery(t, pg, `
		select count(*) = 1
		from shovel.task_updates
		where src_name = 'foo'
		and backfill = true
	`)
	checkQuery(t, pg, `
		select count(*) = 1
		from shovel.ig_updates
		where name = 'bar'
		and src_name = 'foo'
		and backfill = true
	`)
	checkQuery(t, pg, `
		select count(*) = 1
		from shovel.ig_updates
		where name = 'bar'
		and src_name = 'foo'
		and backfill = false
	`)
	checkQuery(t, pg, `
		select count(*) = 1
		from shovel.ig_updates
		where name = 'baz'
		and src_name = 'foo'
		and backfill = true
	`)
	checkQuery(t, pg, `
		select count(*) = 1
		from shovel.ig_updates
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

	var (
		tg   = &testGeth{}
		dest = newTestDestination("bar")
	)
	task1, err1 := NewTask(
		WithPG(pg),
		WithSourceConfig(config.Source{Name: "foo"}),
		WithSourceFactory(tg.factory),
		WithIntegrations(dest.ig()),
		WithIntegrationFactory(dest.factory),
	)
	task2, err2 := NewTask(
		WithPG(pg),
		WithBackfill(true),
		WithSourceConfig(config.Source{Name: "foo"}),
		WithSourceFactory(tg.factory),
		WithIntegrations(dest.ig()),
		WithIntegrationFactory(dest.factory),
	)
	diff.Test(t, t.Errorf, err1, nil)
	diff.Test(t, t.Errorf, err2, nil)

	err = task1.initRows(42, hash(42))
	diff.Test(t, t.Fatalf, err, nil)
	err = task2.initRows(10, hash(10))
	diff.Test(t, t.Fatalf, err, nil)

	diff.Test(t, t.Fatalf, len(task2.igRange), 1)
	err = task2.igRange[0].load(ctx, pg, "bar", "foo")
	diff.Test(t, t.Fatalf, err, nil)
	diff.Test(t, t.Errorf, task2.igRange[0].start, uint64(10))
	diff.Test(t, t.Errorf, task2.igRange[0].stop, uint64(42))
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
		r     igRange
		want  []eth.Block
	}{
		{
			desc:  "empty input",
			input: []eth.Block{},
			r:     igRange{},
			want:  []eth.Block{},
		},
		{
			desc:  "empty range",
			input: br(0, 10),
			r:     igRange{},
			want:  br(0, 10),
		},
		{
			desc:  "[0, 10] -> [1,9]",
			input: br(0, 10),
			r:     igRange{start: 1, stop: 9},
			want:  br(1, 9),
		},
		{
			desc:  "[0, 10] -> [0,5]",
			input: br(0, 10),
			r:     igRange{start: 0, stop: 5},
			want:  br(0, 5),
		},
		{
			desc:  "[0, 10] -> [5,10]",
			input: br(0, 10),
			r:     igRange{start: 5, stop: 10},
			want:  br(5, 10),
		},
		{
			desc:  "[0, 10] -> [10, 15]",
			input: br(0, 10),
			r:     igRange{start: 10, stop: 15},
			want:  []eth.Block(nil),
		},
		{
			desc:  "[0, 10] -> [10, 10]",
			input: br(0, 10),
			r:     igRange{start: 10, stop: 10},
			want:  []eth.Block(nil),
		},
		{
			desc:  "[10, 10] -> [10, 15]",
			input: br(10, 10),
			r:     igRange{start: 10, stop: 10},
			want:  []eth.Block(nil),
		},
		{
			desc:  "[0, 10] -> [15, 20]",
			input: br(0, 10),
			r:     igRange{start: 15, stop: 10},
			want:  []eth.Block(nil),
		},
		{
			desc:  "[0, 10] -> [15, 10]",
			input: br(0, 10),
			r:     igRange{start: 15, stop: 10},
			want:  []eth.Block(nil),
		},
		{
			desc:  "[0, 10] -> [5, 0]",
			input: br(0, 10),
			r:     igRange{start: 5, stop: 0},
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

	conf := config.Root{
		PGURL: pqxtest.DSNForTest(t),
		Sources: []config.Source{
			config.Source{
				Name:    "foo",
				ChainID: 888,
				URL:     "http://foo",
			},
		},
		Integrations: []config.Integration{
			config.Integration{
				Enabled: true,
				Name:    "bar",
				Table: wpg.Table{
					Name: "bar",
					Columns: []wpg.Column{
						wpg.Column{Name: "block_hash", Type: "bytea"},
					},
				},
				Block: []dig.BlockData{
					dig.BlockData{
						Name:   "block_hash",
						Column: "block_hash",
					},
				},
				Sources: []config.Source{
					config.Source{Name: "foo"},
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
	diff.Test(t, t.Fatalf, tasks[1].start, uint64(0))
}

func TestLoadTasks_Backfill(t *testing.T) {
	ctx := context.Background()
	pqxtest.CreateDB(t, Schema)
	pg, err := pgxpool.New(ctx, pqxtest.DSNForTest(t))
	diff.Test(t, t.Fatalf, err, nil)

	conf := config.Root{
		PGURL: pqxtest.DSNForTest(t),
		Sources: []config.Source{
			config.Source{
				Name:    "foo",
				ChainID: 888,
				URL:     "http://foo",
			},
		},
		Integrations: []config.Integration{
			config.Integration{
				Enabled: true,
				Name:    "bar",
				Table: wpg.Table{
					Name: "bar",
					Columns: []wpg.Column{
						wpg.Column{Name: "block_hash", Type: "bytea"},
					},
				},
				Block: []dig.BlockData{
					dig.BlockData{
						Name:   "block_hash",
						Column: "block_hash",
					},
				},
				Sources: []config.Source{
					config.Source{Name: "foo", Start: 2, Stop: 3},
				},
			},
			config.Integration{
				Enabled: true,
				Name:    "baz",
				Table: wpg.Table{
					Name: "baz",
					Columns: []wpg.Column{
						wpg.Column{Name: "block_hash", Type: "bytea"},
					},
				},
				Block: []dig.BlockData{
					dig.BlockData{
						Name:   "block_hash",
						Column: "block_hash",
					},
				},
				Sources: []config.Source{
					config.Source{Name: "foo", Start: 1, Stop: 3},
				},
			},
		},
	}
	tasks, err := loadTasks(ctx, pg, conf)
	diff.Test(t, t.Fatalf, err, nil)
	diff.Test(t, t.Fatalf, len(tasks), 2)
	diff.Test(t, t.Fatalf, tasks[0].backfill, false)
	diff.Test(t, t.Errorf, tasks[1].backfill, true)

	tg := &testGeth{}
	tg.add(0, hash(0), hash(0))
	tg.add(1, hash(1), hash(0))
	tg.add(2, hash(2), hash(1))
	tg.add(3, hash(3), hash(2))
	tg.add(4, hash(4), hash(3))
	tg.add(5, hash(5), hash(4))

	tasks[0].parts[0].src = tg
	diff.Test(t, t.Errorf, nil, tasks[0].Setup())
	checkQuery(t, pg, `
		select count(*) = 1
		from shovel.task_updates
		where src_name = 'foo'
		and backfill = false
	`)
	checkQuery(t, pg, `
		select num = 4
		from shovel.task_updates
		where src_name = 'foo'
		and backfill = false
	`)
	checkQuery(t, pg, `
		select count(*) = 2
		from shovel.ig_updates
		where src_name = 'foo'
		and backfill = false
	`)
	checkQuery(t, pg, `
		select num = 4
		from shovel.ig_updates
		where src_name = 'foo'
		and name = 'baz'
		and backfill = false
	`)
	checkQuery(t, pg, `
		select num = 4
		from shovel.ig_updates
		where src_name = 'foo'
		and name = 'bar'
		and backfill = false
	`)
	tasks[1].parts[0].src = tg
	diff.Test(t, t.Errorf, nil, tasks[1].Setup())
	diff.Test(t, t.Errorf, uint64(1), tasks[1].start)
	diff.Test(t, t.Errorf, uint64(3), tasks[1].stop)
	checkQuery(t, pg, `
		select count(*) = 1
		from shovel.task_updates
		where src_name = 'foo'
		and backfill
	`)
	checkQuery(t, pg, `
		select num = 1
		from shovel.task_updates
		where src_name = 'foo'
		and backfill
	`)
	checkQuery(t, pg, `
		select count(*) = 2
		from shovel.ig_updates
		where src_name = 'foo'
		and backfill
	`)
	checkQuery(t, pg, `
		select num = 2
		from shovel.ig_updates
		where src_name = 'foo'
		and name = 'bar'
		and backfill
	`)
	checkQuery(t, pg, `
		select num = 1
		from shovel.ig_updates
		where src_name = 'foo'
		and name = 'baz'
		and backfill
	`)
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
