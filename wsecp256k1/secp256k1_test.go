package wsecp256k1

import (
	"testing"

	"kr.dev/diff"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/indexsupply/x/tc"
)

func TestSignRecover(t *testing.T) {
	prv, err := secp256k1.GeneratePrivateKey()
	diff.Test(t, t.Fatalf, nil, err)

	h := make([]byte, 32)
	sig, err := Sign(prv, h)
	diff.Test(t, t.Fatalf, nil, err)

	pub, err := Recover(sig, h)
	diff.Test(t, t.Fatalf, nil, err)
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
