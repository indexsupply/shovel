package isxerrors

import "golang.org/x/xerrors"

// Wraps xerrors.Errorf but returns nil if err is nil
func Errorf(format string, args ...interface{}) error {
	for i := range args {
		if _, ok := args[i].(error); ok {
			return xerrors.Errorf(format, args...)
		}
	}
	return nil
}
