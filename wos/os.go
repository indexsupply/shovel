package wos

import (
	"fmt"
	"os"
	"strconv"
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
		return fmt.Errorf("EnvString must be at least 2 bytes")
	}
	data = data[1 : len(data)-1] // remove quotes
	*es = EnvString(Getenv(string(data)))
	return nil
}

type EnvUint64 uint64

func (eu *EnvUint64) UnmarshalJSON(d []byte) error {
	if len(d) >= 2 && d[0] == '"' && d[len(d)-1] == '"' {
		d = d[1 : len(d)-1] // remove quotes
	}
	s := Getenv(string(d))
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return fmt.Errorf("EnvUint64 unable to decode %s: %w", s, err)
	}
	*eu = EnvUint64(n)
	return nil
}

type EnvInt int

func (ei *EnvInt) UnmarshalJSON(d []byte) error {
	if len(d) >= 2 && d[0] == '"' && d[len(d)-1] == '"' {
		d = d[1 : len(d)-1] // remove quotes
	}
	s := Getenv(string(d))
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return fmt.Errorf("EnvInt unable to decode %s: %w", s, err)
	}
	*ei = EnvInt(n)
	return nil
}
