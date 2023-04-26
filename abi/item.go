package abi

import (
	"math/big"

	"github.com/holiman/uint256"
	"github.com/indexsupply/x/abi/schema"
	"github.com/indexsupply/x/bint"
)

func Address(a [20]byte) *Item {
	var i uint256.Int
	i.SetBytes20(a[:])
	b := i.Bytes32()
	return &Item{Type: schema.Static(), d: b[:]}
}

func (item *Item) Address() [20]byte {
	if len(item.d) < 20 {
		return [20]byte{}
	}
	return [20]byte(item.d[12:])
}

func Uint256(i uint256.Int) *Item {
	b := i.Bytes32()
	return &Item{Type: schema.Static(), d: b[:]}
}

func (item *Item) Uint256() uint256.Int {
	var i uint256.Int
	i.SetBytes(item.d)
	return i
}

func BigInt(i *big.Int) *Item {
	var b [32]byte
	i.FillBytes(b[:])
	return &Item{Type: schema.Static(), d: b[:]}
}

func (item *Item) BigInt() *big.Int {
	i := item.Uint256()
	return i.ToBig()
}

func Bool(b bool) *Item {
	var d [32]byte
	if b {
		d[31] = 1
	}
	return &Item{Type: schema.Static(), d: d[:]}
}

func (item *Item) Bool() bool {
	if len(item.d) < 32 {
		return false
	}
	return item.d[31] == 1
}

func Bytes(d []byte) *Item {
	return &Item{Type: schema.Dynamic(), d: d}
}

func (it *Item) Bytes() []byte {
	return it.d
}

func Bytes32(d [32]byte) *Item {
	return &Item{Type: schema.Static(), d: d[:]}
}

func (item *Item) Bytes32() [32]byte {
	if len(item.d) < 32 {
		return [32]byte{}
	}
	return [32]byte(item.d[:32])
}

func Bytes4(d [4]byte) *Item {
	return &Item{Type: schema.Static(), d: d[:]}
}

func (item *Item) Bytes4() [4]byte {
	if len(item.d) < 4 {
		return [4]byte{}
	}
	return [4]byte(item.d[:4])
}

func String(s string) *Item {
	return &Item{Type: schema.Dynamic(), d: []byte(s)}
}

func (item *Item) String() string {
	return string(item.d)
}

func Uint8(i uint8) *Item {
	var b [32]byte
	bint.Encode(b[:], uint64(i))
	return &Item{Type: schema.Static(), d: b[:]}
}

func (item *Item) Uint8() uint8 {
	if len(item.d) < 1 {
		return 0
	}
	return uint8(bint.Decode(item.d))
}

func Uint16(i uint16) *Item {
	var b [32]byte
	bint.Encode(b[:], uint64(i))
	return &Item{Type: schema.Static(), d: b[:]}
}

func (item *Item) Uint16() uint16 {
	if len(item.d) < 2 {
		return 0
	}
	return uint16(bint.Decode(item.d))
}

func Uint32(i uint32) *Item {
	var b [32]byte
	bint.Encode(b[:], uint64(i))
	return &Item{Type: schema.Static(), d: b[:]}
}

func (item *Item) Uint32() uint32 {
	if len(item.d) < 4 {
		return 0
	}
	return uint32(bint.Decode(item.d[:4]))
}

func Uint64(i uint64) *Item {
	var b [32]byte
	bint.Encode(b[:], i)
	return &Item{Type: schema.Static(), d: b[:]}
}

func (item *Item) Uint64() uint64 {
	if len(item.d) < 8 {
		return 0
	}
	return bint.Decode(item.d[:8])
}
