package abi

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/indexsupply/x/abi/abit"
	"github.com/indexsupply/x/tc"
	"github.com/kr/pretty"
)

func h232b(s string) [32]byte {
	b, _ := hex.DecodeString(s)
	return *(*[32]byte)(b)
}

func h2b(s string) []byte {
	b, _ := hex.DecodeString(s)
	return b
}

func TestMatch(t *testing.T) {
	cases := []struct {
		desc string
		l    Log
		e    Event
		ok   bool
		want Item
	}{
		{
			desc: "zero",
			l: Log{
				Topics: [4][32]byte{},
				Data:   []byte{},
			},
			e: Event{
				Anonymous: true,
				Inputs:    []Input{},
			},
			ok:   true,
			want: Tuple([]Item{}...),
		},
		{
			desc: "one indexed event",
			l: Log{
				Topics: [4][32]byte{
					h232b("91376cf23f43e2a9c3647f44b00086ec05ac93e8a8fbca53a196250877534e82"),
					h232b("0000000000000000000000000000000000000000000000000000000000000042"),
				},
				Data: []byte{},
			},
			e: Event{
				SignatureHash: h232b("91376cf23f43e2a9c3647f44b00086ec05ac93e8a8fbca53a196250877534e82"),
				Inputs: []Input{
					Input{
						Indexed: true,
						Name:    "foo",
						Type:    "bytes32",
					},
				},
			},
			ok: true,
			want: Tuple(Bytes32(
				h232b("0000000000000000000000000000000000000000000000000000000000000042"),
			)),
		},
		{
			desc: "invalid signature",
			l: Log{
				Topics: [4][32]byte{
					h232b("deadbeef00000000000000000000000000000000000000000000000000000000"),
					h232b("0000000000000000000000000000000000000000000000000000000000000042"),
				},
				Data: []byte{},
			},
			e: Event{
				SignatureHash: h232b("91376cf23f43e2a9c3647f44b00086ec05ac93e8a8fbca53a196250877534e82"),
				Inputs: []Input{
					Input{
						Indexed: true,
						Name:    "foo",
						Type:    "bytes32",
					},
				},
			},
			ok:   false,
			want: Tuple([]Item{}...),
		},
		{
			desc: "valid signature",
			l: Log{
				Topics: [4][32]byte{
					h232b("0f96ed1b236328f1d2894cbda9d0f2795aefae71821755f5b7d524822393dcae"),
					h232b("0000000000000000000000000000000000000000000000000000000000000042"),
				},
				Data: Encode(Tuple(
					Bytes([]byte("foo")),
					Bytes([]byte("baz")),
				)),
			},
			e: Event{
				SignatureHash: h232b("0f96ed1b236328f1d2894cbda9d0f2795aefae71821755f5b7d524822393dcae"),
				Inputs: []Input{
					Input{
						Indexed: false,
						Name:    "foo",
						Type:    "bytes",
					},
					Input{
						Indexed: true,
						Name:    "bar",
						Type:    "bytes32",
					},
					Input{
						Indexed: false,
						Name:    "baz",
						Type:    "bytes",
					},
				},
			},
			ok: true,
			want: Tuple(
				Bytes([]byte("foo")),
				Bytes32(h232b("0000000000000000000000000000000000000000000000000000000000000042")),
				Bytes([]byte("baz")),
			),
		},
	}
	for _, tc := range cases {
		got, ok := Match(tc.l, tc.e)
		if ok != tc.ok {
			t.Errorf("%q match got: %t want: %t", tc.desc, ok, tc.ok)
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%q item got: %# v want: %# v", tc.desc, pretty.Formatter(got), pretty.Formatter(tc.want))
		}
	}
}

func TestABIType(t *testing.T) {
	cases := []struct {
		input Input
		want  abit.Type
	}{
		{
			input: Input{
				Name: "a",
				Type: "uint8",
			},
			want: abit.Uint8,
		},
		{
			input: Input{
				Name: "a",
				Type: "uint8[]",
			},
			want: abit.List(abit.Uint8),
		},
		{
			input: Input{
				Name: "a",
				Type: "tuple",
				Components: []Input{
					{
						Name: "b",
						Type: "uint8",
					},
				},
			},
			want: abit.Tuple(abit.Uint8),
		},
		{
			input: Input{
				Name: "a",
				Type: "tuple[][]",
				Components: []Input{
					{
						Name: "b",
						Type: "uint8",
					},
					{
						Name: "c",
						Type: "tuple",
						Components: []Input{
							{
								Name: "d",
								Type: "uint8",
							},
						},
					},
				},
			},
			want: abit.List(abit.List(abit.Tuple(
				abit.Uint8,
				abit.Tuple(
					abit.Uint8,
				),
			))),
		},
	}
	for _, tc := range cases {
		got := tc.input.ABIType()
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("got: %s want: %s", got.Name, tc.want.Name)
		}
	}
}

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

func TestEncode(t *testing.T) {
	hb := func(s string) []byte {
		s = strings.Map(func(r rune) rune {
			switch {
			case r >= '0' && r <= '9':
				return r
			case r >= 'a' && r <= 'f':
				return r
			default:
				return -1
			}
		}, strings.ToLower(s))
		b, err := hex.DecodeString(s)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	cases := []struct {
		desc  string
		input Item
		want  []byte
	}{
		{
			desc:  "single static",
			input: Uint8(42),
			want:  hb("000000000000000000000000000000000000000000000000000000000000002a"),
		},
		{
			desc:  "single dynamic",
			input: String("hello world"),
			want: hb(`
				000000000000000000000000000000000000000000000000000000000000000b
				68656c6c6f20776f726c64000000000000000000000000000000000000000000
			`),
		},
		{
			desc:  "dynamic list of static types",
			input: List(Uint8(42)),
			want: hb(`
				0000000000000000000000000000000000000000000000000000000000000001
				000000000000000000000000000000000000000000000000000000000000002a
			`),
		},
		{
			desc:  "dynamic list of dynamic types",
			input: List(String("hello"), String("world")),
			want: hb(`
				0000000000000000000000000000000000000000000000000000000000000002
				0000000000000000000000000000000000000000000000000000000000000040
				0000000000000000000000000000000000000000000000000000000000000080
				0000000000000000000000000000000000000000000000000000000000000005
				68656c6c6f000000000000000000000000000000000000000000000000000000
				0000000000000000000000000000000000000000000000000000000000000005
				776f726c64000000000000000000000000000000000000000000000000000000
			`),
		},
		{
			desc:  "static list of static types",
			input: ListK(2, Uint8(42), Uint8(43)),
			want: hb(`
				000000000000000000000000000000000000000000000000000000000000002a
				000000000000000000000000000000000000000000000000000000000000002b
			`),
		},
		{
			desc:  "static list of dynamic types",
			input: ListK(2, String("hello"), String("world")),
			want: hb(`
				0000000000000000000000000000000000000000000000000000000000000040
				0000000000000000000000000000000000000000000000000000000000000080
				0000000000000000000000000000000000000000000000000000000000000005
				68656c6c6f000000000000000000000000000000000000000000000000000000
				0000000000000000000000000000000000000000000000000000000000000005
				776f726c64000000000000000000000000000000000000000000000000000000
			`),
		},
		{
			desc:  "dynamic nested list of dynamic types",
			input: List(List(String("hello"), String("world")), List(String("bye"))),
			want: hb(`
				0000000000000000000000000000000000000000000000000000000000000002
				0000000000000000000000000000000000000000000000000000000000000040
				0000000000000000000000000000000000000000000000000000000000000120
				0000000000000000000000000000000000000000000000000000000000000002
				0000000000000000000000000000000000000000000000000000000000000040
				0000000000000000000000000000000000000000000000000000000000000080
				0000000000000000000000000000000000000000000000000000000000000005
				68656c6c6f000000000000000000000000000000000000000000000000000000
				0000000000000000000000000000000000000000000000000000000000000005
				776f726c64000000000000000000000000000000000000000000000000000000
				0000000000000000000000000000000000000000000000000000000000000001
				0000000000000000000000000000000000000000000000000000000000000020
				0000000000000000000000000000000000000000000000000000000000000003
				6279650000000000000000000000000000000000000000000000000000000000
			`),
		},
		{
			desc:  "tuple with static fields",
			input: Tuple(Uint8(42), Uint8(43)),
			want: hb(`
				000000000000000000000000000000000000000000000000000000000000002a
				000000000000000000000000000000000000000000000000000000000000002b
			`),
		},
		{
			desc:  "tuple with dynamic fields",
			input: Tuple(Uint8(42), Uint8(43), String("hello")),
			want: hb(`
				000000000000000000000000000000000000000000000000000000000000002a
				000000000000000000000000000000000000000000000000000000000000002b
				0000000000000000000000000000000000000000000000000000000000000060
				0000000000000000000000000000000000000000000000000000000000000005
				68656c6c6f000000000000000000000000000000000000000000000000000000
			`),
		},
		{
			desc: "tuple with list of static tuples",
			input: Tuple(
				List(
					Tuple(Uint8(44), Uint8(45)),
				),
				List(
					Tuple(Uint8(46), Uint8(47)),
					Tuple(Uint8(48), Uint8(49)),
				),
			),
			want: hb(`
				0000000000000000000000000000000000000000000000000000000000000040
				00000000000000000000000000000000000000000000000000000000000000a0
				0000000000000000000000000000000000000000000000000000000000000001
				000000000000000000000000000000000000000000000000000000000000002c
				000000000000000000000000000000000000000000000000000000000000002d
				0000000000000000000000000000000000000000000000000000000000000002
				000000000000000000000000000000000000000000000000000000000000002e
				000000000000000000000000000000000000000000000000000000000000002f
				0000000000000000000000000000000000000000000000000000000000000030
				0000000000000000000000000000000000000000000000000000000000000031
			`),
		},
		{
			desc: "tuple with list of dynamic tuples",
			input: Tuple(
				Uint8(42),
				Uint8(43),
				String("hello"),
				List(
					Tuple(
						Uint8(42),
						Uint8(43),
						String("hello"),
					),
				),
			),
			want: hb(`
				000000000000000000000000000000000000000000000000000000000000002a
				000000000000000000000000000000000000000000000000000000000000002b
				0000000000000000000000000000000000000000000000000000000000000080
				00000000000000000000000000000000000000000000000000000000000000c0
				0000000000000000000000000000000000000000000000000000000000000005
				68656c6c6f000000000000000000000000000000000000000000000000000000
				0000000000000000000000000000000000000000000000000000000000000001
				0000000000000000000000000000000000000000000000000000000000000020
				000000000000000000000000000000000000000000000000000000000000002a
				000000000000000000000000000000000000000000000000000000000000002b
				0000000000000000000000000000000000000000000000000000000000000060
				0000000000000000000000000000000000000000000000000000000000000005
				68656c6c6f000000000000000000000000000000000000000000000000000000
			`),
		},
		{
			desc:  "nested static tuples",
			input: Tuple(Tuple(Uint8(42), Uint8(43), Tuple(Uint8(44), Uint8(45)))),
			want: hb(`
				000000000000000000000000000000000000000000000000000000000000002a
				000000000000000000000000000000000000000000000000000000000000002b
				000000000000000000000000000000000000000000000000000000000000002c
				000000000000000000000000000000000000000000000000000000000000002d
			`),
		},
		{
			desc:  "nested dynamic tuples",
			input: Tuple(Tuple(Uint8(42), Tuple(Uint8(43), String("foo")))),
			want: hb(`
				0000000000000000000000000000000000000000000000000000000000000020
				000000000000000000000000000000000000000000000000000000000000002a
				0000000000000000000000000000000000000000000000000000000000000040
				000000000000000000000000000000000000000000000000000000000000002b
				0000000000000000000000000000000000000000000000000000000000000040
				0000000000000000000000000000000000000000000000000000000000000003
				666f6f0000000000000000000000000000000000000000000000000000000000
			`),
		},
	}
	for _, tc := range cases {
		got := Encode(tc.input)
		if !bytes.Equal(got, tc.want) {
			t.Errorf("%q\ngot: %s\nwant: %s\n", tc.desc, dump(got), dump(tc.want))
		}
	}
}

func dump(b []byte) (out string) {
	out += "\n"
	for i := 0; i < len(b); i += 32 {
		out += fmt.Sprintf("%x\n", b[i:i+32])
	}
	return out
}

func TestDecode(t *testing.T) {
	cases := []struct {
		desc string
		want Item
		t    abit.Type
	}{
		{
			desc: "1 uint8",
			want: Uint8(0),
			t:    abit.Uint8,
		},
		{
			desc: "1 static",
			want: Uint64(0),
			t:    abit.Uint64,
		},
		{
			desc: "N static",
			want: Tuple(Uint64(0), Uint64(1)),
			t:    abit.Tuple(abit.Uint64, abit.Uint64),
		},
		{
			desc: "1 dynamic",
			want: String("hello world"),
			t:    abit.String,
		},
		{
			desc: "N dynamic",
			want: Tuple(String("hello"), String("world")),
			t:    abit.Tuple(abit.String, abit.String),
		},
		{
			desc: "list static",
			want: List(Uint64(0), Uint64(1)),
			t:    abit.List(abit.Uint64),
		},
		{
			desc: "list dynamic",
			want: List(String("hello"), String("world")),
			t:    abit.List(abit.String),
		},
		{
			desc: "fixed size list of static types",
			want: ListK(2, Uint8(42), Uint8(43)),
			t:    abit.ListK(2, abit.Uint8),
		},
		{
			desc: "tuple static",
			want: Tuple(Uint64(0)),
			t:    abit.Tuple(abit.Uint64),
		},
		{
			desc: "tuple static and dynamic",
			want: Tuple(Uint64(0), String("hello")),
			t:    abit.Tuple(abit.Uint64, abit.String),
		},
		{
			desc: "tuple tuple",
			want: Tuple(Uint64(0), Tuple(String("hello"))),
			t:    abit.Tuple(abit.Uint64, abit.Tuple(abit.String)),
		},
		{
			desc: "tuple tuple list",
			want: Tuple(Uint64(0), Tuple(String("hello")), List(Uint64(1))),
			t: abit.Tuple(
				abit.Uint64,
				abit.Tuple(
					abit.String,
				),
				abit.List(abit.Uint64),
			),
		},
		{
			desc: "tuple with nested static tuple",
			want: Tuple(Uint8(42), Uint8(43), Tuple(Uint8(44), Uint8(45))),
			t: abit.Tuple(
				abit.Uint8,
				abit.Uint8,
				abit.Tuple(
					abit.Uint8,
					abit.Uint8,
				),
			),
		},
		{
			desc: "tuple with nested dynamic tuple",
			want: Tuple(Uint8(42), String("foo"), Tuple(Uint8(44), String("bar"))),
			t: abit.Tuple(
				abit.Uint8,
				abit.String,
				abit.Tuple(
					abit.Uint8,
					abit.String,
				),
			),
		},
		{
			desc: "tuple with list of tuples",
			want: Tuple(
				Uint8(42),
				List(
					Tuple(Uint8(43), Uint8(44)),
				),
				List(
					Tuple(Uint8(45), Uint8(46)),
					Tuple(Uint8(47), Uint8(48)),
				),
			),
			t: abit.Tuple(
				abit.Uint8,
				abit.List(abit.Tuple(abit.Uint8, abit.Uint8)),
				abit.List(abit.Tuple(abit.Uint8, abit.Uint8)),
			),
		},
	}
	for _, c := range cases {
		got := Decode(debug(c.desc, t, Encode(c.want)), c.t)
		if !reflect.DeepEqual(c.want, got) {
			t.Errorf("decode %q want: %#v got: %#v", c.desc, c.want, got)
		}
	}
}

func debug(desc string, t *testing.T, b []byte) []byte {
	t.Helper()
	out := fmt.Sprintf("len: %d\n", len(b))
	for i := 0; i < len(b); i += 32 {
		out += fmt.Sprintf("%x\n", b[i:i+32])
	}
	t.Logf("%s\n%s\n", desc, out)
	return b
}
