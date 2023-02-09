// ABI encoding/decoding with log parsing
//
// Implementation based on the [ABI Spec].
//
// [ABI Spec]: https://docs.soliditylang.org/en/latest/abi-spec.html
package abi

import (
	"bytes"

	"github.com/indexsupply/x/abi/schema"
	"github.com/indexsupply/x/bint"
)

type Log struct {
	Address [20]byte
	Topics  [4][32]byte
	Data    []byte
}

type Item struct {
	schema.Type
	d []byte
	l []Item
}

func (it Item) At(i int) Item {
	if len(it.l) <= i {
		return Item{}
	}
	return it.l[i]
}

// Returns length of list, tuple, or bytes depending
// on how the item was constructed.
func (it Item) Len() int {
	if len(it.l) > 0 {
		return len(it.l)
	}
	return len(it.d)
}

func Tuple(items ...Item) Item {
	types := make([]schema.Type, len(items))
	for i, it := range items {
		types[i] = it.Type
	}
	return Item{Type: schema.Tuple(types...), l: items}
}

func Array(items ...Item) Item {
	return Item{Type: schema.Array(items[0].Type), l: items}
}

func ArrayK(items ...Item) Item {
	return Item{Type: schema.ArrayK(len(items), items[0].Type), l: items}
}

func (item Item) Equal(other Item) bool {
	switch {
	case len(item.d) == 0 && len(item.l) == 0:
		return len(other.d) == 0 && len(other.l) == 0
	case len(item.d) > 0 && len(item.l) == 0:
		return bytes.Equal(item.d, other.d)
	case len(item.d) == 0 && len(item.l) > 0:
		if len(item.l) != len(other.l) {
			return false
		}
		for i, it := range item.l {
			if !it.Equal(other.l[i]) {
				return false
			}
		}
		return true
	default:
		panic("item must have set d xor l")
	}
}

func rpad(l int, d []byte) []byte {
	n := len(d) % l
	if n == 0 {
		return d
	}
	return append(d, make([]byte, l-n)...)
}

// ABI encoding. Not packed.
func Encode(item Item) []byte {
	switch item.Kind {
	case 's':
		return item.d
	case 'd':
		var c [32]byte
		bint.Encode(c[:], uint64(len(item.d)))
		return append(c[:], rpad(32, item.d)...)
	case 'a':
		var res []byte
		if item.Length == 0 {
			res = make([]byte, 32)
			bint.Encode(res, uint64(len(item.l)))
		}
		item.Kind = 't'
		return append(res, Encode(item)...)
	case 't':
		var head, tail []byte
		for _, it := range item.l {
			if it.Static {
				head = append(head, Encode(it)...)
				continue
			}
			var offset [32]byte
			bint.Encode(offset[:], uint64(len(item.l)*32+len(tail)))
			head = append(head, offset[:]...)
			tail = append(tail, Encode(it)...)
		}
		return append(head, tail...)
	default:
		panic("abi: encode: unkown type")
	}
}

// Decodes ABI encoded bytes into an [Item] according to
// the 'schema' defined by t. For example:
//	Decode(b, abit.Tuple(abit.String, abit.Uint256))
func Decode(input []byte, t schema.Type) Item {
	switch t.Kind {
	case 's':
		return Item{d: input[:32]}
	case 'd':
		count := bint.Decode(input[:32])
		return Item{d: input[32 : 32+count]}
	case 'a':
		var count, n = t.Length, 0
		if count <= 0 { // dynamic sized list
			count, n = int(bint.Decode(input[:32])), 32
		}
		items := make([]Item, count)
		for i := 0; i < count; i++ {
			if t.Elem.Static {
				items[i] = Decode(input[n:], *t.Elem)
				n += t.Elem.Size
				continue
			}
			offset := bint.Decode(input[n : n+32])
			items[i] = Decode(input[32+offset:], *t.Elem)
			n += 32
		}
		return Item{l: items}
	case 't':
		var n int
		items := make([]Item, len(t.Fields))
		for i, f := range t.Fields {
			if f.Static {
				items[i] = Decode(input[n:], f)
				n += f.Size
				continue
			}
			offset := bint.Decode(input[n : n+32])
			items[i] = Decode(input[offset:], f)
			n += 32
		}
		return Item{l: items}
	default:
		panic("unknown type")
	}
}
