package gentest

import (
	"testing"

	"github.com/indexsupply/x/abi"
)

//go:generate genabi -i sample.json -o sample.go -p gentest

func TestZero(t *testing.T) {
	e1 := E1{&abi.Item{}}
	e1.I1()
	i2 := e1.I2()
	i2.F1()
	f3 := i2.F3()
	f3.F4()
}
