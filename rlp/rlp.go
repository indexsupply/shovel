// This package implements a basic encoder/decoder for
// Ethereum's Recursive-Length Prefix (RLP) Serialization.
// For a detailed description of RLP, see Ethereum's  page:
// https://ethereum.org/en/developers/docs/data-structures-and-encoding/rlp/
package rlp

import (
	"encoding/binary"
	"errors"
)

const (
	str1L, str1H     byte = 000, 127
	str55L, str55H   byte = 128, 183
	strNL, strNH     byte = 184, 191
	list55L, list55H byte = 192, 247
	listNL, listNH   byte = 248, 255
)

// Instead of using standard data types and reflection
// this package chooses to encode Items.
// Set D or L but not both.
// L is a list of *Item so that arbitrarily nested lists
// can be encoded.
// D is the data payload for the item.
type Item struct {
	D []byte
	L []*Item
}

var (
	ErrTooManyArgs = errors.New("must set D xor L")
	ErrTooFewArgs  = errors.New("input Item is nil")
)

func Encode(input *Item) ([]byte, error) {
	if input == nil {
		return nil, ErrTooFewArgs
	}
	if input.D != nil && input.L != nil {
		return nil, ErrTooManyArgs
	}
	if input.D != nil {
		switch n := len(input.D); {
		case n == 1 && input.D[0] <= str1H:
			return input.D, nil
		case n <= 55:
			return append(
				[]byte{str55L + byte(n)},
				input.D...,
			), nil
		default:
			return append(
				encodeLength(str55H, len(input.D)),
				input.D...,
			), nil
		}
	}

	var out []byte
	for i := range input.L {
		b, err := Encode(input.L[i])
		if err != nil {
			return nil, err
		}
		out = append(out, b...)
	}
	if len(out) <= 55 {
		return append(
			[]byte{list55L + byte(len(out))},
			out...,
		), nil
	}
	return append(
		encodeLength(list55H, len(out)),
		out...,
	), nil
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

// returns length of item including the header
func decodeLength(t byte, input []byte) int {
	n := input[0] - t
	length, _ := binary.Uvarint(input[1 : n+1])
	return int(n) + int(length)
}

var (
	ErrNoBytes      = errors.New("input has no bytes")
	ErrTooManyBytes = errors.New("input has more bytes than specified by outermost header")
	ErrTooFewBytes  = errors.New("input has fewer bytes than specified by header")
)

func Decode(input []byte) (*Item, int, error) {
	if len(input) == 0 {
		return nil, 0, ErrNoBytes
	}
	switch {
	case input[0] <= str1H:
		if len(input) > 1 {
			return nil, 0, ErrTooManyBytes
		}
		return &Item{D: []byte{input[0]}}, 1, nil
	case input[0] <= str55H:
		var (
			i = 1
			n = int(input[0] - str1H)
		)
		switch {
		case n > len(input):
			return nil, 0, ErrTooManyBytes
		case n < len(input)-1:
			return nil, 0, ErrTooFewBytes
		}
		return &Item{D: input[i:n]}, n, nil
	case input[0] <= strNH:
		var (
			i = int(1 + (input[0] - str55H))    //head length
			n = 1 + decodeLength(str55H, input) // add 1 for header byte
		)
		switch {
		case n > len(input):
			return nil, 0, ErrTooManyBytes
		case n < len(input)-1:
			return nil, 0, ErrTooFewBytes
		}
		return &Item{D: input[i:n]}, n, nil
	default:
		// The first byte indicates a list
		// and if the first byte is >= 248 (listNL)
		// then the list has a length > 55 and
		// therefore the next (input[0]-247 (list55H))
		// bytes will describe the length of the list.
		// We advance the cursor i past the length description.
		// We also compute the size of the list and check the input
		// to ensure the size matches the header's length description.
		var i, listSize int
		switch {
		case input[0] <= list55H:
			i = 1
			listSize = int(input[0] - list55L)
		case input[0] <= listNH:
			i = 1 //header byte
			i += int(input[0] - list55H)
			listSize = decodeLength(list55H, input)
			listSize-- //account for header byte
		}

		switch {
		case listSize < len(input[i:]):
			return nil, 0, ErrTooManyBytes
		case listSize > len(input[i:]):
			return nil, 0, ErrTooFewBytes
		}

		var bytesRead int
		item := &Item{L: []*Item{}}
		for i < len(input) {
			var n int
			switch {
			case input[i] <= str1H:
				// 1 byte string
				n = 1
			case input[i] <= str55H:
				// <= 55 byte string
				n += 1 //header byte
				n += int(input[i] - str55L)
			case input[i] <= strNH:
				// > 55 byte string
				n += 1 //header byte
				n += decodeLength(str55H, input[i:])
			case input[i] <= list55H:
				n += 1 // header byte
				n += int(input[i] - list55L)
			default:
				n += 1 //header byte
				n += decodeLength(list55H, input[i:])
			}

			if int(i+n) > len(input) {
				return nil, bytesRead, ErrTooFewBytes
			}

			d, nb, err := Decode(input[i : i+n])
			if err != nil {
				return nil, nb, err
			}
			item.L = append(item.L, d)
			i += n
			bytesRead += nb
		}
		return item, bytesRead, nil
	}
}
