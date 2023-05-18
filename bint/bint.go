// big endian, uint64 binary encoding/decoding
package bint

import "github.com/holiman/uint256"

// Encodes a uint64 into a big-endian byte slice
// To avoid an allocation, or to have a padded result,
// supply an initialized value for b -otherwise use nil.
// Panics when provided slice is too small for n.
func Encode(b []byte, n uint64) []byte {
	s := size(n)
	if b == nil {
		b = make([]byte, s)
	}
	if int(s) > len(b) {
		panic("bint: supplied slice is too small for input")
	}
	for i := len(b) - 1; n > 0; i-- {
		b[i] = byte(n & 0xff)
		n = n >> 8
	}
	return b
}

func size(n uint64) (s uint8) {
	if n == 0 {
		return 1
	}
	for n > 0 {
		n = n >> 8
		s++
	}
	return
}

// Decodes big-endian byte array into a uint64
// left-padded zero bytes are ignored.
// Disregards extra bytes if len(b) > 8
func Decode(b []byte) uint64 {
	var n uint64
	for i := 0; i < len(b); i++ {
		n = n << 8
		n += uint64(b[i])
	}
	return n
}

func Uint16(b []byte) uint16 { return uint16(Decode(b)) }
func Uint32(b []byte) uint32 { return uint32(Decode(b)) }
func Uint64(b []byte) uint64 { return uint64(Decode(b)) }

func Uint256(b []byte) uint256.Int {
	var i uint256.Int
	i.SetBytes32(b)
	return i
}
