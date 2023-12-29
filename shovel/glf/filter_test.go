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
			Filter{needs: []string{"bar", "foo"}},
		},
		{
			Filter{addresses: []string{"foo"}},
			Filter{addresses: []string{"bar"}},
			Filter{addresses: []string{"bar", "foo"}},
		},
		{
			Filter{addresses: []string{"foo"}},
			Filter{addresses: []string{"foo", "bar"}},
			Filter{addresses: []string{"bar", "foo"}},
		},
		{
			Filter{topics: [][]string{{}, {"foo"}}},
			Filter{topics: [][]string{{"bar"}, {}}},
			Filter{topics: [][]string{{"bar"}, {"foo"}}},
		},
		{
			Filter{topics: [][]string{{}, {"foo"}}},
			Filter{topics: [][]string{{"bar"}, {"foo"}}},
			Filter{topics: [][]string{{"bar"}, {"foo"}}},
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
