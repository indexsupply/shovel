package rlp

import (
	"bytes"
	"testing"
)

func TestEncode(t *testing.T) {
	cases := []struct {
		item *Item
		want []byte
	}{
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
