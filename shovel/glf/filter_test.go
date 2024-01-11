package glf

import (
	"testing"

	"kr.dev/diff"
)

func TestNeeds(t *testing.T) {
	cases := []struct {
		fields   []string
		headers  bool
		blocks   bool
		receipts bool
		logs     bool
	}{
		{
			fields:   []string{"tx_status", "tx_input", "log_idx"},
			blocks:   true,
			receipts: true,
		},
		{
			fields:  []string{"block_time", "log_idx"},
			headers: true,
			logs:    true,
		},
		{
			fields: []string{"tx_input"},
			blocks: true,
		},
		{
			fields: []string{"log_idx"},
			logs:   true,
		},
		{
			fields:   []string{"tx_status"},
			receipts: true,
		},
	}
	for _, tc := range cases {
		f := Filter{}
		f.Needs(tc.fields)
		diff.Test(t, t.Errorf, f.UseHeaders, tc.headers)
		diff.Test(t, t.Errorf, f.UseBlocks, tc.blocks)
		diff.Test(t, t.Errorf, f.UseReceipts, tc.receipts)
		diff.Test(t, t.Errorf, f.UseLogs, tc.logs)
	}
}
