package abi2

import (
	"database/sql"
	"encoding/json"
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
		queries  []string
	}{
		{
			17943843,
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
		var (
			ev  = Event{}
			bds = make([]BlockData, 0)
			tbl = Table{}
		)
		jd(t, eventJSON, &ev)
		jd(t, tableJSON, &tbl)
		jd(t, blockJSON, &bds)

		c, err := New(ev, bds, tbl)
		diff.Test(t, t.Fatalf, nil, err)
		diff.Test(t, t.Fatalf, nil, CreateTable(th.Context(), th.PG, c.Table))
		th.Process(c, tc.blockNum)
		for i, q := range tc.queries {
			var found bool
			err := th.PG.QueryRow(th.Context(), q).Scan(&found)
			diff.Test(t, t.Errorf, nil, err)
			if err != nil {
				t.Logf("failing test query: %d", i)
			}
		}
	}
}

func jd(tb testing.TB, js []byte, dest any) {
	if err := json.Unmarshal(js, dest); err != nil {
		tb.Fatalf("decoding json: %.4s error: %s", string(js), err.Error())
	}
}

var tableJSON = []byte(`{
  "name": "seaport_test",
  "columns": [
	{"name": "order_hash", "type": "bytea"},
	{"name": "consideration_recipient", "type": "bytea"},
	{"name": "offer_token", "type": "bytea"},
	{"name": "offerer", "type": "bytea"},
	{"name": "zone", "type": "bytea"},
	{"name": "recipient", "type": "bytea"},
	{"name": "block_num", "type": "numeric"},
	{"name": "tx_hash", "type": "bytea"},
	{"name": "log_idx", "type": "numeric"}
  ]

}`)

var blockJSON = []byte(`[
	{"name": "block_num", "column": "block_num"},
	{"name": "tx_hash", "column": "tx_hash", "filter_op": "contains", "filter_arg": ["713df81a2ab53db1d01531106fc5de43012a401ddc3e0586d522e5c55a162d42"]},
	{"name": "log_idx", "column": "log_idx"}
]`)

var eventJSON = []byte(`{
  "anonymous": false,
  "name": "OrderFulfilled",
  "type": "event",
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
  ]
}`)
