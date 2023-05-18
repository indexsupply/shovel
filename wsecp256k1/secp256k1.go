// wsecp256k1 provides a wrapper around the secp256k1 package
// from dcrec --which is an actively maintained codebase built from
// btcd.
//
// Since Ethereum uses secp256k1 in special ways, this package
// exists to encapsulate the special ways so that it is easier to
// use the secp256k1 code in the 'right way.'
//
// Sign and Recover are constructed from the secp256k1 author's
// advice: https://github.com/decred/dcrd/issues/2889,
// https://go.dev/play/p/gIbvbly7n9h
package wsecp256k1

import (
	"errors"

	"github.com/indexsupply/x/isxerrors"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
)

func Sign(k *secp256k1.PrivateKey, d []byte) ([]byte, error) {
	if len(d) != 32 {
		return nil, errors.New("input must be 32 bytes long")
	}
	sig := ecdsa.SignCompact(k, d, false)
	v := sig[0] - 27
	copy(sig, sig[1:])
	sig[64] = v
	if len(sig) != 65 {
		return nil, errors.New("signature must be 65 bytes long")
	}
	return sig, nil
}

func Recover(sig []byte, hash []byte) (*secp256k1.PublicKey, error) {
	if len(sig) != 65 {
		return nil, errors.New("sig must be 65 bytes long")
	}
	if len(hash) != 32 {
		return nil, errors.New("hash must be 32 bytes long")
	}
	if sig[64] >= 4 {
		return nil, errors.New("invalid signature recovery id")
	}
	// Convert from Ethereum signature format with 'recovery id' v at the end.
	var converted [65]byte
	copy(converted[1:], sig)
	converted[0] = sig[64] + 27

	pub, _, err := ecdsa.RecoverCompact(converted[:], hash)
	if err != nil {
		return nil, isxerrors.Errorf("recovering pubkey: %w", err)
	}
	return pub, nil
}

func Encode(pubkey *secp256k1.PublicKey) []byte {
	// SerializeUncompressed returns:
	// 0x04 || 32-byte x coordinate || 32-byte y coordinate
	b := pubkey.SerializeUncompressed()
	return b[1:]
}

func Decode(d []byte) (*secp256k1.PublicKey, error) {
	return secp256k1.ParsePubKey(append([]byte{0x04}, d...))
}

func DecodeCompressed(d []byte) (*secp256k1.PublicKey, error) {
	return secp256k1.ParsePubKey(d)
}
