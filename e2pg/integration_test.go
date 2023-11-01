package e2pg

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"blake.io/pqx/pqxtest"
	"github.com/indexsupply/x/geth/gethtest"
	"github.com/jackc/pgx/v5/pgxpool"
	"kr.dev/diff"
)

func check(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

type Helper struct {
	tb  testing.TB
	ctx context.Context
	PG  *pgxpool.Pool
	gt  *gethtest.Helper
}

// jrpc.Client is required when testdata isn't
// available in the integrations/testdata directory.
func NewHelper(tb testing.TB) *Helper {
	ctx := context.Background()

	pqxtest.CreateDB(tb, Schema)
	pg, err := pgxpool.New(ctx, pqxtest.DSNForTest(tb))
	diff.Test(tb, tb.Fatalf, err, nil)

	return &Helper{
		tb:  tb,
		ctx: ctx,
		PG:  pg,
		gt:  gethtest.New(tb, "http://hera:8545"),
	}
}

// Reset the task table. Call this in-between test cases
func (th *Helper) Reset() {
	_, err := th.PG.Exec(context.Background(), "truncate table e2pg.task")
	check(th.tb, err)
}

func (th *Helper) Context() context.Context {
	return th.ctx
}

func (th *Helper) Done() {
	th.gt.Done()
}

// Process will download the header,bodies, and receipts data
// if it doesn't exist in: integrations/testdata
// In the case that it needs to fetch the data, an RPC
// client will be used. The RPC endpoint needs to support
// the debug_dbAncient and debug_dbGet methods.
func (th *Helper) Process(dest Destination, n uint64) {
	var (
		geth = NewGeth(th.gt.FileCache, th.gt.Client)
		task = NewTask(
			WithSource(0, "", geth),
			WithPG(th.PG),
			WithDestinations(dest),
			WithRange(n, n+1),
		)
	)
	cur, err := geth.Hash(n)
	check(th.tb, err)
	th.gt.SetLatest(n, cur)

	check(th.tb, task.Setup())
	check(th.tb, task.Converge(true))
}

func TestIntegrations(t *testing.T) {
	th := NewHelper(t)
	defer th.Done()
	cases := []struct {
		blockNum uint64
		config   string
		queries  []string
	}{
		{
			17943843,
			"txinput.json",
			[]string{
				`
				select count(*) = 1 from txinput_test
				where tx_hash = '\x713df81a2ab53db1d01531106fc5de43012a401ddc3e0586d522e5c55a162d42'
				and block_number = 17943843
				and block_time = 1692387935
				`,
			},
		},
		{
			17943843,
			"erc721.json",
			[]string{
				`
				select count(*) = 4 from erc721_test
				where tx_hash = '\x713df81a2ab53db1d01531106fc5de43012a401ddc3e0586d522e5c55a162d42'
				and contract = '\x57f1887a8bf19b14fc0df6fd9b2acc9af147ea85'
				`,
			},
		},
		{
			17943843,
			"seaport.json",
			[]string{
				`
				select true from seaport_test
				where order_hash = '\xdaf50b59a508ee06e269125af28e796477ebf55d22a3c6a24e42d038d9d8d8ee'
				and tx_hash = '\x713df81a2ab53db1d01531106fc5de43012a401ddc3e0586d522e5c55a162d42'
				and log_idx = 3
				and abi_idx = 0
				and offer_token = '\x57f1887a8bf19b14fc0df6fd9b2acc9af147ea85'
				and consideration_recipient is null
				`,
				`
				select true from seaport_test
				where order_hash = '\xdaf50b59a508ee06e269125af28e796477ebf55d22a3c6a24e42d038d9d8d8ee'
				and tx_hash = '\x713df81a2ab53db1d01531106fc5de43012a401ddc3e0586d522e5c55a162d42'
				and log_idx = 3
				and abi_idx = 1
				and offer_token is null
				and consideration_recipient = '\x5e97a8773122bde31d44756f271c87893991a6ea'
				`,
				`
				select true from seaport_test
				where order_hash = '\xdaf50b59a508ee06e269125af28e796477ebf55d22a3c6a24e42d038d9d8d8ee'
				and tx_hash = '\x713df81a2ab53db1d01531106fc5de43012a401ddc3e0586d522e5c55a162d42'
				and log_idx = 3
				and abi_idx = 2
				and offer_token is null
				and consideration_recipient = '\x0000a26b00c1f0df003000390027140000faa719'
				`,
			},
		},
	}
	for _, tc := range cases {
		th.Reset()
		ig := Integration{}
		decode(t, read(t, tc.config), &ig)
		dest, err := getDest(th.PG, ig)
		diff.Test(t, t.Errorf, nil, err)
		th.Process(dest, tc.blockNum)
		for i, q := range tc.queries {
			var found bool
			err := th.PG.QueryRow(th.Context(), q).Scan(&found)
			diff.Test(t, t.Errorf, nil, err)
			if err != nil {
				t.Logf("failing test query: %d", i)
			}
			if !found {
				t.Errorf("test %s failed", tc.config)
			}
		}
	}
}

func read(tb testing.TB, name string) []byte {
	path := "testdata/" + name
	b, err := os.ReadFile(path)
	if err != nil {
		tb.Fatalf("unable to read file %s", path)
	}
	return b
}

func decode(tb testing.TB, js []byte, dest any) {
	if err := json.Unmarshal(js, dest); err != nil {
		tb.Fatalf("decoding json: %.4s error: %s", string(js), err.Error())
	}
}
