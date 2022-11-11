package bint

import "testing"

func TestDecode(t *testing.T) {
	var cases = []uint8{0, 8, 16, 32, 64}
	for _, e := range cases {
		i := uint64(1<<e - 1)
		b, n := Encode(i)
		if n != e/8 {
			t.Errorf("num bytes expected %d got: %d", e/8, n)
		}
		got := Decode(b)
		if got != i {
			t.Errorf("expected %d got: %d", i, got)
		}
	}
}
