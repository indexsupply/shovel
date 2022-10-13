package rlp

import (
	"encoding/binary"
	"math"
)

type Item struct {
	D []byte
	L []*Item
}

func Encode(input *Item) []byte {
	if input.D != nil {
		if len(input.D) == 1 && input.D[0] < 128 {
			return input.D
		}
		return append(encodeLength(input.D, 128), input.D...)
	}

	var out []byte
	for i := range input.L {
		out = append(out, Encode(input.L[i])...)
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
			// header must be <= 8 bytes
			buf    = make([]byte, 8)
			n      = binary.PutUvarint(buf, l) //bytes needed to encode length
			header = int(offset) + 55 + n
		)
		return append([]byte{byte(header)}, buf[:n]...)
	default:
		return []byte{}
	}
}

func decodeLength(input []byte, offset uint64) uint64 {
	n := uint64(input[0]) - offset
	length, _ := binary.Uvarint(input[1 : n+1])
	return n + length
}

func Decode(input []byte) *Item {
	switch {
	case input[0] < 128: // string
		return &Item{D: []byte{input[0]}}
	case input[0] < 184: // string
		return &Item{D: input[1:]}
	case input[0] < 192: // string
		headerSize := 1 + input[0] - 183
		return &Item{D: input[headerSize:]}
	default:
		// The first byte indicates a list
		// and if the first byte is > 247 then
		// the list has a length of > 55 and
		// therefore the next (input[0] - 247)
		// bytes will represent the length of the list.
		// We advance the cursor past the length.
		var i uint64 = 1
		if input[0] > 247 {
			i += uint64(input[0]) - 247
		}

		item := &Item{L: []*Item{}}
		for i < uint64(len(input)) {
			var n uint64
			switch {
			case input[i] < 128:
				// 1 byte string
				n = 1
			case input[i] < 184:
				// < 55 byte string
				n += 1 //header byte
				n += uint64(input[i]) - 128
			case input[i] < 192:
				// > 55 byte string
				n += 1 //header byte
				n += decodeLength(input[i:], 183)
			case input[i] < 248:
				n += 1 // header byte
				n += uint64(input[i]) - 192
			default:
				n += 1 //header byte
				n += decodeLength(input[i:], 247)
			}

			item.L = append(item.L, Decode(input[i:i+n]))
			i += n
		}
		return item
	}
}
