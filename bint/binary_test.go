package bint

import (
	"bytes"
	"testing"
)

func TestDecode(t *testing.T) {
	var cases = []uint8{8, 16, 32, 64}
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

func TestDecode_0(t *testing.T) {
	b, n := Encode(nil, 0)
	if n != 1 {
		t.Errorf("expected 0 to be 1 byte got: %d", n)
	}
	if b[0] != 0 {
		t.Errorf("expected 0 to encode to 0x00 bot: %x", b[0])
	}

	got := Decode(b)
	exp := uint64(0)
	if exp != got {
		t.Errorf("expected: %d got: %d", exp, got)
	}
}

func TestDecode_Pad(t *testing.T) {
	got := Decode([]byte{0x00, 0x00, 0x01, 0x00})
	exp := uint64(256)
	if exp != got {
		t.Errorf("expected: %d got: %d", exp, got)
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
