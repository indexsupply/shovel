package abit

import (
	"reflect"
	"testing"
)

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
	}
	for _, tc := range cases {
		r := Resolve(tc.desc)
		if !reflect.DeepEqual(r, tc.want) {
			t.Errorf("got: %s want: %s", r.Name, tc.want.Name)
		}
	}
}

func TestName(t *testing.T) {
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
	}
	for _, tc := range cases {
		if tc.t.Signature() != tc.want {
			t.Errorf("got: %s want: %s", tc.t.Signature(), tc.want)
		}
	}
}
