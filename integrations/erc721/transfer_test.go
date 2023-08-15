package erc721

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
			937821,
			`
				select true from nft_transfers
				where block_hash = '\x48b4a89444ac40f06a28321764cf3843a0be9aeff111c5722c9d813ed5a7d8e8'
				and transaction_hash = '\x9f73358a1fd71319c6ea0f96f1e5ed7cd6b86071f087b76e956bb4da20d7f87f'
				and tx_signer = '\x38150290c18D9B28c6D13B12beBf779C36f76cb1'
				and f = '\x38150290c18d9b28c6d13b12bebf779c36f76cb1'
				and t = '\x38150290c18d9b28c6d13b12bebf779c36f76cb1'
				and token_id = 0
			`,
		},
		{
			937821,
			`
				select true from nft_transfers
				where block_hash = '\x48b4a89444ac40f06a28321764cf3843a0be9aeff111c5722c9d813ed5a7d8e8'
				and transaction_hash = '\x9f73358a1fd71319c6ea0f96f1e5ed7cd6b86071f087b76e956bb4da20d7f87f'
				and tx_signer = '\x38150290c18D9B28c6D13B12beBf779C36f76cb1'
				and f = '\xf58b008970f45b9de73a65c9f80307c20527a0f5'
				and t = '\x38150290c18d9b28c6d13b12bebf779c36f76cb1'
				and token_id = 100000000000
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
