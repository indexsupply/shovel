package example

import (
	"reflect"
	"testing"
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
