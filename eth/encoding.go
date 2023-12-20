package eth

import "encoding/hex"

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
