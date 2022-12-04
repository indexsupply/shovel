// Implementation of: https://docs.soliditylang.org/en/latest/abi-spec.html
package abi

import (
	"math/big"

	"github.com/indexsupply/x/abi/at"
	"github.com/indexsupply/x/bint"
)

func rpad(d []byte) []byte {
	n := len(d) % 32
	if n == 0 {
		return d
	}
	return append(d, make([]byte, 32-n)...)
}

type Item struct {
	at.Type

	// must be d XOR l
	d []byte
	l []Item
}

func Bytes(d []byte) Item {
	var b = make([]byte, 32)
	bint.Encode(b, uint64(len(b)))
	return Item{
		Type: at.Bytes,
		d:    append(b, rpad(d)...),
	}
}

func (it Item) Bytes() []byte {
	if len(it.d) < 64 {
		return []byte{}
	}
	return it.d[32:]
}

func String(s string) Item {
	var b = make([]byte, 32)
	bint.Encode(b, uint64(len(s)))
	return Item{
		Type: at.String,
		d:    append(b, rpad([]byte(s))...),
	}
}

func (it Item) String() string {
	return string(it.d)
}

func Bool(b bool) Item {
	var d [32]byte
	if b {
		d[31] = 1
	}
	return Item{
		Type: at.Bool,
		d:    d[:],
	}
}

func (it Item) Bool() bool {
	if len(it.d) < 32 {
		return false
	}
	return it.d[31] == 1
}

func BigInt(i *big.Int) Item {
	var b [32]byte
	i.FillBytes(b[:])
	return Item{
		Type: at.Uint256,
		d:    b[:],
	}
}

func (it Item) BigInt() *big.Int {
	x := &big.Int{}
	x.SetBytes(it.d)
	return x
}

func Address(a [20]byte) Item {
	return Item{
		Type: at.Address,
		d:    rpad(a[:]),
	}
}

func (it Item) Address() [20]byte {
	if len(it.d) < 32 {
		return [20]byte{}
	}
	return *(*[20]byte)(it.d[:20])
}

func Int(i int) Item {
	var b [32]byte
	bint.Encode(b[:], uint64(i))
	return Item{
		Type: at.Int,
		d:    b[:],
	}
}

func (it Item) Int() int {
	return int(bint.Decode(it.d))
}

func List(items ...Item) Item {
	return Item{
		Type: at.List(items[0].Type),
		l:    items,
	}
}

func (it Item) At(i int) Item {
	if len(it.l) < i {
		return Item{}
	}
	return it.l[i]
}

func (it Item) Len() int {
	return len(it.l)
}

func Tuple(items ...Item) Item {
	var types []at.Type
	for i := range items {
		types = append(types, items[i].Type)
	}
	return Item{
		Type: at.Tuple(types...),
		l:    items,
	}
}

func Encode(item Item) []byte {
	var head, tail []byte
	for _, it := range item.l {
		switch it.Kind {
		case at.S:
			head = append(head, it.d...)
		case at.D:
			var offset [32]byte
			bint.Encode(offset[:], uint64(len(item.l)*32+len(tail)))
			head = append(head, offset[:]...)
			tail = append(tail, it.d...)
		case at.L:
			var offset, count [32]byte
			bint.Encode(offset[:], uint64(len(item.l)*32+len(tail)))
			head = append(head, offset[:]...)
			bint.Encode(count[:], uint64(len(it.l)))
			tail = append(tail, count[:]...)
			tail = append(tail, Encode(it)...)
		}
	}
	return append(head, tail...)
}

func Decode(input []byte, t at.Type) Item {
	switch t.Kind {
	case at.S:
		return Item{
			Type: t,
			d:    input[:32],
		}
	case at.D:
		var (
			offset = bint.Decode(input[:32])
			count  = bint.Decode(input[offset : offset+32])
		)
		return Item{
			Type: t,
			d:    input[offset+32 : offset+32+count],
		}
	case at.T:
		var (
			n     int
			items []Item
		)
		for _, f := range t.Fields {
			items = append(items, Decode(input[n:], *f))
			n += 32
		}
		return Item{
			Type: t,
			l:    items,
		}
	case at.L:
		switch t.ElementType.Kind {
		case at.L:
			offset := bint.Decode(input[:32])
			count := bint.Decode(input[offset : offset+32])
			items := []Item{}
			for j := uint64(1); j <= count; j++ {
				head := offset + (32 * j)
				tail := offset + 32 + bint.Decode(input[head:head+32])
				items = append(items, Decode(input[tail:], *t.ElementType))
			}
			return Item{
				Type: t,
				l:    items,
			}
		default:
			count := bint.Decode(input[:32])
			items := []Item{}
			for j := uint64(1); j <= count; j++ {
				items = append(items, Decode(input[32*j:], *t.ElementType))
			}
			return Item{
				Type: t,
				l:    items,
			}
		}
	default:
		panic("unhandled type")
	}
}
