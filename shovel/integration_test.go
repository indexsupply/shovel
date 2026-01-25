package shovel

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/indexsupply/shovel/jrpc2"
	"github.com/indexsupply/shovel/shovel/config"
	"github.com/indexsupply/shovel/tc"
	"github.com/indexsupply/shovel/wpg"
)

func TestIntegrations(t *testing.T) {
	rpcURL := os.Getenv("ETH_RPC_URL")
	if rpcURL == "" {
		rpcURL = os.Getenv("MAINNET_RPC_URL")
	}
	if rpcURL == "" {
		t.Skip("ETH_RPC_URL or MAINNET_RPC_URL not set")
	}
	cases := []struct {
		blockNum uint64
		config   string
		check    []string
		delete   string
	}{
		{
			17943843,
			"filter-ref.json",
			[]string{
				`select count(*) = 1 from tx`,
				`select count(*) = 1 from tx_w_block`,
			},
			"select count(*) = 0 from tx",
		},
		{
			19583743,
			"receipt.json",
			[]string{
				`
				select count(*) = 0 from receipt_test
				where block_num = 19583743
				`,
			},
			"select count(*) = 0 from receipt_test",
		},
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

	for _, c := range cases {
		var (
			ctx  = context.Background()
			pg   = wpg.TestPG(t, Schema)
			conf = config.Root{Integrations: []config.Integration{{}}}
		)
		decode(t, read(t, c.config), &conf.Integrations)
		tc.NoErr(t, config.ValidateFix(&conf))
		tc.NoErr(t, config.Migrate(ctx, pg, conf))
		for _, ig := range conf.Integrations {
			task, err := NewTask(
				WithPG(pg),
				WithSource(jrpc2.New(rpcURL)),
				WithIntegration(ig),
				WithRange(c.blockNum, c.blockNum+1),
			)
			tc.NoErr(t, err)
			tc.NoErr(t, task.Converge())
		}
		check(t, c.config, pg, c.check...)
		for _, ig := range conf.Integrations {
			task, err := NewTask(
				WithPG(pg),
				WithSource(jrpc2.New(rpcURL)),
				WithIntegration(ig),
			)
			tc.NoErr(t, err)
			tc.NoErr(t, task.Delete(pg, c.blockNum))
			check(t, c.config, pg, c.delete)
		}
	}
}

func check(t *testing.T, name string, pg wpg.Conn, queries ...string) {
	for i, q := range queries {
		var res bool
		tc.NoErr(t, pg.QueryRow(context.Background(), q).Scan(&res))
		if !res {
			t.Errorf("query %s/%d failed: %q", name, i, q)
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
