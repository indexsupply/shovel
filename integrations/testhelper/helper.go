// Easily test e2pg integrations
package testhelper

import (
	"context"
	"testing"

	"github.com/indexsupply/x/e2pg"
	"github.com/indexsupply/x/geth/gethtest"

	"blake.io/pqx/pqxtest"
	"github.com/jackc/pgx/v5/pgxpool"
	"kr.dev/diff"
)

func check(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

type H struct {
	tb  testing.TB
	ctx context.Context
	PG  *pgxpool.Pool
	gt  *gethtest.Helper
}

// jrpc.Client is required when testdata isn't
// available in the integrations/testdata directory.
func New(tb testing.TB) *H {
	ctx := context.Background()

	pqxtest.CreateDB(tb, e2pg.Schema)
	pg, err := pgxpool.New(ctx, pqxtest.DSNForTest(tb))
	diff.Test(tb, tb.Fatalf, err, nil)

	return &H{
		tb:  tb,
		ctx: ctx,
		PG:  pg,
		gt:  gethtest.New(tb, "http://hera:8545"),
	}
}

// Reset the task table. Call this in-between test cases
func (th *H) Reset() {
	_, err := th.PG.Exec(context.Background(), "truncate table e2pg.task")
	check(th.tb, err)
}

func (th *H) Context() context.Context {
	return th.ctx
}

func (th *H) Done() {
	th.gt.Done()
}

// Process will download the header,bodies, and receipts data
// if it doesn't exist in: integrations/testdata
// In the case that it needs to fetch the data, an RPC
// client will be used. The RPC endpoint needs to support
// the debug_dbAncient and debug_dbGet methods.
func (th *H) Process(ig e2pg.Integration, n uint64) {
	var (
		geth = e2pg.NewGeth(th.gt.FileCache, th.gt.Client)
		task = e2pg.NewTask(0, 0, "main", 1, 1, geth, th.PG, 0, 0, ig)
	)
	cur, err := geth.Hash(n)
	check(th.tb, err)
	prev, err := geth.Hash(n - 1)
	check(th.tb, err)
	th.gt.SetLatest(n, cur)
	check(th.tb, task.Insert(n-1, prev))
	check(th.tb, task.Converge(true))
}
