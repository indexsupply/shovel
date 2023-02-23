package example

import (
	"reflect"
	"testing"

	"github.com/indexsupply/x/abi"
)

//go:generate genabi -i example.json -o example.go -p example

func TestGenZero(t *testing.T) {
	x := Transfer{}
	if x.From != [20]byte{} {
		t.Errorf("want: %v got: %v", [20]byte{}, x.From)
	}
	if x.To != [20]byte{} {
		t.Errorf("want: %v got: %v", [20]byte{}, x.To)
	}
	if x.Id != nil {
		t.Errorf("want: nil got: %v", x.Id.String())
	}
	if !reflect.DeepEqual(x.Details, [][]Details(nil)) {
		t.Errorf("want: %#v got: %#v", [][]Details(nil), x.Details)
	}
}

func BenchmarkMatch(b *testing.B) {
	topics := [][32]byte{
		transferSignature,
		[32]byte{},
		[32]byte{},
		[32]byte{},
	}
	data := abi.Encode(abi.Tuple(
		abi.ArrayK(
			abi.ArrayK(abi.Uint8(1), abi.Uint8(2)),
			abi.ArrayK(abi.Uint8(2), abi.Uint8(4)),
			abi.ArrayK(abi.Uint8(3), abi.Uint8(6)),
		),
		abi.Array(abi.Array(
			abi.Tuple(
				abi.Address([20]byte{}),
				abi.Bytes32([32]byte{}),
				abi.Bytes([]byte{}),
				abi.Tuple(
					abi.Uint8(0),
					abi.Uint8(1),
				),
			),
			abi.Tuple(
				abi.Address([20]byte{}),
				abi.Bytes32([32]byte{}),
				abi.Bytes([]byte{}),
				abi.Tuple(
					abi.Uint8(0),
					abi.Uint8(1),
				),
			),
		)),
	))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		MatchTransfer(topics, data)
	}
}
