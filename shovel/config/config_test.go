package config

import (
	"testing"

	"github.com/indexsupply/shovel/dig"
	"github.com/indexsupply/shovel/wpg"

	"kr.dev/diff"
)

func TestUnion(t *testing.T) {
	cases := []struct {
		a    wpg.Table
		b    wpg.Table
		want wpg.Table
	}{
		{
			a: wpg.Table{
				Name: "a",
				Columns: []wpg.Column{
					{Name: "c1", Type: "int"},
				},
			},
			b: wpg.Table{
				Name: "a",
				Columns: []wpg.Column{
					{Name: "c1", Type: "int"},
					{Name: "c2", Type: "int"},
				},
			},
			want: wpg.Table{
				Name: "a",
				Columns: []wpg.Column{
					{Name: "c1", Type: "int"},
					{Name: "c2", Type: "int"},
				},
			},
		},
		{
			a: wpg.Table{
				Name: "a",
				Columns: []wpg.Column{
					{Name: "c1", Type: "int"},
					{Name: "c2", Type: "int"},
				},
			},
			b: wpg.Table{
				Name: "a",
				Columns: []wpg.Column{
					{Name: "c1", Type: "int"},
				},
			},
			want: wpg.Table{
				Name: "a",
				Columns: []wpg.Column{
					{Name: "c1", Type: "int"},
					{Name: "c2", Type: "int"},
				},
			},
		},
		{
			a: wpg.Table{
				Name:    "a",
				Columns: []wpg.Column{},
			},
			b: wpg.Table{
				Name: "a",
				Columns: []wpg.Column{
					{Name: "c1", Type: "int"},
					{Name: "c2", Type: "int"},
				},
			},
			want: wpg.Table{
				Name: "a",
				Columns: []wpg.Column{
					{Name: "c1", Type: "int"},
					{Name: "c2", Type: "int"},
				},
			},
		},
	}
	for _, tc := range cases {
		got := union(tc.a, tc.b)
		diff.Test(t, t.Errorf, got, tc.want)
	}
}

func TestDDL(t *testing.T) {
	conf := &Root{
		Integrations: []Integration{
			{
				Name: "foo",
				Table: wpg.Table{
					Name: "foo",
					Columns: []wpg.Column{
						{Name: "block_num", Type: "numeric"},
						{Name: "b", Type: "bytea"},
						{Name: "c", Type: "bytea"},
						{Name: "from", Type: "bytea"},
					},
				},
				Block: []dig.BlockData{
					{Name: "block_num", Column: "block_num"},
				},
				Event: dig.Event{
					Name: "bar",
					Inputs: []dig.Input{
						{Indexed: true, Name: "a"},
						{Indexed: true, Name: "b", Column: "b"},
						{Indexed: false, Name: "c", Column: "c"},
					},
				},
			},
		},
	}
	diff.Test(t, t.Errorf, ValidateFix(conf), nil)
	diff.Test(t, t.Errorf, DDL(*conf), []string{
		"create table if not exists foo(block_num numeric, b bytea, c bytea, \"from\" bytea, ig_name text, src_name text, tx_idx int, log_idx int, abi_idx int2)",
		"create unique index if not exists u_foo on foo (ig_name, src_name, block_num, tx_idx, log_idx, abi_idx)",
	})
}

func TestValidateFix(t *testing.T) {
	conf := &Root{
		Integrations: []Integration{
			{
				Name: "foo",
				Table: wpg.Table{
					Name: "foo",
					Columns: []wpg.Column{
						{Name: "block_num", Type: "numeric"},
						{Name: "b", Type: "bytea"},
						{Name: "c", Type: "bytea"},
					},
				},
				Block: []dig.BlockData{
					{Name: "block_num", Column: "block_num"},
				},
				Event: dig.Event{
					Name: "bar",
					Inputs: []dig.Input{
						{Indexed: true, Name: "a"},
						{Indexed: true, Name: "b", Column: "b"},
						{Indexed: false, Name: "c", Column: "c"},
					},
				},
			},
		},
	}
	diff.Test(t, t.Errorf, ValidateFix(conf), nil)
	diff.Test(t, t.Errorf, conf.Integrations[0].Table, wpg.Table{
		Name: "foo",
		Columns: []wpg.Column{
			{Name: "block_num", Type: "numeric"},
			{Name: "b", Type: "bytea"},
			{Name: "c", Type: "bytea"},
			{Name: "ig_name", Type: "text"},
			{Name: "src_name", Type: "text"},
			{Name: "tx_idx", Type: "int"},
			{Name: "log_idx", Type: "int"},
			{Name: "abi_idx", Type: "int2"},
		},
		DisableUnique: false,
		Unique: [][]string{
			{"ig_name", "src_name", "block_num", "tx_idx", "log_idx", "abi_idx"},
		},
	})
}

func TestValidateFix_MissingCols(t *testing.T) {
	conf := &Root{
		Integrations: []Integration{
			{
				Name: "foo",
				Table: wpg.Table{
					Name: "foo",
					Columns: []wpg.Column{
						{Name: "c", Type: "bytea"},
					},
				},
				Event: dig.Event{
					Name: "bar",
					Inputs: []dig.Input{
						{Indexed: true, Name: "a"},
						{Indexed: true, Name: "b", Column: "b"},
						{Indexed: true, Name: "c", Column: "c"},
					},
				},
			},
		},
	}
	const want = "checking config for references: missing column for b"
	diff.Test(t, t.Errorf, ValidateFix(conf).Error(), want)
}
