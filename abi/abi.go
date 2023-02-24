// ABI encoding/decoding
//
// Implementation based on the [ABI Spec].
//
// [ABI Spec]: https://docs.soliditylang.org/en/latest/abi-spec.html
package abi

import (
	"bytes"
	"errors"
	"sync"

	"github.com/indexsupply/x/abi/schema"
	"github.com/indexsupply/x/bint"
)

var (
	SigMismatch   = errors.New("event signature doesn't match topics[0]")
	IndexMismatch = errors.New("num indexed inputs doesn't match len(topics)")
)

type Log struct {
	Address [20]byte
	Topics  [][32]byte
	Data    []byte
}

type Item struct {
	schema.Type
	d []byte
	l []*Item
}

func (it *Item) At(i int) *Item {
	if len(it.l) <= i {
		return &Item{}
	}
	return it.l[i]
}

// Returns length of list, tuple, or bytes depending
// on how the item was constructed.
func (it *Item) Len() int {
	if len(it.l) > 0 {
		return len(it.l)
	}
	return len(it.d)
}

func Tuple(items ...*Item) *Item {
	types := make([]schema.Type, len(items))
	for i, it := range items {
		types[i] = it.Type
	}
	return &Item{Type: schema.Tuple(types...), l: items}
}

func Array(items ...*Item) *Item {
	return &Item{Type: schema.Array(items[0].Type), l: items}
}

func ArrayK(items ...*Item) *Item {
	return &Item{Type: schema.ArrayK(len(items), items[0].Type), l: items}
}

func (item *Item) Equal(other *Item) bool {
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
func Encode(item *Item) []byte {
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
		var hlen int
		for _, it := range item.l {
			switch {
			case it.Static:
				hlen += it.Size
			default:
				hlen += 32
			}
		}
		var head, tail []byte
		for _, it := range item.l {
			if it.Static {
				head = append(head, Encode(it)...)
				continue
			}
			var offset [32]byte
			bint.Encode(offset[:], uint64(hlen+len(tail)))
			head = append(head, offset[:]...)
			tail = append(tail, Encode(it)...)
		}
		return append(head, tail...)
	default:
		panic("abi: encode: unkown type")
	}
}

var itemPool = sync.Pool{New: func() any { return &Item{} }}

func (item *Item) reset() {
	item.d = item.d[:0]
	item.l = item.l[:0]
}

func (item *Item) Done() {
	if item == nil {
		return
	}
	for _, i := range item.l {
		i.Done()
	}
	itemPool.Put(item)
}

// Decodes ABI encoded bytes into an [Item] according to
// the 'schema' defined by t. For example:
//	Decode(b, schema.Tuple(schema.Dynamic(), schema.Static()))
// Returns the item and the number of bytes read from input
func Decode(input []byte, t schema.Type) (*Item, int, error) {
	item := itemPool.Get().(*Item)
	item.reset()
	switch t.Kind {
	case 's':
		if len(input) < 32 {
			return item, 0, errors.New("EOF")
		}
		item.d = input[:32]
		return item, 32, nil
	case 'd':
		length := int(bint.Decode(input[:32]))
		nbytes := length + (32 - (length % 32))
		if len(input) < 32+length {
			return item, 0, errors.New("EOF")
		}
		item.d = input[32 : 32+length]
		return item, 32 + nbytes, nil
	case 'a':
		var length, start, nbytes, pos = t.Length, 0, 0, 0
		if length <= 0 { // dynamic sized array
			if len(input) < 32 {
				return item, 0, errors.New("EOF")
			}
			length, start, nbytes, pos = int(bint.Decode(input[:32])), 32, 32, 32
		}
		for i := 0; i < length; i++ {
			switch {
			case t.Elem.Static:
				if len(input) < pos {
					return item, nbytes, errors.New("EOF")
				}
				it, n, err := Decode(input[pos:], *t.Elem)
				if err != nil {
					return item, nbytes, err
				}
				item.l = append(item.l, it)
				pos += t.Elem.Size
				nbytes += n
			default:
				if len(input) < pos+32 {
					return item, nbytes, errors.New("EOF")
				}
				offset := int(bint.Decode(input[pos : pos+32]))
				if len(input) < start+offset {
					return item, nbytes, errors.New("EOF")
				}
				it, n, err := Decode(input[start+offset:], *t.Elem)
				if err != nil {
					return item, nbytes, errors.New("EOF")
				}
				item.l = append(item.l, it)
				pos += 32
				nbytes += 32 + n
			}
		}
		return item, nbytes, nil
	case 't':
		var pos, nbytes int
		for _, f := range t.Fields {
			switch {
			case f.Static:
				if len(input) < pos {
					return item, nbytes, errors.New("EOF")
				}
				it, n, err := Decode(input[pos:], f)
				if err != nil {
					return item, nbytes, errors.New("EOF")
				}
				item.l = append(item.l, it)
				pos += f.Size
				nbytes += n
			default:
				if len(input) < pos+32 {
					return item, nbytes, errors.New("EOF")
				}
				offset := int(bint.Decode(input[pos : pos+32]))
				if len(input) < offset {
					return item, nbytes, errors.New("EOF")
				}
				it, n, err := Decode(input[offset:], f)
				if err != nil {
					return item, nbytes, errors.New("EOF")
				}
				item.l = append(item.l, it)
				pos += 32
				nbytes += 32 + n
			}
		}
		return item, nbytes, nil
	default:
		panic("unknown type")
	}
}
