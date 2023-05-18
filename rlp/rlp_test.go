package rlp

import (
	"encoding/hex"
	"testing"

	"github.com/indexsupply/x/bint"
	"kr.dev/diff"
)

func BenchmarkEncode(b *testing.B) {
	b.ReportAllocs()
	for n := 0; n < b.N; n++ {
		Encode([]byte("hello world"))
	}
}

func BenchmarkDecode(b *testing.B) {
	eb := Encode([]byte("hello world"))
	b.ReportAllocs()
	for n := 0; n < b.N; n++ {
		Bytes(eb)
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
			nil,
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
		itr := Iter(EncodeList(tc.input...))
		res := [][]byte{}
		for itr.HasNext() {
			res = append(res, itr.Bytes())
		}
		diff.Test(t, t.Errorf, tc.input, res)
	}
}

func TestDecode_List_Nested(t *testing.T) {
	// Set-theoretic definition of 3
	// [ [], [[]], [ [], [[]] ] ]
	var (
		zero  = EncodeList([]byte{})
		one   = EncodeList(zero)
		two   = EncodeList(zero, one)
		input = EncodeList(zero, one, two)
	)
	for i, s := 0, Iter(input); s.HasNext(); i++ {
		switch i {
		case 0:
			s0 := Iter(s.Bytes())
			diff.Test(t, t.Errorf, []byte(nil), s0.Bytes())
		case 1:
			s1 := Iter(s.Bytes())
			s2 := Iter(s1.Bytes())
			diff.Test(t, t.Errorf, []byte(nil), s2.Bytes())
		case 2:
			s1 := Iter(s.Bytes())
			s2 := Iter(s1.Bytes())
			diff.Test(t, t.Errorf, []byte(nil), s2.Bytes())
			s3 := Iter(s1.Bytes())
			s4 := Iter(s3.Bytes())
			diff.Test(t, t.Errorf, []byte(nil), s4.Bytes())
		}
	}
}

func hb(s string) []byte {
	b, _ := hex.DecodeString(s)
	return b
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
		diff.Test(t, t.Errorf, tc.want, EncodeList(tc.input...))
	}
}
