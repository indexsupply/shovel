package gentest

import (
	"math/big"

	"github.com/indexsupply/x/abi"
)

var EEvent = abi.Event{
	Name: "e",
	Inputs: []abi.Input{
		abi.Input{
			Name: "address",
			Type: "address",
		},
		abi.Input{
			Name: "address_list",
			Type: "address[]",
		},
		abi.Input{
			Name: "bool",
			Type: "bool",
		},
		abi.Input{
			Name: "bool_list",
			Type: "bool[]",
		},
		abi.Input{
			Name: "uint256",
			Type: "uint256",
		},
		abi.Input{
			Name: "uint256_list",
			Type: "uint256[]",
		},
		abi.Input{
			Name: "i2",
			Type: "tuple",
			Inputs: []abi.Input{
				abi.Input{
					Name: "f1",
					Type: "address",
				},
				abi.Input{
					Name: "f2",
					Type: "address[]",
				},
				abi.Input{
					Name: "f3",
					Type: "tuple",
					Inputs: []abi.Input{
						abi.Input{
							Name: "f4",
							Type: "address",
						},
					},
				},
			},
		},
	},
}

type E struct {
	it *abi.Item
}

type I2 struct {
	it *abi.Item
}

type F3 struct {
	it *abi.Item
}

func (x *E) Address() [20]byte {
	return x.it.At(0).Address()
}

func (x *E) AddressList() [][20]byte {
	it := x.it.At(1)
	res := make([][20]byte, it.Len())
	for i, v := range it.List() {
		res[i] = v.Address()
	}
	return res
}

func (x *E) Bool() bool {
	return x.it.At(2).Bool()
}

func (x *E) BoolList() []bool {
	it := x.it.At(3)
	res := make([]bool, it.Len())
	for i, v := range it.List() {
		res[i] = v.Bool()
	}
	return res
}

func (x *E) Uint256() *big.Int {
	return x.it.At(4).BigInt()
}

func (x *E) Uint256List() []*big.Int {
	it := x.it.At(5)
	res := make([]*big.Int, it.Len())
	for i, v := range it.List() {
		res[i] = v.BigInt()
	}
	return res
}

func (x *E) I2() *I2 {
	i := x.it.At(6)
	return &I2{&i}
}

func (x *I2) F1() [20]byte {
	return x.it.At(0).Address()
}

func (x *I2) F2() [][20]byte {
	it := x.it.At(1)
	res := make([][20]byte, it.Len())
	for i, v := range it.List() {
		res[i] = v.Address()
	}
	return res
}

func (x *I2) F3() *F3 {
	i := x.it.At(2)
	return &F3{&i}
}

func (x *F3) F4() [20]byte {
	return x.it.At(0).Address()
}
