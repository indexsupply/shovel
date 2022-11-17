// This package implements a basic encoder/decoder for
// Ethereum's Recursive-Length Prefix (RLP) Serialization.
// For a detailed description of RLP, see Ethereum's  page:
// https://ethereum.org/en/developers/docs/data-structures-and-encoding/rlp/
package rlp

import (
	"errors"

	"github.com/indexsupply/x/bint"
)

const (
	str1L, str1H     byte = 000, 127
	str55L, str55H   byte = 128, 183
	strNL, strNH     byte = 184, 191
	list55L, list55H byte = 192, 247
	listNL, listNH   byte = 248, 255
)

func List(items ...Item) Item {
	if items == nil {
		items = []Item{}
	}
	return Item{l: items}
}

func (i Item) At(pos int) Item {
	if len(i.l) < pos {
		return Item{}
	}
	return i.l[pos]
}

func (i Item) List() []Item {
	return i.l
}

// Instead of using standard data types and reflection
// this package chooses to encode Items.
// Set d or l but not both.
// l is a list of Item for arbitrarily nested lists.
// d is the data payload for the item.
type Item struct {
	d []byte
	l []Item
}

func Encode(input Item) []byte {
	if input.d != nil && input.l != nil {
		panic("must set d xor l")
	}
	if input.d != nil {
		switch n := len(input.d); {
		case n == 1 && input.d[0] == 0:
			return []byte{0x80}
		case n == 1 && input.d[0] <= str1H:
			return input.d
		case n <= 55:
			return append(
				[]byte{str55L + byte(n)},
				input.d...,
			)
		default:
			length, lengthSize := encodeLength(len(input.d))
			header := append(
				[]byte{str55H + lengthSize},
				length...,
			)
			return append(header, input.d...)
		}
	}

	var out []byte
	for _, l := range input.l {
		out = append(out, Encode(l)...)
	}
	if len(out) <= 55 {
		return append(
			[]byte{list55L + byte(len(out))},
			out...,
		)
	}
	length, lengthSize := encodeLength(len(out))
	header := append(
		[]byte{list55H + lengthSize},
		length...,
	)
	return append(header, out...)
}

func encodeLength(n int) ([]byte, uint8) {
	if n == 0 {
		return []byte{}, 0
	}
	b := bint.Encode(nil, uint64(n))
	return b, uint8(len(b))
}

// Returns two values representing the length of the
// header and payload respectively.
func decodeLength(t byte, input []byte) (int, int) {
	n := (input[0] - t) + 1 //add 1 byte for length size (input[0])
	l := bint.Decode(input[1:n])
	return int(n), int(l)
}

var (
	errNoBytes     = errors.New("input has no bytes")
	errTooFewBytes = errors.New("input has fewer bytes than specified by header")
)

func Decode(input []byte) (Item, error) {
	if len(input) == 0 {
		return Item{}, errNoBytes
	}
	switch {
	case input[0] <= str1H:
		return Item{d: []byte{input[0]}}, nil
	case input[0] <= str55H:
		i, n := 1, int(input[0]-str55L)
		if len(input) < i+n {
			return Item{}, errTooFewBytes
		}
		return Item{d: input[i : i+n]}, nil
	case input[0] <= strNH:
		i, n := decodeLength(str55H, input)
		if len(input) < i+n {
			return Item{}, errTooFewBytes
		}
		return Item{d: input[i : i+n]}, nil
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
		case len(input[i:]) < listSize:
			return Item{}, errTooFewBytes
		case len(input[i:]) > listSize:
			// It's possible that the input contains
			// more bytes that is specified by the
			// header's length. In this case, instead
			// of returning an error, we simply remove
			// the extra bytes.
<<<<<<< HEAD
			input = input[:i+listSize]
=======
			input = input[: i+listSize]
>>>>>>> 01b3363 (fix test)
		}

		item := Item{l: []Item{}}
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
				return Item{}, errTooFewBytes
			}

			d, err := Decode(input[i : i+headerSize+payloadSize])
			if err != nil {
				return Item{}, err
			}
			item.l = append(item.l, d)
			i += headerSize + payloadSize
		}
		return item, nil
	}
}
