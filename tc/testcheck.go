package tc

import (
	"reflect"
	"testing"

	"github.com/kr/pretty"
)

func NoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Errorf("expected no error. got: %s", err)
	}
}

func WantGot(tb testing.TB, want, got any) {
	if !reflect.DeepEqual(want, got) {
		tb.Error(pretty.Sprintf("want: %v got: %v", want, got))
	}
}
