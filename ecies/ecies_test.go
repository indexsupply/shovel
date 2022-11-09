package ecies

import (
	"bytes"
	"testing"

	"github.com/indexsupply/x/tc"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
)

func TestEncode(t *testing.T) {
	k, err := secp256k1.GeneratePrivateKey()
	tc.NoErr(t, err)
	msg := []byte("hello world")
	ct, err := Encrypt(k.PubKey(), msg)
	tc.NoErr(t, err)
	pt, err := Decrypt(k, ct)
	tc.NoErr(t, err)
	if !bytes.Equal(pt, msg) {
		t.Errorf("expected: %s got: %s", msg, pt)
	}
}
