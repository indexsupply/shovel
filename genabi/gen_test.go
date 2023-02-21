package genabi

import (
	"testing"

	"kr.dev/diff"
)

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
