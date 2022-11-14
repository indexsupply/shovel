// big endian, uint64 binary encoding/decoding
package bint

// Encodes a uint64 into a big-endian byte slice
// To avoid an allocation, or to have a padded result,
// supply an initialized value for b -otherwise use nil.
func Encode(b []byte, n uint64) ([]byte, uint8) {
	if b == nil {
		b = make([]byte, size(n))
	}
	for i := len(b) - 1; n > 0; i-- {
		b[i] = byte(n & 0xff)
		n = n >> 8
	}
	return b, uint8(len(b))
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
func Decode(b []byte) uint64 {
	var n uint64
	for i := 0; i < len(b); i++ {
		n = n << 8
		n += uint64(b[i])
	}
	return n
}
