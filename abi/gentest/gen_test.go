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

func TestMatch(t *testing.T) {
	f, ok := MatchFoo(abi.Log{
		Topics: [4][32]byte{
			FooEvent.SignatureHash(),
			*(*[32]byte)(abi.Encode(abi.Uint64(42))),
		},
		Data: abi.Encode(abi.Tuple(abi.String("baz"))),
	})
	if !ok {
		t.Fatal("expected testmatch to match")
	}
	if f.Bar() != 42 {
		t.Errorf("got: %d want: %d", f.Bar(), 42)
	}
	if f.Baz() != "baz" {
		t.Errorf("got: %s want: %s", f.Baz(), "baz")
	}
}

func TestNestedArray(t *testing.T) {
	b, ok := MatchBar(abi.Log{
		Topics: [4][32]byte{
			BarEvent.SignatureHash(),
			*(*[32]byte)(abi.Encode(abi.Uint64(42))),
		},
		Data: abi.Encode(abi.Tuple(abi.List(abi.List(abi.List(abi.String("qux")))))),
	})
	if !ok {
		t.Fatal("expected nested array to match")
	}
	want := [][][]string{[][]string{[]string{"qux"}}}
	if !reflect.DeepEqual(b.Baz(), want) {
		t.Errorf("got: %#v want: %# v", b.Baz(), want)
	}
}
