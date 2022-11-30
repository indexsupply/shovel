// Implementation of: https://docs.soliditylang.org/en/latest/abi-spec.html
package abi

import (
	"math/big"

	"github.com/indexsupply/x/bint"
)

type kind byte

const (
	static kind = iota
	dynamic
)

func rpad(d []byte) []byte {
	n := len(d) % 32
	if n == 0 {
		return d
	}
	return append(d, make([]byte, 32-n)...)
}

type Item struct {
	k kind
	t string

	// must be d XOR l
	d []byte
	l []Item
}

func Bytes(d []byte) Item {
	var b = make([]byte, 32)
	bint.Encode(b, uint64(len(b)))
	return Item{
		k: dynamic,
		t: "bytes",
		d: append(b, rpad(d)...),
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
		k: dynamic,
		t: "string",
		d: append(b, rpad([]byte(s))...),
	}
}

func (it Item) String() string {
	if len(it.d) < 64 {
		return ""
	}
	return string(it.d[32:])
}

func Bool(b bool) Item {
	var d [32]byte
	if b {
		d[31] = 1
	}
	return Item{
		k: static,
		t: "bool",
		d: d[:],
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
		k: static,
		t: "uint256",
		d: b[:],
	}
}

func (i Item) BigInt() *big.Int {
	x := &big.Int{}
	x.SetBytes(i.d)
	return x
}

func Int(i int) Item {
	var b [32]byte
	bint.Encode(b[:], uint64(i))
	return Item{
		k: static,
		t: "int",
		d: b[:],
	}
}

func List(items ...Item) Item {
	return Item{l: items}
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

func Encode(items ...Item) []byte {
	var head, tail []byte
	for i := range items {
		switch {
		case items[i].d != nil:
			switch items[i].k {
			case static:
				head = append(head, items[i].d...)
			case dynamic:
				var (
					n      = len(items)*32 + len(tail)
					offset = [32]byte{}
				)
				bint.Encode(offset[:], uint64(n))
				head = append(head, offset[:]...)
				tail = append(tail, items[i].d...)
			}
		case len(items[i].l) > 0:
			var (
				n      = len(items)*32 + len(tail)
				offset = [32]byte{}
				count  = [32]byte{}
			)
			bint.Encode(offset[:], uint64(n))
			head = append(head, offset[:]...)
			bint.Encode(count[:], uint64(len(items[i].l)))
			tail = append(tail, count[:]...)
			tail = append(tail, Encode(items[i].l...)...)
		default:
			panic("item must have data or list")
		}
	}
	return append(head, tail...)
}
