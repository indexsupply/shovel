// M3:2048 Bloom filter
//
// Useful for reducing a log entry into a single 256-byte hash
package bloom

/*
Bloom filter implementation based on the Yello Paper/4.3.1:

	M3:2048 is a specialised Bloom filter that sets three
	bits out of 2048, given an arbitrary byte sequence. It does
	this through taking the low-order 11 bits of each of the
	first three pairs of bytes in a Keccak-256 hash of the byte
	sequence.
*/
type Filter [256]byte

// (32) m(x,i) ≡ KEC(x)[i, i + 1] mod 2048
func m(x []byte, i int) uint16 {
	return (uint16(x[0])<<8 | uint16(x[1])) & 0x7ff
}

// returns byte position [0,255] based on m(x,i)
func byteIndex(m uint16) uint8 {
	return uint8((2047 - m) / 8)
}

// returns bit position [0,8] based on m(x,i)
func bitIndex(m uint16) uint8 {
	return uint8(m % 8)
}

// Returns true if the data isn't in the filter.
// Returns false when the data might be in the filter.
//
// Caller is responsible for keccak hashing d
func (bf Filter) Missing(d []byte) bool {
	m1, m2, m3 := m(d, 0), m(d, 2), m(d, 4)
	b1 := bf[byteIndex(m1)]&(1<<(bitIndex(m1))) == 0
	b2 := bf[byteIndex(m2)]&(1<<(bitIndex(m2))) == 0
	b3 := bf[byteIndex(m3)]&(1<<(bitIndex(m3))) == 0
	return b1 && b2 && b3
}

// Sets appropriate bits in bf.
// Caller is responsible for keccak hashing d
// and concurrency control.
func (bf *Filter) Add(d []byte) {
	m1, m2, m3 := m(d, 0), m(d, 2), m(d, 4)
	bf[byteIndex(m1)] |= (1 << bitIndex(m1))
	bf[byteIndex(m2)] |= (1 << bitIndex(m2))
	bf[byteIndex(m3)] |= (1 << bitIndex(m3))
}
