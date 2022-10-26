// This package implements a basic encoder/decoder for
// Ethereum's Recursive-Length Prefix (RLP) Serialization.
// For a detailed description of RLP, see Ethereum's  page:
// https://ethereum.org/en/developers/docs/data-structures-and-encoding/rlp/
package rlp

import (
	"encoding/binary"
	"errors"
	"math/big"
	"net/netip"
	"time"
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

var errNoData = errors.New("requested item contains 0 bytes")

func (i Item) NetIPAddr() (netip.Addr, error) {
	var a netip.Addr
	if len(i.d) == 0 {
		return a, errNoData
	}
	a, _ = netip.AddrFromSlice(i.d)
	return a, nil
}

func Time(t time.Time) Item {
	return Uint64(uint64(t.Unix()))
}

func Byte(b byte) Item {
	return Item{d: []byte{b}}
}

func Bytes(b []byte) Item {
	if b == nil {
		return Item{d: []byte{}}
	}
	return Item{d: b}
}

func String(s string) Item {
	return Item{d: []byte(s)}
}

func Int(n int) Item {
	bi := big.NewInt(int64(n))
	return Item{d: bi.Bytes()}
}

func Uint64(n uint64) Item {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf[:], n)
	return Item{d: buf[4:8]}
}

func Uint16(n uint16) Item {
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf[:], n)
	return Item{d: buf[:2]}
}

func (i Item) Uint16() (uint16, error) {
	if len(i.d) == 0 {
		return 0, errNoData
	}
	return binary.BigEndian.Uint16(i.d), nil
}

func (i Item) Hash() ([32]byte, error) {
	var h [32]byte
	if len(i.d) == 0 {
		return h, errNoData
	}
	if len(i.d) != 32 {
		return h, errors.New("hash must be exactly 32 bytes")
	}
	copy(h[:], i.d)
	return h, nil
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
		case n == 1 && input.d[0] <= str1H:
			return input.d
		case n <= 55:
			return append(
				[]byte{str55L + byte(n)},
				input.d...,
			)
		default:
			lengthSize, length := encodeLength(uint64(len(input.d)))
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
	lengthSize, length := encodeLength(uint64(len(out)))
	header := append(
		[]byte{list55H + lengthSize},
		length...,
	)
	return append(header, out...)
}

func encodeLength(n uint64) (uint8, []byte) {
	// Tommy's algorithm
	var buf []byte
	for i := n; i > 0; {
		buf = append([]byte{byte(i & 0xff)}, buf...)
		i = i >> 8
	}
	return uint8(len(buf)), buf
}

// Returns two values representing the length of the
// header and payload respectively.
func decodeLength(t byte, input []byte) (int, int) {
	n := input[0] - t
	paddedBytes := make([]byte, 8)
	// binary.BigEndian.Uint64 expects an 8 byte array so we have to left pad
	// it in case the length is less. Big-endian format is used.
	copy(paddedBytes[8-n:], input[1:n+1])
	length := binary.BigEndian.Uint64(paddedBytes)
	return int(n + 1), int(length)
}

var (
	errNoBytes      = errors.New("input has no bytes")
	errTooManyBytes = errors.New("input has more bytes than specified by outermost header")
	errTooFewBytes  = errors.New("input has fewer bytes than specified by header")
)

func Decode(input []byte) (Item, error) {
	if len(input) == 0 {
		return Item{}, errNoBytes
	}
	switch {
	case input[0] <= str1H:
		if len(input) > 1 {
			return Item{}, errTooManyBytes
		}
		return Item{d: []byte{input[0]}}, nil
	case input[0] <= str55H:
		i, n := 1, int(input[0]-str55L)
		switch {
		case len(input) < i+n:
			return Item{}, errTooFewBytes
		case len(input) > i+n:
			return Item{}, errTooManyBytes
		}
		return Item{d: input[i : i+n]}, nil
	case input[0] <= strNH:
		i, n := decodeLength(str55H, input)
		switch {
		case len(input) < i+n:
			return Item{}, errTooFewBytes
		case len(input) > i+n:
			return Item{}, errTooManyBytes
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
		case len(input[i:]) > listSize:
			return Item{}, errTooManyBytes
		case len(input[i:]) < listSize:
			return Item{}, errTooFewBytes
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
