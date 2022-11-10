package bint

func Encode(n uint64) ([]byte, uint8) {
	if n == 0 {
		return []byte{}, 0
	}
	// Tommy's algorithm
	var buf []byte
	for i := n; i > 0; {
		buf = append([]byte{byte(i & 0xff)}, buf...)
		i = i >> 8
	}
	return buf, uint8(len(buf))
}
