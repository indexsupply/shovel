package wos

import (
	"encoding/json"
	"os"
	"testing"

	"kr.dev/diff"
)

func TestUnmarshal(t *testing.T) {
	diff.Test(t, t.Fatalf, nil, os.Setenv("XXX", "42"))
	cases := []struct {
		input []byte
		want  int
		err   string
	}{
		{
			[]byte(``),
			0,
			"unexpected end of JSON input",
		},
		{
			[]byte(`10`),
			10,
			"",
		},
		{
			[]byte(`"10"`),
			10,
			"",
		},
		{
			[]byte(`"$XXX"`),
			42,
			"",
		},
	}
	for _, tc := range cases {
		var (
			got    EnvInt
			gotErr string
		)
		err := json.Unmarshal(tc.input, &got)
		if err != nil {
			gotErr = err.Error()
		}
		diff.Test(t, t.Errorf, tc.err, gotErr)
		diff.Test(t, t.Errorf, tc.want, int(got))
	}
}
