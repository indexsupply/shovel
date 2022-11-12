package bint

// Encodes a uint64 into a big-endian byte slice
// To avoid an allocation, or to have a padded result,
// supply an initialized value for b -otherwise use nil.
func Encode(b []byte, n uint64) ([]byte, uint8) {
	if b == nil {
		b = make([]byte, size(n))
	}
	if n == 0 {
		return b, 0
	}
	for i := uint64(0); n > 0; i++ {
		b[i] = byte(n & 0xff)
		n = n >> 8
	}
	return b, uint8(len(b))
}

func size(n uint64) uint8 {
	var s uint8
	for n > 0 {
		n = n >> 8
		s++
	}
	return s
}

// Decodes big-endian byte array into a uint64
// Right-padded zero bytes are ignored.
func Decode(b []byte) uint64 {
	var n uint64
	for i := 0; i < len(b) && b[i] != 0; i++ {
		n = n << 8
		n += uint64(b[i])
	}
	return n
}
