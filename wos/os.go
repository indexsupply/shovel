package wos

import (
	"fmt"
	"os"
	"strings"
)

// If s has a $ prefix then we assume
// that it is a placeholder url and the actual
// url is in an env variable.
//
// # If there is no env var for s then the program will crash with an error
//
// if there is no $ prefix then s is returned
func Getenv(s string) string {
	if strings.HasPrefix(s, "$") {
		v := os.Getenv(strings.ToUpper(strings.TrimPrefix(s, "$")))
		if v == "" {
			fmt.Printf("expected database url in env: %q\n", s)
			os.Exit(1)
		}
		return v
	}
	return s
}
