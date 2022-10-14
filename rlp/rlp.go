package rlp

import "encoding/binary"

const (
	str1L, str1H   byte = 000, 127
	str5L, str5H   byte = 128, 183
	strNL, strNH   byte = 184, 191
	list5L, list5H byte = 192, 247
	listNL, listNH byte = 248, 255
)

type Item struct {
	D []byte
	L []*Item
}

func Encode(input *Item) []byte {
	if input.D != nil {
		switch n := len(input.D); {
		case n == 1 && input.D[0] <= str1H:
			return input.D
		case n < 56:
			return append(
				[]byte{str5L + byte(n)},
				input.D...,
			)
		default:
			return append(
				encodeLength(str5H, len(input.D)),
				input.D...,
			)
		}
	}

	var out []byte
	for i := range input.L {
		out = append(out, Encode(input.L[i])...)
	}
	if len(out) < 56 {
		return append(
			[]byte{list5L + byte(len(out))},
			out...,
		)
	}
	return append(
		encodeLength(list5H, len(out)),
		out...,
	)
}

func encodeLength(t byte, l int) []byte {
	// header must be <= 8 bytes
	buf := make([]byte, 8)
	// bytes needed to encode length
	n := binary.PutUvarint(buf, uint64(l))
	return append(
		[]byte{byte(uint8(t) + uint8(n))},
		buf[:n]...,
	)
}

func decodeLength(t byte, input []byte) int {
	n := input[0] - t
	length, _ := binary.Uvarint(input[1 : n+1])
	return int(n) + int(length)
}

func Decode(input []byte) *Item {
	switch {
	case input[0] <= str1H:
		return &Item{D: []byte{input[0]}}
	case input[0] <= str5H:
		n := input[0] - str1H
		return &Item{D: input[1:n]}
	case input[0] <= strNH:
		headerSize := 1 + input[0] - str5H
		return &Item{D: input[headerSize:]}
	default:
		// The first byte indicates a list
		// and if the first byte is > 247 then
		// the list has a length of > 55 and
		// therefore the next (input[0] - 247)
		// bytes will represent the length of the list.
		// We advance the cursor past the length.
		i := 1
		if input[0] >= listNL {
			i += int(input[0] - list5H)
		}

		item := &Item{L: []*Item{}}
		for i < len(input) {
			var n int
			switch {
			case input[i] <= str1H:
				// 1 byte string
				n = 1
			case input[i] <= str5H:
				// < 55 byte string
				n += 1 //header byte
				n += int(input[i] - str5L)
			case input[i] <= strNH:
				// > 55 byte string
				n += 1 //header byte
				n += decodeLength(str5H, input[i:])
			case input[i] <= list5H:
				n += 1 // header byte
				n += int(input[i] - list5L)
			default:
				n += 1 //header byte
				n += decodeLength(list5H, input[i:])
			}

			item.L = append(item.L, Decode(input[i:i+n]))
			i += n
		}
		return item
	}
}
