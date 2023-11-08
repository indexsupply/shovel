package wstrings

import (
	"errors"
	"unicode"
)

func Safe(s string) error {
	for _, r := range s {
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-') {
			return errors.New("must be 'a-z', 'A-Z', '0-9', '_', or '-'")
		}
	}
	return nil
}
