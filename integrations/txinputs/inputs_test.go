package txinputs

import (
	"database/sql"
	"encoding/hex"
	"testing"

	"github.com/indexsupply/x/integrations/testhelper"

	"blake.io/pqx/pqxtest"
	"github.com/jackc/pgx/v5/stdlib"
	"kr.dev/diff"
)

func TestMain(m *testing.M) {
	sql.Register("postgres", stdlib.GetDefaultDriver())
	pqxtest.TestMain(m)
}

func TestTransfer(t *testing.T) {
	th := testhelper.New(t)
	defer th.Done()

	cases := []struct {
		blockNum uint64
		query    string
	}{
		{
			6307510, //first usdc transfer
			`
				select true from tx_inputs
				where block_hash = '\x755ad481c67c136fdb6daafab432257ebc2aec940019e95b72a020ec1b4041a7'
				and tx_hash = '\xdc6bb2a1aff2dbb2613113984b5fbd560e582c0a4369149402d7ea83b0f5983e'
				and tx_signer = '\x5B6122C109B78C6755486966148C1D70a50A47D7'
				and tx_to = '\xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48'
				and tx_input = '\x40c10f1900000000000000000000000055fe002aeff02f77364de339a1292923a15844b80000000000000000000000000000000000000000000000000000000001312d00'
			`,
		},
	}
	for _, tc := range cases {
		th.Reset()
		filterFrom, _ = hex.DecodeString("5B6122C109B78C6755486966148C1D70a50A47D7")
		filterTo, _ = hex.DecodeString("A0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48")
		th.Process(Integration, tc.blockNum)
		var found bool
		diff.Test(t, t.Errorf, nil, th.PG.QueryRow(th.Context(), tc.query).Scan(&found))
	}
}
