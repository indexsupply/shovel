package config

import (
	"database/sql"
	"encoding/json"
	"os"
	"testing"

	"blake.io/pqx/pqxtest"
	"github.com/indexsupply/x/integrations/testhelper"
	"github.com/jackc/pgx/v5/stdlib"
	"kr.dev/diff"
)

func TestMain(m *testing.M) {
	sql.Register("postgres", stdlib.GetDefaultDriver())
	pqxtest.TestMain(m)
}

func TestIntegrations(t *testing.T) {
	th := testhelper.New(t)
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
				and offer_token is null
				and consideration_recipient = '\x5e97a8773122bde31d44756f271c87893991a6ea'
				`,
				`
				select true from seaport_test
				where order_hash = '\xdaf50b59a508ee06e269125af28e796477ebf55d22a3c6a24e42d038d9d8d8ee'
				and tx_hash = '\x713df81a2ab53db1d01531106fc5de43012a401ddc3e0586d522e5c55a162d42'
				and log_idx = 3
				and offer_token is null
				and consideration_recipient = '\x0000a26b00c1f0df003000390027140000faa719'
				`,
				`
				select true from seaport_test
				where order_hash = '\xdaf50b59a508ee06e269125af28e796477ebf55d22a3c6a24e42d038d9d8d8ee'
				and tx_hash = '\x713df81a2ab53db1d01531106fc5de43012a401ddc3e0586d522e5c55a162d42'
				and log_idx = 3
				and offer_token = '\x57f1887a8bf19b14fc0df6fd9b2acc9af147ea85'
				and consideration_recipient is null
				`,
			},
		},
	}
	for _, tc := range cases {
		th.Reset()
		ig := Integration{}
		decode(t, read(t, tc.config), &ig)
		eig, err := getIntegration(th.PG, ig)
		diff.Test(t, t.Errorf, nil, err)
		th.Process(eig, tc.blockNum)
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
