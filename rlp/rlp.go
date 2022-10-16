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

// Returns  two values representing the length of the
// header and payload respectively.
func decodeLength(t byte, input []byte) (int, int) {
	n := input[0] - t
	paddedBytes := make([]byte, 8)
	// binary.BigEndian.Uint64 expects an 8 byte array so we have to left pad
	// it in case the length is less. Big-endian format is used.
	copy(paddedBytes[8-n:], input[1 : n+1])
	length := binary.BigEndian.Uint64(paddedBytes)
	return int(n + 1), int(length)
}

var (
	ErrNoBytes      = errors.New("input has no bytes")
	ErrTooManyBytes = errors.New("input has more bytes than specified by outermost header")
	ErrTooFewBytes  = errors.New("input has fewer bytes than specified by header")
)

func Decode(input []byte) (*Item, error) {
	if len(input) == 0 {
		return nil, ErrNoBytes
	}
	switch {
	case input[0] <= str1H:
		if len(input) > 1 {
			return nil, ErrTooManyBytes
		}
		return &Item{D: []byte{input[0]}}, nil
	case input[0] <= str55H:
		i, n := 1, int(input[0]-str55L)
		switch {
		case len(input) < i+n:
			return nil, ErrTooFewBytes
		case len(input) > i+n:
			return nil, ErrTooManyBytes
		}
		return &Item{D: input[i : i+n]}, nil
	case input[0] <= strNH:
		i, n := decodeLength(str55H, input)
		switch {
		case len(input) < i+n:
			return nil, ErrTooFewBytes
		case len(input) > i+n:
			return nil, ErrTooManyBytes
		}
		return &Item{D: input[i : i+n]}, nil
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
			i, listSize = 1, int(input[0]-list55L)
		case input[0] <= listNH:
			i, listSize = decodeLength(list55H, input)
		}

		switch {
		case len(input[i:]) > listSize:
			return nil, ErrTooManyBytes
		case len(input[i:]) < listSize:
			return nil, ErrTooFewBytes
		}

		item := &Item{L: []*Item{}}
		for i < len(input) {
			var headerSize, payloadSize int
			switch {
			case input[i] <= str1H:
				headerSize = 0
				payloadSize = 1
			case input[i] <= str55H:
				headerSize = 1
				payloadSize = int(input[i] - str55L)
			case input[i] <= strNH:
				headerSize, payloadSize = decodeLength(str55H, input[i:])
			case input[i] <= list55H:
				headerSize = 1
				payloadSize = int(input[i] - list55L)
			default:
				headerSize, payloadSize = decodeLength(list55H, input[i:])
			}

			if int(i+headerSize+payloadSize) > len(input) {
				return nil, ErrTooFewBytes
			}

			d, err := Decode(input[i : i+headerSize+payloadSize])
			if err != nil {
				return nil, err
			}
			item.L = append(item.L, d)
			i += headerSize + payloadSize
		}
		return item, nil
	}
}
