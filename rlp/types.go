package rlp

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/indexsupply/x/bint"
	"github.com/indexsupply/x/isxsecp256k1"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
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
	return Item{d: bint.Encode(nil, uint64(n))}
}

func (i Item) Uint16() (uint16, error) {
	if len(i.d) == 0 {
		return 0, errNoData
	}
	return binary.BigEndian.Uint16(leftPad(i.d, 2)), nil
}

func Uint64(n uint64) Item {
	return Item{d: bint.Encode(nil, n)}
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

func Time(t time.Time) Item {
	return Uint64(uint64(t.Unix()))
}

func (i Item) Time() (time.Time, error) {
	var t time.Time
	ts, err := i.Uint64()
	if err != nil {
		return t, err
	}
	return time.Unix(int64(ts), 0), nil
}

func (i Item) Secp256k1PublicKey() (*secp256k1.PublicKey, error) {
	switch len(i.d) {
	case 0:
		return nil, errNoData
	case 33:
		var b [33]byte
		copy(b[:], i.d)
		return isxsecp256k1.DecodeCompressed(b)
	case 64:
		var b [64]byte
		copy(b[:], i.d)
		return isxsecp256k1.Decode(b)
	default:
		return nil, errors.New(fmt.Sprintf("secp256k1 pubkey must be 33 or 64 bytes. got: %d", len(i.d)))
	}
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

func (i Item) IP() (net.IP, error) {
	switch len(i.d) {
	case 0:
		return nil, errNoData
	case 4, 16:
		return net.IP(i.d), nil
	default:
		return nil, errors.New(fmt.Sprintf("ip must be 4 or 16 bytes. got: %d", len(i.d)))
	}
}

func Byte(b byte) Item {
	if b == 0 {
		return Item{d: []byte{}}
	}
	return Item{d: []byte{b}}
}

func Int(n int) Item {
	return Item{d: bint.Encode(nil, uint64(n))}
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
