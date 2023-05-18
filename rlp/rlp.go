// This package implements a basic encoder/decoder for
// Ethereum's Recursive-Length Prefix (RLP) Serialization.
// For a detailed description of RLP, see Ethereum's  page:
// https://ethereum.org/en/developers/docs/data-structures-and-encoding/rlp/
package rlp

import (
	"bytes"

	"github.com/indexsupply/x/bint"
)

const (
	str1L, str1H     byte = 000, 127
	str55L, str55H   byte = 128, 183
	strNL, strNH     byte = 184, 191
	list55L, list55H byte = 192, 247
	listNL, listNH   byte = 248, 255
)

func Encode(input []byte) []byte {
	switch n := len(input); {
	case n == 1 && input[0] == 0:
		return []byte{0x80}
	case n == 1 && input[0] <= str1H:
		return input
	case n <= 55:
		return append(
			[]byte{str55L + byte(n)},
			input...,
		)
	default:
		length, lengthSize := encodeLength(len(input))
		header := append(
			[]byte{str55H + lengthSize},
			length...,
		)
		return append(header, input...)
	}
}

func EncodeList(inputs ...[]byte) []byte {
	var out []byte
	for _, input := range inputs {
		out = append(out, Encode(input)...)
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

// Removes RLP encoding data and returns the encoded bytes
// See [Iterator] for decoding a list of RLP data
func Bytes(input []byte) []byte {
	switch {
	case len(input) == 0:
		return nil
	case bytes.Equal(input, []byte{0x80}):
		return nil
	case input[0] <= str1H:
		return input[0:1]
	case input[0] <= str55H:
		i, n := 1, int(input[0]-str55L)
		if len(input) < i+n {
			return nil
		}
		return input[i : i+n]
	case input[0] <= strNH:
		i, n := decodeLength(str55H, input)
		if len(input) < i+n {
			return nil
		}
		return input[i : i+n]
	default:
		return input
	}
}

// For iterating over an RLP list
type Iterator struct {
	i    int
	data []byte
}

// Returns a new iter. Input is assumed to be
// raw RLP bytes. The input size bytes are removed
// from the beginning of input and trailing bytes
// are removed.
//
// A nil [Iterator] is returned when the
// input doesn't contain a list or
// the list data is corrupt.
func Iter(input []byte) *Iterator {
	if len(input) == 0 {
		return nil
	}
	if input[0] <= strNH { // not a list
		return nil
	}
	var i, listSize int
	switch {
	case input[0] <= list55H:
		i, listSize = 1, int(input[0]-list55L)
	case input[0] <= listNH:
		i, listSize = decodeLength(list55H, input)
	}
	if len(input[i:]) < listSize {
		return nil
	}
	//ignore bytes beyond listSize
	return &Iterator{data: input[i : i+listSize]}
}

func size(input []byte) int {
	var headerSize, payloadSize int
	switch {
	case input[0] <= str1H:
		headerSize = 0
		payloadSize = 1
	case input[0] <= str55H:
		headerSize = 1
		payloadSize = int(input[0] - str55L)
	case input[0] <= strNH:
		headerSize, payloadSize = decodeLength(str55H, input[0:])
	case input[0] <= list55H:
		headerSize = 1
		payloadSize = int(input[0] - list55L)
	default:
		headerSize, payloadSize = decodeLength(list55H, input[0:])
	}
	return headerSize + payloadSize
}

func (it *Iterator) HasNext() bool {
	if it == nil {
		return false
	}
	return len(it.data[it.i:]) > 0
}

// Moves the iter forward by one
// and calls [Bytes] on the underlying value
// Returns nil if there is no more data to scan
// of if the RLP size header is corrupt
func (it *Iterator) Bytes() []byte {
	if it == nil {
		return nil
	}
	if it.i > len(it.data) {
		return nil
	}
	data := it.data[it.i:]
	ln := len(data)
	if ln == 0 {
		return nil
	}
	sz := size(data)
	if ln < sz {
		return nil
	}
	it.i += sz
	return Bytes(data[:sz])
}
