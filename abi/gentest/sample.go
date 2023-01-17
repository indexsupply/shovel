package gentest

import "github.com/indexsupply/x/abi"

var E1Event = abi.Event{
	Name: "e1",
	Inputs: []abi.Input{
		abi.Input{
			Name: "i1",
			Type: "address",
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

type E1 struct {
	it *abi.Item
}

type I2 struct {
	it *abi.Item
}

type F3 struct {
	it *abi.Item
}

func (x *E1) I1() [20]byte {
	return x.it.At(0).Address()
}

func (x *E1) I2() *I2 {
	i := x.it.At(1)
	return &I2{&i}
}

func (x *I2) F1() [20]byte {
	return x.it.At(0).Address()
}

func (x *I2) F2() [][20]byte {
	it := x.it.At(1)
	addrs := make([][20]byte, it.Len())
	for i, a := range it.List() {
		addrs[i] = a.Address()
	}
	return addrs
}

func (x *I2) F3() *F3 {
	i := x.it.At(2)
	return &F3{&i}
}

func (x *F3) F4() [20]byte {
	return x.it.At(0).Address()
}
