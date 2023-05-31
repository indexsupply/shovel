package rlp

import (
	"crypto/rand"
	"encoding/hex"
	"testing"

	"github.com/indexsupply/x/bint"
	"kr.dev/diff"
)

func hb(s string) []byte {
	b, _ := hex.DecodeString(s)
	return b
}

func BenchmarkEncode(b *testing.B) {
	b.ReportAllocs()
	for n := 0; n < b.N; n++ {
		Encode([]byte("hello world"))
	}
}

func BenchmarkDecode(b *testing.B) {
	eb := List(
		Encode(hb("aa")),
		Encode(hb("bb")),
		Encode(hb("cc")),
		Encode(hb("dd")),
	)
	b.ReportAllocs()
	for n := 0; n < b.N; n++ {
		for itr := Iter(eb); itr.HasNext(); {
			itr.Bytes()
		}
	}
}

func TestDecodeLength(t *testing.T) {
	cases := []struct {
		t              byte
		header         []byte
		expectedLength int
	}{
		// list more than 55 bytes, full 8 bytes needed for length
		{
			t:              list55H,
			header:         []byte{0xff, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			expectedLength: 1 << 56,
		},
		// list more than 55 bytes, but binary encoding of length only fits into one byte
		{
			t:              list55H,
			header:         []byte{list55H + 1, 0xe2},
			expectedLength: 226, // e2 is 226 in decimal
		},
		// list more than 55 bytes, but binary encoding of length  fits into two bytes
		{
			t:              list55H,
			header:         []byte{list55H + 2, 0x12, 0xab},
			expectedLength: 4779, // 12ab is 4779 in decimal
		},
		// string more than 55 bytes, but binary encoding of length fits into two bytes
		{
			t:              str55H,
			header:         []byte{str55H + 2, 0x12, 0xab},
			expectedLength: 4779, // 12ab is 4779 in decimal
		},
		// string more than 55 bytes, but binary encoding of length fits into two bytes
		{
			t:              str55H,
			header:         []byte{str55H + 2, 0x12, 0xab},
			expectedLength: 4779, // 12ab is 4779 in decimal
		},
	}

	for _, c := range cases {
		gotHeaderLength, gotLength := decodeLength(c.t, c.header)
		if gotHeaderLength != len(c.header) {
			t.Errorf("expected header length %d, got %d", gotHeaderLength, len(c.header))
		}
		if gotLength != c.expectedLength {
			t.Errorf("expected length %d, got %d", gotLength, c.expectedLength)
		}
	}
}

func TestDecode(t *testing.T) {
	cases := []struct {
		desc  string
		input []byte
	}{
		{
			"empty bytes",
			[]byte{},
		},
		{
			"short string",
			[]byte("foo"),
		},
		{
			"long string",
			[]byte("Lorem ipsum dolor sit amet, consectetur adipisicing elit"),
		},
	}
	for _, tc := range cases {
		diff.Test(t, t.Errorf, tc.input, Bytes(Encode(tc.input)))
	}
}

func TestDecode_List(t *testing.T) {
	cases := []struct {
		desc  string
		input [][]byte
	}{
		{
			"empty list",
			[][]byte{},
		},
		{
			"list of short strings",
			[][]byte{
				[]byte("foo"),
				[]byte("bar"),
			},
		},
		{
			"list of long strings",
			[][]byte{
				[]byte("Lorem ipsum dolor sit amet, consectetur adipisicing elit"),
				[]byte("Porem ipsum dolor sit amet, consectetur adipisicing elit"),
			},
		},
	}
	for _, tc := range cases {
		itr := Iter(List(Encode(tc.input...)))
		res := [][]byte{}
		for itr.HasNext() {
			res = append(res, itr.Bytes())
		}
		diff.Test(t, t.Errorf, tc.input, res)
	}
}

func rb(n int) []byte {
	b := make([]byte, n)
	rand.Read(b)
	return b
}

func TestDecode_List_Nested_Long(t *testing.T) {
	a, b, c, d, e := rb(1<<6), rb(1<<6), rb(1<<6), rb(1<<6), rb(1<<6)
	input := List(
		Encode(a),
		List(Encode(b, c, d)),
		Encode(e),
	)
	for i, it := 0, Iter(input); it.HasNext(); i++ {
		switch i {
		case 0:
			diff.Test(t, t.Errorf, a, it.Bytes())
		case 1:
			diff.Test(t, t.Errorf, List(Encode(b, c, d)), it.Bytes())
		case 2:
			diff.Test(t, t.Errorf, e, it.Bytes())
		default:
			t.Fatal("should only be three items")
		}
	}
}

func TestDecode_List_Nested(t *testing.T) {
	// Set-theoretic definition of 3
	// [ [], [[]], [ [], [[]] ] ]
	var (
		zero  = List(Encode([]byte{}))
		one   = List(zero)
		two   = List(zero, one)
		input = List(zero, one, two)
	)
	for i, s := 0, Iter(input); s.HasNext(); i++ {
		switch i {
		case 0:
			s0 := Iter(s.Bytes())
			diff.Test(t, t.Errorf, []byte{}, s0.Bytes())
		case 1:
			s1 := Iter(s.Bytes())
			s2 := Iter(s1.Bytes())
			diff.Test(t, t.Errorf, []byte{}, s2.Bytes())
		case 2:
			s1 := Iter(s.Bytes())
			s2 := Iter(s1.Bytes())
			diff.Test(t, t.Errorf, []byte{}, s2.Bytes())
			s3 := Iter(s1.Bytes())
			s4 := Iter(s3.Bytes())
			diff.Test(t, t.Errorf, []byte{}, s4.Bytes())
		}
	}
}

func TestEncode(t *testing.T) {
	cases := []struct {
		desc  string
		input []byte
		want  []byte
	}{
		{
			"zero byte",
			[]byte{0},
			[]byte{0x80},
		},
		{
			"int 1024",
			bint.Encode(nil, 1<<10),
			[]byte{0x82, 0x04, 0x00},
		},
		{
			"long string",
			[]byte("foobarbazfoobarbazfoobarbazfoobarbazfoobarbazfoobarbaz"),
			hb("b6666f6f62617262617a666f6f62617262617a666f6f62617262617a666f6f62617262617a666f6f62617262617a666f6f62617262617a"),
		},
	}
	for _, tc := range cases {
		diff.Test(t, t.Errorf, tc.want, Encode(tc.input))
	}
}

func TestEncode_List(t *testing.T) {
	cases := []struct {
		desc  string
		input [][]byte
		want  []byte
	}{
		{
			"empty list",
			nil,
			[]byte{0xc0},
		},
		{
			"long list",
			[][]byte{
				[]byte("foobarbazfoobarbazfoobarbazfoobarbazfoobarbazfoobarbaz"),
				[]byte("foobarbazfoobarbazfoobarbazfoobarbazfoobarbazfoobarbaz"),
			},
			hb("f86eb6666f6f62617262617a666f6f62617262617a666f6f62617262617a666f6f62617262617a666f6f62617262617a666f6f62617262617ab6666f6f62617262617a666f6f62617262617a666f6f62617262617a666f6f62617262617a666f6f62617262617a666f6f62617262617a"),
		},

		{
			"list of strings",
			[][]byte{
				[]byte("cat"),
				[]byte("dog"),
			},
			[]byte{
				0xc8, // 200
				0x83, // 131
				0x63, // c
				0x61, // a
				0x74, // t
				0x83, // 131
				0x64, // d
				0x6f, // o
				0x67, // g
			},
		},
	}
	for _, tc := range cases {
		diff.Test(t, t.Errorf, tc.want, List(Encode(tc.input...)))
	}
}
