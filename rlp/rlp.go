package rlp

import (
	"encoding/binary"
	"math"
)

type Item struct {
	D []byte
	L []*Item
}

func encode(input *Item) []byte {
	if input.D != nil {
		if len(input.D) == 1 && input.D[0] < 128 {
			return input.D
		}
		return append(encodeLength(input.D, 128), input.D...)
	}

	var out []byte
	for i := range input.L {
		out = append(out, encode(input.L[i])...)
	}
	return append(encodeLength(out, 192), out...)
}

func encodeLength(input []byte, offset uint8) []byte {
	switch l := uint64(len(input)); {
	case l < 56:
		header := uint64(offset) + l
		return []byte{byte(header)} // (max(128,192) + 55) < 255
	case l <= math.MaxUint64:
		var (
			buf    = make([]byte, binary.MaxVarintLen64)
			n      = binary.PutUvarint(buf, l) //bytes needed to encode length
			header = int(offset) + 55 + n
		)
		return append([]byte{byte(header)}, buf[:n]...)
	default:
		return []byte{}
	}
}
