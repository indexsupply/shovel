package tc

import "testing"

func NoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Errorf("expected no error. got: %s", err)
	}
}
