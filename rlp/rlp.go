// rlp encoding and decoding
//
// For a detailed description of RLP, see Ethereum's page:
// https://ethereum.org/en/developers/docs/data-structures-and-encoding/rlp/
package rlp

import (
	"github.com/indexsupply/x/bint"
)

const (
	str1L, str1H     byte = 000, 127
	str55L, str55H   byte = 128, 183
	strNL, strNH     byte = 184, 191
	list55L, list55H byte = 192, 247
	listNL, listNH   byte = 248, 255
)

// Encodes one or more items
// Will not encode a list. First call [Encode]
// on all the items in the list and then call
// [List] to add the list header.
func Encode(inputs ...[]byte) (res []byte) {
	for _, input := range inputs {
		switch n := len(input); {
		case n == 1 && input[0] == 0:
			res = append(res, 0x80)
		case n == 1 && input[0] <= str1H:
			res = append(res, input...)
		case n <= 55:
			res = append(res, append([]byte{str55L + byte(n)}, input...)...)
		default:
			length, lengthSize := encodeLength(len(input))
			header := append([]byte{str55H + lengthSize}, length...)
			res = append(res, append(header, input...)...)
		}
	}
	return
}

// Combines previously enocded items and
// add the list header.
// Callers should call [Encode] prior to calling [List]
func List(inputs ...[]byte) []byte {
	var out []byte
	for i := range inputs {
		out = append(out, inputs[i]...)
	}
	if len(out) <= 55 {
		return append([]byte{list55L + byte(len(out))}, out...)
	}
	length, lengthSize := encodeLength(len(out))
	header := append([]byte{list55H + lengthSize}, length...)
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

// Returns the RLP decoded value of a non-list, single item input
// Use [Iterator] and [Iterator.Bytes] for parsing a list
func Bytes(input []byte) []byte {
	it := Iter(input)
	return it.Bytes()
}

type Iterator struct {
	d    []byte
	i, n int
}

// Prepares an [Iterator] so that the caller can iterate through
// the values by calling [Iterator.Bytes].
// This function also works for a non-list, single value. Calling
// [Iterator.Bytes] will return the value.
func Iter(input []byte) Iterator {
	switch {
	case len(input) == 0:
		return Iterator{}
	case input[0] <= strNH:
		return Iterator{d: input}
	case input[0] <= list55H:
		i, n := 1, int(input[0]-list55L)
		return Iterator{d: input, i: i, n: n}
	default:
		i, n := decodeLength(list55H, input)
		return Iterator{d: input, i: i, n: n}
	}
}

func (it *Iterator) HasNext() bool {
	return it.i < it.n
}

// Returns the byte value of the current item and advances the iterator
// for the next item. Returns nil when there are no more items.
// If the underlying item is a list (meaning you are parsing a list
// of lists) then the returned bytes should be used to call [Iter]
// again.
func (it *Iterator) Bytes() []byte {
	if len(it.d) <= it.i {
		return nil
	}
	data := it.d[it.i:]
	switch b := data[0]; {
	case b <= str1H:
		it.i++
		return data[0:1]
	case b == 0x80:
		it.i++
		return []byte{0}
	case b <= str55H:
		m, n := 1, int(b-str55L)
		it.i += m + n
		return data[m : m+n]
	case b <= strNH:
		m, n := decodeLength(str55H, data)
		it.i += m + n
		return data[m : m+n]
	case b <= list55H:
		m, n := 1, int(b-list55L)
		it.i += m + n
		return data[:m+n]
	default:
		m, n := decodeLength(list55H, data)
		it.i += m + n
		return data[:m+n]
	}
}
