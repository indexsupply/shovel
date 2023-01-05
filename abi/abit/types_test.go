package abit

import "testing"

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
		if tc.t.Name() != tc.want {
			t.Errorf("got: %s want: %s", tc.t.Name(), tc.want)
		}
	}
}
