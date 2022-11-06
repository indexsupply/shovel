package rlp

import (
	"encoding/binary"
	"errors"
	"net/netip"
	"time"
)

var errNoData = errors.New("requested item contains 0 bytes")

func Bytes(b []byte) Item {
	if b == nil {
		return Item{d: []byte{}}
	}
	return Item{d: b}
}

func (i Item) Bytes() ([]byte, error) {
	if len(i.d) == 0 {
		return nil, errNoData
	}
	return i.d, nil
}

func Uint16(n uint16) Item {
	_, b := encodeUint(uint64(n))
	return Item{d: b}
}

func (i Item) Uint16() (uint16, error) {
	if len(i.d) == 0 {
		return 0, errNoData
	}
	return binary.BigEndian.Uint16(leftPad(i.d, 2)), nil
}

func Uint64(n uint64) Item {
	_, b := encodeUint(n)
	return Item{d: b}
}

func (i Item) Uint64() (uint64, error) {
	if len(i.d) == 0 {
		return 0, errNoData
	}
	return binary.BigEndian.Uint64(leftPad(i.d, 8)), nil
}

func String(s string) Item {
	return Item{d: []byte(s)}
}

func (i Item) String() (string, error) {
	if len(i.d) == 0 {
		return "", errNoData
	}
	return string(i.d), nil
}

func (i Item) Bytes33() ([33]byte, error) {
	var b [33]byte
	if len(i.d) == 0 {
		return b, errNoData
	}
	if len(i.d) != 33 {
		return b, errors.New("must be exactly 33 bytes")
	}
	copy(b[:], i.d)
	return b, nil
}

func (i Item) Hash() ([32]byte, error) {
	var h [32]byte
	if len(i.d) == 0 {
		return h, errNoData
	}
	if len(i.d) != 32 {
		return h, errors.New("hash must be exactly 32 bytes")
	}
	copy(h[:], i.d)
	return h, nil
}

func (i Item) NetIPAddr() (netip.Addr, error) {
	var a netip.Addr
	if len(i.d) == 0 {
		return a, errNoData
	}
	a, _ = netip.AddrFromSlice(i.d)
	return a, nil
}

func Time(t time.Time) Item {
	return Uint64(uint64(t.Unix()))
}

func Byte(b byte) Item {
	if b == 0 {
		return Item{d: []byte{}}
	}
	return Item{d: []byte{b}}
}

func Int(n int) Item {
	_, b := encodeUint(uint64(n))
	return Item{d: b}
}

// left pads the provided byte array to the wantedLength, in bytes, using 0s.
// does nothing if b is already at the wanted length.
func leftPad(b []byte, wantedLength int) []byte {
	if len(b) >= wantedLength {
		return b
	}
	padded := make([]byte, wantedLength)
	bytesNeeded := wantedLength - len(b)
	copy(padded[bytesNeeded:], b)
	return padded
}
