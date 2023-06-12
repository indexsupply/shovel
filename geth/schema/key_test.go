package schema

import (
	"crypto/rand"
	"testing"

	"kr.dev/diff"
)

func r32() []byte {
	var b [32]byte
	rand.Read(b[:])
	return b[:]
}

func TestParseKey(t *testing.T) {
	cases := []struct {
		s string
		n uint64
		h []byte
	}{
		{"headers", 42, r32()},
		{"bodies", 42, r32()},
		{"receipts", 42, r32()},
	}
	for _, tc := range cases {
		s, n, h := ParseKey(Key(tc.s, tc.n, tc.h))
		diff.Test(t, t.Errorf, s, tc.s)
		diff.Test(t, t.Errorf, n, tc.n)
		diff.Test(t, t.Errorf, h, tc.h)
	}
}
