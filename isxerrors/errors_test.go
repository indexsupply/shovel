package isxerrors

import (
	"errors"
	"testing"
)

func TestErrorf(t *testing.T) {
	err := Errorf("no error: %w", nil)
	if err != nil {
		t.Errorf("expected no error to be returned. got: %s", err)
	}
	err = Errorf("no error: %w", errors.New("isxerrors"))
	if err == nil {
		t.Errorf("expected error. got none")
	}
}
