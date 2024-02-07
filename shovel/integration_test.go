package shovel

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/indexsupply/x/jrpc2"
	"github.com/indexsupply/x/shovel/config"
	"github.com/indexsupply/x/wpg"
	"github.com/jackc/pgx/v5/pgxpool"
	"kr.dev/diff"
)

func check(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// Process will download the header,bodies, and receipts data
// if it doesn't exist in: integrations/testdata
// In the case that it needs to fetch the data, an RPC
// client will be used. The RPC endpoint needs to support
// the debug_dbAncient and debug_dbGet methods.
func process(tb testing.TB, pg *pgxpool.Pool, conf config.Root, n uint64) *Task {
	check(tb, config.ValidateFix(&conf))
	check(tb, config.Migrate(context.Background(), pg, conf))

	task, err := NewTask(
		WithPG(pg),
		WithSource(jrpc2.New("https://ethereum.publicnode.com")),
		WithIntegration(conf.Integrations[0]),
		WithRange(n, n+1),
	)
	check(tb, err)
	check(tb, task.Converge())
	return task
}

func TestIntegrations(t *testing.T) {
	cases := []struct {
		blockNum    uint64
		config      string
		queries     []string
		deleteQuery string
	}{
		{
			17943843,
			"txinput.json",
			[]string{
				`
				select count(*) = 1 from txinput_test
				where tx_hash = '\x713df81a2ab53db1d01531106fc5de43012a401ddc3e0586d522e5c55a162d42'
				and block_num = 17943843
				and block_time = 1692387935
				`,
			},
			"select count(*) = 0 from txinput_test",
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
				`
				select count(*) = 1 from erc721_test
				where tx_hash = '\x713df81a2ab53db1d01531106fc5de43012a401ddc3e0586d522e5c55a162d42'
				and contract = '\x57f1887a8bf19b14fc0df6fd9b2acc9af147ea85'
				and "from" = '\xce020e4bca3a181cacd771a4750e03f384779313'
				`,
			},
			"select count(*) = 0 from erc721_test",
		},
		{
			17943843,
			"seaport.json",
			[]string{
				`
				select true from seaport_test
				where order_hash = '\xdaf50b59a508ee06e269125af28e796477ebf55d22a3c6a24e42d038d9d8d8ee'
				and tx_hash = '\x713df81a2ab53db1d01531106fc5de43012a401ddc3e0586d522e5c55a162d42'
				and log_idx = 264
				and abi_idx = 0
				and offer_token = '\x57f1887a8bf19b14fc0df6fd9b2acc9af147ea85'
				and consideration_recipient is null
				`,
				`
				select true from seaport_test
				where order_hash = '\xdaf50b59a508ee06e269125af28e796477ebf55d22a3c6a24e42d038d9d8d8ee'
				and tx_hash = '\x713df81a2ab53db1d01531106fc5de43012a401ddc3e0586d522e5c55a162d42'
				and log_idx = 264
				and abi_idx = 1
				and offer_token is null
				and consideration_recipient = '\x5e97a8773122bde31d44756f271c87893991a6ea'
				`,
				`
				select true from seaport_test
				where order_hash = '\xdaf50b59a508ee06e269125af28e796477ebf55d22a3c6a24e42d038d9d8d8ee'
				and tx_hash = '\x713df81a2ab53db1d01531106fc5de43012a401ddc3e0586d522e5c55a162d42'
				and log_idx = 264
				and abi_idx = 2
				and offer_token is null
				and consideration_recipient = '\x0000a26b00c1f0df003000390027140000faa719'
				`,
			},
			"select count(*) = 0 from seaport_test",
		},
	}
	for _, tc := range cases {
		pg := wpg.TestPG(t, Schema)
		conf := config.Root{Integrations: []config.Integration{{}}}
		decode(t, read(t, tc.config), &conf.Integrations[0])
		task := process(t, pg, conf, tc.blockNum)
		for i, q := range tc.queries {
			var found bool
			err := pg.QueryRow(context.Background(), q).Scan(&found)
			diff.Test(t, t.Errorf, nil, err)
			if err != nil {
				t.Logf("failing test query: %d", i)
			}
			if !found {
				t.Errorf("test %s failed", tc.config)
			}
		}

		check(t, task.Delete(pg, tc.blockNum))

		var deleted bool
		err := pg.QueryRow(context.Background(), tc.deleteQuery).Scan(&deleted)
		diff.Test(t, t.Errorf, nil, err)
		if !deleted {
			t.Errorf("%s was not cleaned up after ig.Delete", tc.config)
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
