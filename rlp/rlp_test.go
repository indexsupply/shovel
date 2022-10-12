package rlp

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func intTo2b(i uint16) []byte {
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, i)
	return b
}

func TestEncode(t *testing.T) {
	cases := []struct {
		item *Item
		want []byte
	}{
		{
			&Item{D: intTo2b(1024)},
			[]byte{0x82, 0x04, 0x00},
		},
		{
			&Item{
				L: []*Item{
					&Item{D: []byte("cat")},
					&Item{D: []byte("dog")},
				},
			},
			[]byte{
				0xc8,
				0x83,
				0x63, //c
				0x61, //a
				0x74, //t
				0x83,
				0x64, //d
				0x6f, //o
				0x67, //g
			},
		},
		{
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
		got := encode(tc.item)
		if !bytes.Equal(tc.want, got) {
			t.Errorf("want:\n%v\ngot:\n%v\n", tc.want, got)
		}
	}
}
