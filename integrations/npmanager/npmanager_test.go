package npmanager

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
			17618219,
			`
				select true from npmanager_dao_deployed
				where block_hash = '\x46b291d192c27591b5e7fdba018bd5f85e377c70ec1db6d3901952eede4f4b45'
				and transaction_hash = '\x6ba8fabbcc2aa79356ffb83c44bbbd004031159d9c9bcd84cfb56918d87d81ed'
				and token  = '\xDb1dFf78386a006f6349D45B185b8B68F69CAedc'
				and metadata = '\x1C3bA8413CF9Bc6B7Bbb31EbDd5b92EE445E4439'
				and auction = '\x71c73FbD4866889eee560Cb831FA3019278DEB26'
				and treasury = '\x0d41Ae0bA91b6b0F1446616c70519AEc5455E1fB'
				and governor = '\x63FF7BD37F5b48d3d4A6aCA87D05841746cD2814'
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
