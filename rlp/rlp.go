package rlp

import (
	"math"
	"math/bits"
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
		// range of first byte in decimal: [128, 183]
		return []byte{byte(uint64(offset) + l)}
	case l <= math.MaxUint64:
		// The following calculation finds the number
		// of bytes needed to represent the length.
		// This is a standard way to compute the cieling
		// of the length in bits divided by the number
		// of bits in a byte.
		n := 1 + ((bits.Len(64) - 1) / 8)
		// range of first byte in decimal: [184, 191]
		return []byte{
			byte(int(offset) + 55 + n),
			byte(l),
		}
	default:
		return []byte{}
	}
}
