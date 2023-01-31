package abit

import (
	"reflect"
	"testing"
)

func TestStatic(t *testing.T) {
	cases := []struct {
		desc string
		t    Type
		want bool
	}{
		{
			desc: "simple static",
			t:    Uint8,
			want: true,
		},
		{
			desc: "simple dynamic",
			t:    Bytes,
			want: false,
		},
		{
			desc: "static size list with static elements",
			t:    ListK(1, Uint8),
			want: true,
		},
		{
			desc: "static size list with dynamic elements",
			t:    ListK(1, Bytes),
			want: false,
		},
		{
			desc: "dynamic size list with static elements",
			t:    List(Uint8),
			want: false,
		},
		{
			desc: "dynamic size list with dynamic elements",
			t:    List(Bytes),
			want: false,
		},
		{
			desc: "tuple list with static fields",
			t:    Tuple(Uint8),
			want: true,
		},
		{
			desc: "tuple list with dynamic fields",
			t:    Tuple(Bytes),
			want: false,
		},
		{
			desc: "tuple list with dynamic & static fields",
			t:    Tuple(Uint8, Bytes),
			want: false,
		},
		{
			desc: "tuple with nested static tuple",
			t:    Tuple(Uint8, Tuple(Uint8)),
			want: true,
		},
	}
	for _, tc := range cases {
		got := tc.t.Static()
		if got != tc.want {
			t.Errorf("%q got: %t want: %t", tc.desc, got, tc.want)
		}
	}
}

func TestResolve(t *testing.T) {
	cases := []struct {
		desc string
		want Type
	}{
		{
			desc: "uint8",
			want: Uint8,
		},
		{
			desc: "uint8[]",
			want: List(Uint8),
		},
		{
			desc: "uint8[][][]",
			want: List(List(List(Uint8))),
		},
		{
			desc: "tuple",
			want: Tuple(),
		},
		{
			desc: "tuple[]",
			want: List(Tuple()),
		},
		{
			desc: "address[6]",
			want: ListK(6, Address),
		},
		{
			desc: "address[][6]",
			want: ListK(6, List(Address)),
		},
	}
	for _, tc := range cases {
		r := Resolve(tc.desc)
		if !reflect.DeepEqual(r, tc.want) {
			t.Errorf("got: %s want: %s", r.Name, tc.want.Name)
		}
	}
}

func TestSignature(t *testing.T) {
	cases := []struct {
		t    Type
		want string
	}{
		{
			t:    Address,
			want: "address",
		},
		{
			t:    Tuple(Address, Uint256),
			want: "(address,uint256)",
		},
		{
			t:    List(Tuple(Address, Uint256)),
			want: "(address,uint256)[]",
		},
		{
			t:    List(Address),
			want: "address[]",
		},
		{
			t:    ListK(8, Address),
			want: "address[8]",
		},
	}
	for _, tc := range cases {
		if tc.t.Signature() != tc.want {
			t.Errorf("got: %s want: %s", tc.t.Signature(), tc.want)
		}
	}
}
