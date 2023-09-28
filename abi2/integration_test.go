package abi2

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

func TestInsert(t *testing.T) {
	th := testhelper.New(t)
	defer th.Done()
	cases := []struct {
		blockNum uint64
		query    string
	}{
		{
			17943843,
			`
				select true from seaport_test
				where order_hash = '\x796820863892f449ec5dc02582e6708292255cb1ed21f5c3b0ff4bff04328ff7'
			`,
		},
	}
	for _, tc := range cases {
		th.Reset()
		c, err := New(j)
		diff.Test(t, t.Fatalf, nil, err)
		diff.Test(t, t.Fatalf, nil, CreateTable(th.Context(), th.PG, c.Table()))
		th.Process(c, tc.blockNum)
		var found bool
		diff.Test(t, t.Errorf, nil, th.PG.QueryRow(th.Context(), tc.query).Scan(&found))
	}
}

var j = []byte(`{
  "anonymous": false,
  "inputs": [
    {
      "indexed": false,
      "internalType": "bytes32",
      "name": "orderHash",
      "type": "bytes32",
	  "column": "order_hash"
    },
    {
      "indexed": true,
      "internalType": "address",
      "name": "offerer",
      "type": "address",
	  "column": "offerer"
    },
    {
      "indexed": true,
      "internalType": "address",
      "name": "zone",
      "type": "address",
	  "column": "zone"
    },
    {
      "indexed": false,
      "internalType": "address",
      "name": "recipient",
      "type": "address",
	  "column": "recipient"
    },
    {
      "components": [
        {
          "internalType": "enum ItemType",
          "name": "itemType",
          "type": "uint8"
        },
        {
          "internalType": "address",
          "name": "token",
          "type": "address",
		  "column": "offer_token"
        },
        {
          "internalType": "uint256",
          "name": "identifier",
          "type": "uint256"
        },
        {
          "internalType": "uint256",
          "name": "amount",
          "type": "uint256"
        }
      ],
      "indexed": false,
      "internalType": "struct SpentItem[]",
      "name": "offer",
      "type": "tuple[]"
    },
    {
      "components": [
        {
          "internalType": "enum ItemType",
          "name": "itemType",
          "type": "uint8"
        },
        {
          "internalType": "address",
          "name": "token",
          "type": "address"
        },
        {
          "internalType": "uint256",
          "name": "identifier",
          "type": "uint256"
        },
        {
          "internalType": "uint256",
          "name": "amount",
          "type": "uint256"
        },
        {
          "internalType": "address payable",
          "name": "recipient",
          "type": "address",
		  "column": "consideration_recipient"
        }
      ],
      "indexed": false,
      "internalType": "struct ReceivedItem[]",
      "name": "consideration",
      "type": "tuple[]"
    }
  ],
  "extra": {
	  "table": {
		  "name": "seaport_test",
		  "columns": [
			{"name": "order_hash", "type": "bytea"},
			{"name": "consideration_recipient", "type": "bytea"},
			{"name": "offer_token", "type": "bytea"},
			{"name": "offerer", "type": "bytea"},
			{"name": "zone", "type": "bytea"},
			{"name": "recipient", "type": "bytea"}
		  ]
	  }
  },
  "name": "OrderFulfilled",
  "type": "event"
}`)
