package ecies

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/indexsupply/x/tc"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
)

func TestEncode(t *testing.T) {
	k, err := secp256k1.GeneratePrivateKey()
	tc.NoErr(t, err)
	msg := []byte("hello world")
	shared := []byte("shared info")
	ct, err := Encrypt(k.PubKey(), msg, shared)
	tc.NoErr(t, err)
	pt, err := Decrypt(k, ct, shared)
	tc.NoErr(t, err)
	if !bytes.Equal(pt, msg) {
		t.Errorf("expected: %s got: %s", msg, pt)
	}
}

func TestGethVector(t *testing.T) {
	b, err := hex.DecodeString("3d942b0bff62634bc9e51fb67873fcb60a6627708c5ca8c88873979a6891007b")
	tc.NoErr(t, err)
	k := secp256k1.PrivKeyFromBytes(b)
	ct, err := hex.DecodeString("049853fba031ea58f41780b5181bb6684c7dbe8cbb7a7ea5aa4b15b40c9960a510ce9eef396f39c991bbee623f5c40be3143e4f13627b37b4af834696a9bcd910f9f0f4777c66308303c9e6e83642ec74f55b0a5980bc609909eeaf83c47e3a2790d55a468fb47dfb1c09f858f2e523cced6c7cee8b0df125c2604e14669")
	tc.NoErr(t, err)
	pt, err := Decrypt(k, ct, nil)
	tc.NoErr(t, err)
	if string(pt) != "Hello, world." {
		t.Errorf("epected: Hello, world. got: %q", string(pt))
	}
}
