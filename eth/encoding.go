package eth

import (
	"encoding/hex"
	"strconv"
)

// deals with eth's 0x prefix and possible odd length
func DecodeHex(s string) []byte {
	if len(s) >= 2 && s[0] == '0' && (s[1] == 'x' || s[1] == 'X') {
		s = s[2:]
	}
	if len(s)%2 == 1 {
		s = "0" + s
	}
	h, _ := hex.DecodeString(s)
	return h
}

// 0x prefixed hex encoded string
func EncodeHex(b []byte) string {
	return "0x" + hex.EncodeToString(b)
}

func DecodeUint64(s string) uint64 {
	if len(s) >= 2 && s[0] == '0' && (s[1] == 'x' || s[1] == 'X') {
		s = s[2:]
	}
	if len(s)%2 == 1 {
		s = "0" + s
	}
	n, err := strconv.ParseUint(s, 16, 64)
	if err != nil {
		panic(err)
	}
	return n
}

func EncodeUint64(n uint64) string {
	return "0x" + strconv.FormatUint(n, 16)
}
