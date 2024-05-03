package tc

import (
	"reflect"
	"testing"

	"github.com/kr/pretty"
)

func NoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("expected no error. got: %s", err)
	}
}

func WantErr(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error. got: %s", err)
	}
}

func WantGot(tb testing.TB, want, got any) {
	tb.Helper()
	if !reflect.DeepEqual(want, got) {
		tb.Error(pretty.Sprintf("want: %v got: %v", want, got))
	}
}
