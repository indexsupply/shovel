package eth

import (
	"testing"

	"kr.dev/diff"
)

func TestDecodeHex(t *testing.T) {
	cases := []struct {
		input string
		want  []byte
	}{
		{
			input: "0x6a6a",
			want:  []byte{0x6a, 0x6a},
		},
		{
			input: "0X6a6a",
			want:  []byte{0x6a, 0x6a},
		},
		{
			input: "6a6a",
			want:  []byte{0x6a, 0x6a},
		},
		{
			input: "6",
			want:  []byte{0x06},
		},
	}
	for _, tc := range cases {
		diff.Test(t, t.Errorf, DecodeHex(tc.input), tc.want)
	}
}
