package rlp

import (
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

func (i Item) Bytes() []byte {
	return i.d
}

func Uint16(n uint16) Item {
	return Item{d: bint.Encode(nil, uint64(n))}
}

func (i Item) Uint16() uint16 {
	return uint16(bint.Decode(i.d))
}

func Uint64(n uint64) Item {
	return Item{d: bint.Encode(nil, n)}
}

func (i Item) Uint64() uint64 {
	return bint.Decode(i.d)
}

func String(s string) Item {
	return Item{d: []byte(s)}
}

func (i Item) String() string {
	return string(i.d)
}

func Time(t time.Time) Item {
	return Uint64(uint64(t.Unix()))
}

func (i Item) Time() time.Time {
	return time.Unix(int64(i.Uint64()), 0)
}

// Uncompressed secpk256k1 public key
func Secp256k1PublicKey(pubk *secp256k1.PublicKey) Item {
	b := isxsecp256k1.Encode(pubk)
	return Bytes(b[:])
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

func (i Item) Bytes32() ([32]byte, error) {
	if len(i.d) != 32 {
		return [32]byte{}, errors.New("must be exactly 32 bytes")
	}
	return *(*[32]byte)(i.d), nil
}

func (i Item) Bytes65() ([65]byte, error) {
	if len(i.d) != 65 {
		return [65]byte{}, errors.New("must be exactly 65 bytes")
	}
	return *(*[65]byte)(i.d), nil
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
