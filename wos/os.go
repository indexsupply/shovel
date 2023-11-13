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
			fmt.Printf("expected %s to be set\n", s)
			os.Exit(1)
		}
		return v
	}
	return s
}

type EnvString string

func (es *EnvString) UnmarshalJSON(data []byte) error {
	if len(data) < 2 {
		return fmt.Errorf("EnvString must be at leaset 2 bytes")
	}
	data = data[1 : len(data)-1] // remove quotes
	*es = EnvString(Getenv(string(data)))
	return nil
}
