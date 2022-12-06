package abi

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"reflect"
	"testing"

	"github.com/indexsupply/x/abi/at"
	"github.com/indexsupply/x/tc"
)

func TestPad(t *testing.T) {
	cases := []struct {
		desc  string
		input []byte
		want  []byte
	}{
		{
			desc:  "< 4",
			input: []byte{0x2a},
			want:  []byte{0x2a, 0x00, 0x00, 0x00},
		},
		{
			desc:  "= 4",
			input: []byte{0x2a, 0x00, 0x00, 0x00},
			want:  []byte{0x2a, 0x00, 0x00, 0x00},
		},
		{
			desc:  "> 4",
			input: []byte{0x2a, 0x00, 0x00, 0x00, 0x00},
			want:  []byte{0x2a, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		},
	}
	for _, c := range cases {
		got := rpad(4, c.input)
		if !bytes.Equal(c.want, got) {
			t.Errorf("want: %x got: %x", c.want, got)
		}
	}
}

func TestSolidityVectors(t *testing.T) {
	cases := []struct {
		desc  string
		input Item
		want  string
	}{
		{
			desc: "https://docs.soliditylang.org/en/latest/abi-spec.html#examples",
			input: Tuple(
				String("dave"),
				Bool(true),
				List(Uint64(1), Uint64(2), Uint64(3)),
			),
			want: `0000000000000000000000000000000000000000000000000000000000000060000000000000000000000000000000000000000000000000000000000000000100000000000000000000000000000000000000000000000000000000000000a0000000000000000000000000000000000000000000000000000000000000000464617665000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000003000000000000000000000000000000000000000000000000000000000000000100000000000000000000000000000000000000000000000000000000000000020000000000000000000000000000000000000000000000000000000000000003`,
		},
		{
			desc: "https://docs.soliditylang.org/en/latest/abi-spec.html#use-of-dynamic-types",
			input: Tuple(
				List(List(Uint64(1), Uint64(2)), List(Uint64(3))),
				List(String("one"), String("two"), String("three")),
			),
			want: `000000000000000000000000000000000000000000000000000000000000004000000000000000000000000000000000000000000000000000000000000001400000000000000000000000000000000000000000000000000000000000000002000000000000000000000000000000000000000000000000000000000000004000000000000000000000000000000000000000000000000000000000000000a0000000000000000000000000000000000000000000000000000000000000000200000000000000000000000000000000000000000000000000000000000000010000000000000000000000000000000000000000000000000000000000000002000000000000000000000000000000000000000000000000000000000000000100000000000000000000000000000000000000000000000000000000000000030000000000000000000000000000000000000000000000000000000000000003000000000000000000000000000000000000000000000000000000000000006000000000000000000000000000000000000000000000000000000000000000a000000000000000000000000000000000000000000000000000000000000000e000000000000000000000000000000000000000000000000000000000000000036f6e650000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000374776f000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000057468726565000000000000000000000000000000000000000000000000000000`,
		},
	}
	for _, c := range cases {
		want, err := hex.DecodeString(c.want)
		tc.NoErr(t, err)
		got := Encode(c.input)
		if !bytes.Equal(want, got) {
			t.Errorf("want: %x got: %x", want, got)
		}
	}
}

func TestDecode(t *testing.T) {
	cases := []struct {
		desc string
		want Item
		t    at.Type
	}{
		{
			desc: "1 static",
			want: Uint64(0),
			t:    at.Uint64,
		},
		{
			desc: "N static",
			want: Tuple(Uint64(0), Uint64(1)),
			t:    at.Tuple(at.Uint64, at.Uint64),
		},
		{
			desc: "1 dynamic",
			want: String("hello world"),
			t:    at.String,
		},
		{
			desc: "N dynamic",
			want: Tuple(String("hello"), String("world")),
			t:    at.Tuple(at.String, at.String),
		},
		{
			desc: "list static",
			want: List(Uint64(0), Uint64(1)),
			t:    at.List(at.Uint64),
		},
		{
			desc: "list dynamic",
			want: List(String("hello"), String("world")),
			t:    at.List(at.String),
		},
		{
			desc: "tuple static",
			want: Tuple(Uint64(0)),
			t:    at.Tuple(at.Uint64),
		},
		{
			desc: "tuple static and dynamic",
			want: Tuple(Uint64(0), String("hello")),
			t:    at.Tuple(at.Uint64, at.String),
		},
		{
			desc: "tuple tuple",
			want: Tuple(Uint64(0), Tuple(String("hello"))),
			t:    at.Tuple(at.Uint64, at.Tuple(at.String)),
		},
		{
			desc: "tuple tuple list",
			want: Tuple(Uint64(0), Tuple(String("hello")), List(Uint64(1))),
			t:    at.Tuple(at.Uint64, at.Tuple(at.String), at.List(at.Uint64)),
		},
	}
	for _, c := range cases {
		got := Decode(debug(t, Encode(c.want)), c.t)
		if !reflect.DeepEqual(c.want, got) {
			t.Errorf("decode %q want: %#v got: %#v", c.desc, c.want, got)
		}
	}
}

func debug(t *testing.T, b []byte) []byte {
	t.Helper()
	out := fmt.Sprintf("len: %d\n", len(b))
	for i := 0; i < len(b); i += 32 {
		out += fmt.Sprintf("%x\n", b[i:i+32])
	}
	t.Logf("debug:\n%s\n", out)
	return b
}
