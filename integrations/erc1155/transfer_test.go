package erc1155

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
		desc     string
		blockNum uint64
		query    string
	}{
		{
			"first single in the chain",
			6930510,
			`
				select true from nft_transfers
				where block_hash = '\x5afac1c72a123eff51574fd20be1b1eb04fcb5a27a735422632284fef3f83427'
				and transaction_hash = '\x223600ba642f4dc6644e5eb4b0a02a6f67589ee4802be640b373fdd30bb00ff4'
				and tx_signer = '\x463dEF03F98b328A75051EE5Ebe9a6235De4ac59'
				and f = '\x0000000000000000000000000000000000000000'
				and t = '\x463dEF03F98b328A75051EE5Ebe9a6235De4ac59'
				and token_id = 32
				and quantity = 100000000
			`,
		},
		{
			"first batch in the chain",
			7409272,
			`
				select true from nft_transfers
				where block_hash = '\x87dbca1b9ae14e9fc60d836381892cda02186f106f4b5cba295bc529ab8ee85b'
				and transaction_hash = '\x52278badbf9de263906e6fb58be134cf806f380cd48c104cb4a0a6cdb7df4826'
				and tx_signer = '\xd046B3C521c0F5513C8A47eB3C2011684eA80B27'
				and f = '\x0000000000000000000000000000000000000000'
				and t = '\x3Cd5016f1AeB6723ebc67F3E17C0F4A163e78F51'
				and token_id = 474
				and quantity = 1
			`,
		},
		{
			"first batch in the chain",
			7409272,
			`
				select true from nft_transfers
				where block_hash = '\x87dbca1b9ae14e9fc60d836381892cda02186f106f4b5cba295bc529ab8ee85b'
				and transaction_hash = '\x52278badbf9de263906e6fb58be134cf806f380cd48c104cb4a0a6cdb7df4826'
				and f = '\x0000000000000000000000000000000000000000'
				and t = '\x3Cd5016f1AeB6723ebc67F3E17C0F4A163e78F51'
				and token_id = 13
				and quantity = 5
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
