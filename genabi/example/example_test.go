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
	b.ReportAllocs()
	l := abi.Log{
		Topics: [4][32]byte{
			TransferSignature,
			[32]byte{},
			[32]byte{},
			[32]byte{},
		},
		Data: abi.Encode(abi.Tuple(abi.Array(abi.Array(
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
		)))),
	}
	for i := 0; i < b.N; i++ {
		MatchTransfer(l)
	}
}
