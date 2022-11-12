package bint

import (
	"bytes"
	"testing"
)

func TestDecode(t *testing.T) {
	var cases = []uint8{0, 8, 16, 32, 64}
	for _, e := range cases {
		i := uint64(1<<e - 1)
		b, n := Encode(nil, i)
		if n != e/8 {
			t.Errorf("num bytes expected %d got: %d", e/8, n)
		}
		got := Decode(b)
		if got != i {
			t.Errorf("expected %d got: %d", i, got)
		}
	}
}

func TestEncodeNil(t *testing.T) {
	i := uint64(1<<16 - 1)
	b, n := Encode(nil, i)
	if n != 2 {
		t.Errorf("expected %d to use 2 bytes. got: %d", i, n)
	}
	if !bytes.Equal(b, []byte{0xff, 0xff}) {
		t.Errorf("expected %d to be encoded as: ffff got: %x", i, b)
	}
}

func TestEncodeBuf(t *testing.T) {
	b := make([]byte, 2)
	i := uint64(1<<16 - 1)
	_, n := Encode(b[:], i)
	if n != 2 {
		t.Errorf("expected %d to use 2 bytes. got: %d", i, n)
	}
	if !bytes.Equal(b, []byte{0xff, 0xff}) {
		t.Errorf("expected %d to be encoded as: ffff got: %x", i, b)
	}

}

func TestEncodeBuf_Pad(t *testing.T) {
	b := make([]byte, 4)
	i := uint64(1<<16 - 1)
	_, n := Encode(b[:], i)
	if n != 4 {
		t.Errorf("expected %d to use 2 bytes. got: %d", i, n)
	}
	if !bytes.Equal(b, []byte{0xff, 0xff, 0x00, 0x00}) {
		t.Errorf("expected %d to be encoded as: ffff got: %x", i, b)
	}
}
