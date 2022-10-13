package rlp

import (
	"bytes"
	"encoding/binary"
	"math/rand"
	"reflect"
	"testing"
)

func randStr(n int) string {
	all := []rune("abcdefghijklmnopqrstuvwxyz")
	res := make([]rune, n)
	for i := range res {
		res[i] = all[rand.Intn(len(all))]
	}
	return string(res)
}

func FuzzEncodeList(f *testing.F) {
	var x, y uint8 = 1, 10
	f.Add(x, y)
	f.Fuzz(func(t *testing.T, numItems, itemLength uint8) {
		item := &Item{L: []*Item{}}
		for i := 0; i < int(numItems); i++ {
			item.L = append(item.L, &Item{D: []byte(randStr(int(itemLength)))})
		}
		got := Decode(Encode(item))
		if !reflect.DeepEqual(got, item) {
			t.Error("not equal")
		}
	})
}

func BenchmarkEncode(b *testing.B) {
	payload := []byte("hello world")
	b.ReportAllocs()
	for n := 0; n < b.N; n++ {
		Encode(&Item{D: payload})
	}
}

func intTo2b(i uint16) []byte {
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, i)
	return b
}

func TestDecode(t *testing.T) {
	cases := []struct {
		desc string
		item *Item
	}{
		{
			"short string",
			&Item{D: []byte("a")},
		},
		{
			"long string",
			&Item{D: []byte("Lorem ipsum dolor sit amet, consectetur adipisicing elit")},
		},

		{
			"empty list",
			&Item{L: []*Item{}},
		},
		{
			"list of short strings",
			&Item{
				L: []*Item{
					&Item{D: []byte("a")},
					&Item{D: []byte("b")},
				},
			},
		},
		{
			"list of long strings",
			&Item{
				L: []*Item{
					&Item{D: []byte("Lorem ipsum dolor sit amet, consectetur adipisicing elit")},
					&Item{D: []byte("Porem ipsum dolor sit amet, consectetur adipisicing elit")},
				},
			},
		},
		{
			"the set theoretical representation of three",
			&Item{
				L: []*Item{
					&Item{L: []*Item{}},
					&Item{L: []*Item{
						&Item{L: []*Item{}},
					}},
					&Item{L: []*Item{
						&Item{L: []*Item{}},
						&Item{L: []*Item{
							&Item{L: []*Item{}},
						}},
					}},
				},
			},
		},
	}
	for _, tc := range cases {
		got := Decode(Encode(tc.item))
		if !reflect.DeepEqual(tc.item, got) {
			t.Errorf("%s\nwant:\n%# v\ngot:\n%# v\n", tc.desc, tc.item, got)
		}
	}
}

func TestEncode(t *testing.T) {
	cases := []struct {
		desc string
		item *Item
		want []byte
	}{
		{
			"zero byte",
			&Item{D: []byte{byte(0)}},
			[]byte{0x00},
		},
		{
			"int 0",
			&Item{D: []byte{}},
			[]byte{0x80},
		},
		{
			"int 1024",
			&Item{D: intTo2b(1024)},
			[]byte{0x82, 0x04, 0x00},
		},
		{
			"empty string",
			&Item{D: []byte("")},
			[]byte{0x80},
		},
		{
			"non-empty string",
			&Item{D: []byte("Lorem ipsum dolor sit amet, consectetur adipisicing elit")},
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
			&Item{L: []*Item{}},
			[]byte{0xc0},
		},
		{
			"list of strings",
			&Item{
				L: []*Item{
					&Item{D: []byte("cat")},
					&Item{D: []byte("dog")},
				},
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
		{
			"the set theoretical representation of three",
			&Item{
				L: []*Item{
					&Item{L: []*Item{}},
					&Item{L: []*Item{
						&Item{L: []*Item{}},
					}},
					&Item{L: []*Item{
						&Item{L: []*Item{}},
						&Item{L: []*Item{
							&Item{L: []*Item{}},
						}},
					}},
				},
			},
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
