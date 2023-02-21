package genabi

import (
	"testing"

	"kr.dev/diff"
)

func TestHasNext(t *testing.T) {
	cases := []struct {
		lh   listHelper
		want bool
	}{
		{
			lh:   listHelper{Input: Input{Type: "uint8"}},
			want: false,
		},
		{
			lh:   listHelper{Input: Input{Type: "uint8[][][]"}, Index: 0},
			want: true,
		},
		{
			lh:   listHelper{Input: Input{Type: "uint8[][][]"}, Index: 1},
			want: true,
		},
		{
			lh:   listHelper{Input: Input{Type: "uint8[][][]"}, Index: 2},
			want: false,
		},
	}
	for _, tc := range cases {
		diff.Test(t, t.Errorf, tc.lh.HasNext(), tc.want)
	}
}

func TestFixedLength(t *testing.T) {
	cases := []struct {
		lh   listHelper
		want bool
	}{
		{
			lh:   listHelper{Input: Input{Type: "uint8"}},
			want: false,
		},
		{
			lh:   listHelper{Input: Input{Type: "uint8[]"}},
			want: false,
		},
		{
			lh:   listHelper{Input: Input{Type: "uint8[2]"}},
			want: true,
		},
		{
			lh:   listHelper{Input: Input{Type: "uint8[200]"}},
			want: true,
		},
	}
	for _, tc := range cases {
		diff.Test(t, t.Errorf, tc.lh.FixedLength(), tc.want)
	}
}

func TestMakeArg(t *testing.T) {
	cases := []struct {
		lh   listHelper
		want string
	}{
		{
			lh:   listHelper{Input: Input{Type: "uint8"}},
			want: "uint8",
		},
		{
			lh:   listHelper{Input: Input{Type: "uint8[]"}},
			want: "[]uint8",
		},
		{
			lh:   listHelper{Input: Input{Type: "uint8[2]"}},
			want: "[2]uint8",
		},
		{
			lh:   listHelper{Input: Input{Type: "uint8[2][3]"}},
			want: "[3][2]uint8",
		},
		{
			lh:   listHelper{Input: Input{Type: "uint8[][]"}},
			want: "[][]uint8",
		},
		{
			lh:   listHelper{Input: Input{Type: "uint8[][]"}, Index: 0},
			want: "[][]uint8",
		},
		{
			lh:   listHelper{Input: Input{Type: "uint8[][]"}, Index: 1},
			want: "[]uint8",
		},
		{
			lh:   listHelper{Input: Input{Type: "uint8[][]"}, Index: 2},
			want: "uint8",
		},
	}
	for _, tc := range cases {
		diff.Test(t, t.Errorf, tc.lh.MakeArg(), tc.want)
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
			want:  "",
		},
		{
			input: "__",
			want:  "",
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
			want:  "FooBar",
		},
	}
	for _, tc := range cases {
		diff.Test(t, t.Errorf, camel(tc.input), tc.want)
	}
}
