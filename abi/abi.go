// Implementation of: https://docs.soliditylang.org/en/latest/abi-spec.html
package abi

import (
	"math/big"

	"github.com/indexsupply/x/abi/at"
	"github.com/indexsupply/x/bint"
)

type Item struct {
	at.Type

	// must be d XOR l
	d []byte
	l []Item
}

func Bytes(d []byte) Item {
	return Item{Type: at.Bytes, d: d}
}

func (it Item) Bytes() []byte {
	return it.d
}

func String(s string) Item {
	return Item{Type: at.String, d: []byte(s)}
}

func (it Item) String() string {
	return string(it.d)
}

func Bool(b bool) Item {
	var d [32]byte
	if b {
		d[31] = 1
	}
	return Item{Type: at.Bool, d: d[:]}
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
		d:    rpad(32, a[:]),
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

func rpad(l int, d []byte) []byte {
	n := len(d) % l
	if n == 0 {
		return d
	}
	return append(d, make([]byte, l-n)...)
}

func Encode(it Item) []byte {
	switch it.Kind {
	case at.S:
		return it.d
	case at.D:
		var c [32]byte
		bint.Encode(c[:], uint64(len(it.d)))
		return append(c[:], rpad(32, it.d)...)
	case at.L:
		var c [32]byte
		bint.Encode(c[:], uint64(len(it.l)))
		return append(c[:], Encode(Tuple(it.l...))...)
	case at.T:
		var head, tail []byte
		for i := range it.l {
			switch it.l[i].Kind {
			case at.S:
				head = append(head, Encode(it.l[i])...)
			default:
				var offset [32]byte
				bint.Encode(offset[:], uint64(len(it.l)*32+len(tail)))
				head = append(head, offset[:]...)
				tail = append(tail, Encode(it.l[i])...)
			}
		}
		return append(head, tail...)
	default:
		panic("abi: encode: unkown type")
	}
}

func Decode(input []byte, t at.Type) Item {
	switch t.Kind {
	case at.S:
		return Item{Type: t, d: input[:32]}
	case at.D:
		count := bint.Decode(input[:32])
		return Item{Type: t, d: input[32 : 32+count]}
	case at.L:
		count := bint.Decode(input[:32])
		items := make([]Item, count)
		for i := uint64(0); i < count; i++ {
			n := 32 + (32 * i) //skip count (head)
			switch t.ElementType.Kind {
			case at.S:
				items[i] = Decode(input[n:], *t.ElementType)
			default:
				tail := 32 + bint.Decode(input[n:n+32])
				items[i] = Decode(input[tail:], *t.ElementType)
			}
		}
		return List(items...)
	case at.T:
		items := make([]Item, len(t.Fields))
		for i, f := range t.Fields {
			n := 32 * i
			switch f.Kind {
			case at.S:
				items[i] = Decode(input[n:n+32], *f)
			default:
				offset := bint.Decode(input[n : n+32])
				items[i] = Decode(input[offset:], *f)
			}
		}
		return Tuple(items...)
	default:
		panic("abi: encode: unkown type")
	}
}
