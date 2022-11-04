package isxsecp256k1

import (
	"testing"

	"github.com/indexsupply/x/tc"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
)

func TestSignRecover(t *testing.T) {
	prv, err := secp256k1.GeneratePrivateKey()
	tc.NoErr(t, err)
	h := [32]byte{}
	sig, err := Sign(prv, h)
	tc.NoErr(t, err)
	pub, err := Recover(sig, h)
	tc.NoErr(t, err)
	if !pub.IsEqual(prv.PubKey()) {
		t.Error("expected pub key to match")
	}
}

func TestEncodeDecode(t *testing.T) {
	prv, err := secp256k1.GeneratePrivateKey()
	tc.NoErr(t, err)
	got, err := Decode(Encode(prv.PubKey()))
	tc.NoErr(t, err)
	if !prv.PubKey().IsEqual(got) {
		t.Errorf("want: %x got: %x",
			prv.PubKey().SerializeCompressed(),
			got.SerializeCompressed(),
		)
	}
}
