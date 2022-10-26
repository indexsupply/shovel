package rlp

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"math"
	"reflect"
	"testing"
)

func FuzzEncode(f *testing.F) {
	var (
		numItems uint64 = 10
		payload         = []byte("hello")
	)
	f.Add(numItems, payload)
	f.Fuzz(func(t *testing.T, n uint64, d []byte) {
		var items []Item
		for i := 0; i < int(n); i++ {
			items = append(items, Bytes(d))
		}
		item := List(items...)
		got, err := Decode(Encode(item))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(item, got) {
			t.Errorf("want:\n%v\ngot:\n%v\n", item, got)
		}
	})
}

func BenchmarkEncode(b *testing.B) {
	payload := []byte("hello world")
	b.ReportAllocs()
	for n := 0; n < b.N; n++ {
		Encode(Bytes(payload))
	}
}

func intTo2b(i uint16) []byte {
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, i)
	return b
}

func randBytes(n int) []byte {
	res := make([]byte, n)
	rand.Read(res)
	return res
}

func TestDecode_Errors(t *testing.T) {
	cases := []struct {
		desc  string
		input []byte
		err   error
	}{
		{
			"short string no error",
			[]byte{byte(1)},
			nil,
		},
		{
			"long string. too many bytes",
			append(
				[]byte{
					byte(str55H + 1),
					byte(56),
				},
				randBytes(57)...,
			),
			errTooManyBytes,
		},
		{
			"long string. too few bytes",
			append(
				[]byte{
					byte(str55H + 1),
					byte(56),
				},
				randBytes(55)...,
			),
			errTooFewBytes,
		},
	}
	for _, tc := range cases {
		_, err := Decode(tc.input)
		if tc.err == nil {
			if err != nil {
				t.Errorf("expected nil error got: %v", err)
			}
		} else {
			if !errors.Is(tc.err, err) {
				t.Errorf("expected %v got %v", tc.err, err)
			}
		}
	}
}

func TestEncodeLength(t *testing.T) {
	cases := []struct {
		desc string
		n    uint64
		l    uint8
		b    []byte
	}{
		{
			"0",
			0,
			0,
			[]byte{},
		},
		{
			"1 byte",
			1,
			1,
			[]byte{0x01},
		},
		{
			"2 bytes",
			256,
			2,
			[]byte{0x01, 0x00},
		},
		{
			"max. 8 bytes",
			math.MaxUint64,
			8,
			[]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
		},
	}
	for _, tc := range cases {
		lengthSize, length := encodeLength(tc.n)
		if lengthSize != tc.l {
			t.Errorf("%s: expected: %d got: %d", tc.desc, tc.l, lengthSize)
		}
		if !bytes.Equal(length, tc.b) {
			t.Errorf("%s: expected: %x got: %x", tc.desc, tc.b, length)
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
		desc string
		item Item
	}{
		{
			"empty bytes",
			Bytes(nil),
		},
		{
			"short string",
			String("a"),
		},
		{
			"long string",
			String("Lorem ipsum dolor sit amet, consectetur adipisicing elit"),
		},
		{
			"empty list",
			List(),
		},
		{
			"list of short strings",
			List(String("a"), String("b")),
		},
		{
			"list of long strings",
			List(
				String("Lorem ipsum dolor sit amet, consectetur adipisicing elit"),
				String("Porem ipsum dolor sit amet, consectetur adipisicing elit"),
			),
		},
		{
			"the set theoretical representation of three",
			List(
				List(),
				List(List()),
				List(List(), List(List())),
			),
		},
	}
	for _, tc := range cases {
		got, err := Decode(Encode(tc.item))
		if err != nil {
			t.Errorf("error %s: %s", tc.desc, err)
		}
		if !reflect.DeepEqual(tc.item, got) {
			t.Errorf("%s\nwant:\n%# v\ngot:\n%# v\n", tc.desc, tc.item, got)
		}
	}
}

func TestEncode(t *testing.T) {
	cases := []struct {
		desc string
		item Item
		want []byte
	}{
		{
			"zero byte",
			Byte(0),
			[]byte{0x00},
		},
		{
			"int 0",
			Int(0),
			[]byte{0x80},
		},
		{
			"int 1024",
			Int(1024),
			[]byte{0x82, 0x04, 0x00},
		},
		{
			"empty string",
			String(""),
			[]byte{0x80},
		},
		{
			"non-empty string",
			String("Lorem ipsum dolor sit amet, consectetur adipisicing elit"),
			[]byte{
				0xB8,
				0x38,
				0x4C,
				0x6F,
				0x72,
				0x65,
				0x6D,
				0x20,
				0x69,
				0x70,
				0x73,
				0x75,
				0x6D,
				0x20,
				0x64,
				0x6F,
				0x6C,
				0x6F,
				0x72,
				0x20,
				0x73,
				0x69,
				0x74,
				0x20,
				0x61,
				0x6D,
				0x65,
				0x74,
				0x2C,
				0x20,
				0x63,
				0x6F,
				0x6E,
				0x73,
				0x65,
				0x63,
				0x74,
				0x65,
				0x74,
				0x75,
				0x72,
				0x20,
				0x61,
				0x64,
				0x69,
				0x70,
				0x69,
				0x73,
				0x69,
				0x63,
				0x69,
				0x6E,
				0x67,
				0x20,
				0x65,
				0x6C,
				0x69,
				0x74,
			},
		},
		{
			"empty list",
			List(),
			[]byte{0xc0},
		},
		{
			"list of strings",
			List(String("cat"), String("dog")),
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
		{
			"the set theoretical representation of three",
			List(
				List(),
				List(List()),
				List(List(), List(List())),
			),
			[]byte{
				0xc7,
				0xc0,
				0xc1,
				0xc0,
				0xc3,
				0xc0,
				0xc1,
				0xc0,
			},
		},
	}
	for _, tc := range cases {
		got := Encode(tc.item)
		if !bytes.Equal(tc.want, got) {
			t.Errorf("%s\nwant:\n%v\ngot:\n%v\n", tc.desc, tc.want, got)
		}
	}
}
