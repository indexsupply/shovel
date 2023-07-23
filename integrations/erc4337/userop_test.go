package erc4337

import (
	"database/sql"
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
			17749342, //random block with userop txs
			`
				select true from erc4337_userops
				where block_hash = '\xa380903891ed01a0778d95c38a1b09cc130fe612d3ed7e071add32aaf00fd2b7'
				and transaction_hash = '\x552e073f0a639c052e550dcbf2f48507f6190371e045f6c7ab5e0aeef83f8440'
				and op_hash = '\xB5DD2735A1BCDC273B063CB57B0460C93D8A2192C0681FA1F484D06F416835F3'
				and op_sender = '\x659fe7D0E5f63cA0af5bcBE593F33cDf8411eDEd'
				and op_paymaster = '\x0000000000000000000000000000000000000000'
				and op_nonce = 23
				and op_success = true
				and op_actual_gas_cost = 3422112536957888
				and op_actual_gas_used = 165689
			`,
		},
	}
	for _, tc := range cases {
		th.Reset()
		th.Process(Integration, tc.blockNum)
		var found bool
		diff.Test(t, t.Errorf, nil, th.PG.QueryRow(th.Context(), tc.query).Scan(&found))
	}
}
