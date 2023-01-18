package gentest

import (
	"math/big"
	"reflect"
	"testing"

	"github.com/indexsupply/x/abi"
)

//go:generate genabi -i sample.json -o sample.go -p gentest

func TestZero(t *testing.T) {
	e := E{&abi.Item{}}

	if e.Address() != [20]byte{} {
		t.Error("expected empty address")
	}
	if !reflect.DeepEqual(e.AddressList(), [][20]byte{}) {
		t.Error("expected empty slice of addresses")
	}
	if e.Bool() != false {
		t.Error("expected default value bool")
	}
	if !reflect.DeepEqual(e.BoolList(), []bool{}) {
		t.Error("expected empty slice of bools")
	}
	if !reflect.DeepEqual(e.Bytes(), []byte{}) {
		t.Error("expected empty bytes")
	}
	if !reflect.DeepEqual(e.BytesList(), [][]byte{}) {
		t.Error("expected empty slice of bytes")
	}
	if e.String() != "" {
		t.Error("expected zero string")
	}
	if !reflect.DeepEqual(e.StringList(), []string{}) {
		t.Error("expected empty slice of strings")
	}
	if e.Uint8() != 0 {
		t.Error("expected zero uint8")
	}
	if !reflect.DeepEqual(e.Uint8List(), []byte{}) {
		t.Error("expected empty slice of uint8s")
	}
	if e.Uint64() != uint64(0) {
		t.Error("expected zero uint64")
	}
	if !reflect.DeepEqual(e.Uint64List(), []uint64{}) {
		t.Error("expected empty slice of uint64s")
	}
	if e.Uint256().String() != (&big.Int{}).String() {
		t.Error("expected empty big int")
	}
	if !reflect.DeepEqual(e.Uint256List(), []*big.Int{}) {
		t.Error("expected empty slice of big ints")
	}
}
