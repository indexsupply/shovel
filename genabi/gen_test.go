package genabi

import (
	"testing"

	"kr.dev/diff"
)

func TestSchema(t *testing.T) {
	cases := []struct {
		event Descriptor
		want  string
	}{
		{
			event: Descriptor{
				Inputs: []Field{},
			},
			want: "()",
		},
		{
			event: Descriptor{
				Inputs: []Field{
					Field{Indexed: true, Type: "uint8"},
				},
			},
			want: "()",
		},
		{
			event: Descriptor{
				Inputs: []Field{
					Field{Indexed: false, Type: "uint8"},
				},
			},
			want: "(uint8)",
		},
		{
			event: Descriptor{
				Inputs: []Field{
					Field{Indexed: true, Type: "uint8"},
					Field{Indexed: false, Type: "uint8"},
				},
			},
			want: "(uint8)",
		},
		{
			event: Descriptor{
				Inputs: []Field{
					Field{Indexed: false, Type: "uint8"},
					Field{Indexed: true, Type: "uint8"},
				},
			},
			want: "(uint8)",
		},
	}
	for _, tc := range cases {
		diff.Test(t, t.Errorf, schema(unindexed(tc.event.Inputs)), tc.want)
	}
}

func TestHasNext(t *testing.T) {
	cases := []struct {
		ah   arrayHelper
		want bool
	}{
		{
			ah:   arrayHelper{Field: Field{Type: "uint8"}},
			want: false,
		},
		{
			ah:   arrayHelper{Field: Field{Type: "uint8[][][]"}, Index: 0},
			want: true,
		},
		{
			ah:   arrayHelper{Field: Field{Type: "uint8[][][]"}, Index: 1},
			want: true,
		},
		{
			ah:   arrayHelper{Field: Field{Type: "uint8[][][]"}, Index: 2},
			want: false,
		},
	}
	for _, tc := range cases {
		diff.Test(t, t.Errorf, tc.ah.HasNext(), tc.want)
	}
}

func TestFixedLength(t *testing.T) {
	cases := []struct {
		ah   arrayHelper
		want bool
	}{
		{
			ah:   arrayHelper{Field: Field{Type: "uint8"}},
			want: false,
		},
		{
			ah:   arrayHelper{Field: Field{Type: "uint8[]"}},
			want: false,
		},
		{
			ah:   arrayHelper{Field: Field{Type: "uint8[2]"}},
			want: true,
		},
		{
			ah:   arrayHelper{Field: Field{Type: "uint8[200]"}},
			want: true,
		},
	}
	for _, tc := range cases {
		diff.Test(t, t.Errorf, tc.ah.FixedLength(), tc.want)
	}
}

func TestType(t *testing.T) {
	cases := []struct {
		index  int
		goType string
		want   string
	}{
		{
			goType: "uint8",
			want:   "uint8",
		},
		{
			goType: "[]uint8",
			want:   "[]uint8",
		},
		{
			goType: "[2]uint8",
			want:   "[2]uint8",
		},
		{
			goType: "[3][2]uint8",
			want:   "[3][2]uint8",
		},
		{
			goType: "[][]uint8",
			want:   "[][]uint8",
		},
		{
			index:  1,
			goType: "[][]uint8",
			want:   "[]uint8",
		},
		{
			index:  2,
			goType: "[][]uint8",
			want:   "uint8",
		},
	}
	for _, tc := range cases {
		ah := arrayHelper{Index: tc.index}
		diff.Test(t, t.Errorf, ah.Type(tc.goType), tc.want)
	}
}

func TestLower(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{
			input: "Foo",
			want:  "foo",
		},
		{
			input: "FooBar",
			want:  "fooBar",
		},
		{
			input: "FOOBAR",
			want:  "fOOBAR",
		},
	}
	for _, tc := range cases {
		diff.Test(t, t.Errorf, lower(tc.input), tc.want)
	}
}

func TestCamel(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{
			input: "",
			want:  "",
		},
		{
			input: "_",
			want:  "_",
		},
		{
			input: "__",
			want:  "_",
		},
		{
			input: "Foo",
			want:  "Foo",
		},
		{
			input: "FooBar",
			want:  "FooBar",
		},
		{
			input: "FOOBAR",
			want:  "FOOBAR",
		},
		{
			input: "foo_bar",
			want:  "FooBar",
		},
		{
			input: "foo_bar_",
			want:  "FooBar",
		},
		{
			input: "_foo_bar_",
			want:  "_FooBar",
		},
	}
	for _, tc := range cases {
		diff.Test(t, t.Errorf, camel(tc.input), tc.want)
	}
}
