package jrpc

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"kr.dev/diff"
)

func TestEthCall(t *testing.T) {
	want := make([]byte, 32)
	want[31] = 1
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := json.NewEncoder(w).Encode([]Response{
			Response{
				Version: "2.0",
				ID:      "1",
				Result:  json.RawMessage(fmt.Sprintf(`"0x%x"`, want)),
			},
		})
		diff.Test(t, t.Fatalf, nil, err)
	}))
	defer ts.Close()
	c, err := New(WithHTTP(ts.URL))
	diff.Test(t, t.Fatalf, nil, err)
	got, err := c.EthCall([20]byte{}, []byte{})
	diff.Test(t, t.Fatalf, nil, err)
	diff.Test(t, t.Errorf, want, got)
}
