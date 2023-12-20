package glf

import (
	"testing"

	"kr.dev/diff"
)

func TestMerge(t *testing.T) {
	cases := []struct {
		a, b Filter
		want Filter
	}{
		{},
		{
			Filter{needs: []string{"foo"}},
			Filter{needs: []string{"foo", "bar"}},
			Filter{needs: []string{"foo", "bar"}},
		},
		{
			Filter{Address: []string{"foo"}},
			Filter{Address: []string{"bar"}},
			Filter{Address: []string{"foo", "bar"}},
		},
		{
			Filter{Address: []string{"foo"}},
			Filter{Address: []string{"foo", "bar"}},
			Filter{Address: []string{"foo", "bar"}},
		},
		{
			Filter{Topics: [][]string{{}, {"foo"}}},
			Filter{Topics: [][]string{{"bar"}, {}}},
			Filter{Topics: [][]string{{"bar"}, {"foo"}}},
		},
		{
			Filter{Topics: [][]string{{}, {"foo"}}},
			Filter{Topics: [][]string{{"bar"}, {"foo"}}},
			Filter{Topics: [][]string{{"bar"}, {"foo"}}},
		},
	}
	for _, tc := range cases {
		tc.a.Merge(tc.b)
		diff.Test(t, t.Errorf, tc.a, tc.want)
	}
}

func TestNeeds(t *testing.T) {
	cases := []struct {
		fields   []string
		blocks   bool
		txs      bool
		receipts bool
		logs     bool
	}{
		{
			fields:   []string{"tx_status", "tx_input", "log_idx"},
			blocks:   true,
			txs:      true,
			receipts: true,
		},
		{
			fields: []string{"block_time", "log_idx"},
			blocks: true,
			logs:   true,
		},
		{
			fields: []string{"tx_input"},
			blocks: true,
			txs:    true,
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
		diff.Test(t, t.Errorf, f.UseBlocks, tc.blocks)
		diff.Test(t, t.Errorf, f.UseTxs, tc.txs)
		diff.Test(t, t.Errorf, f.UseReceipts, tc.receipts)
		diff.Test(t, t.Errorf, f.UseLogs, tc.logs)
	}
}
