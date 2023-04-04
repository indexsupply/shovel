package jrpc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEthCall(t *testing.T) {
	want := make([]byte, 32)
	want[31] = 1
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(Response{
			Version: "2.0",
			ID:      "1",
			Result:  fmt.Sprintf("0x%x", want),
		})
	}))
	defer ts.Close()
	c := New(WithURL(ts.URL))
	got, err := c.EthCall([20]byte{}, []byte{})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(want, got) {
		t.Errorf("want: %x got: %x", want, got)
	}
}
