// isxsecp256k1 provides a wrapper around the secp256k1 package
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
package isxsecp256k1

import (
	"errors"

	"github.com/indexsupply/x/isxerrors"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
)

func Sign(k *secp256k1.PrivateKey, d [32]byte) ([65]byte, error) {
	sig := ecdsa.SignCompact(k, d[:], false)
	v := sig[0] - 27
	copy(sig, sig[1:])
	sig[64] = v
	if len(sig) != 65 {
		return [65]byte{}, errors.New("signature must be 65 bytes long")
	}
	return *(*[65]byte)(sig), nil
}

func Recover(sig [65]byte, hash [32]byte) (*secp256k1.PublicKey, error) {
	if sig[64] >= 4 {
		return nil, errors.New("invalid signature recovery id")
	}
	// Convert from Ethereum signature format with 'recovery id' v at the end.
	var converted [65]byte
	copy(converted[1:], sig[:])
	converted[0] = sig[64] + 27

	pub, _, err := ecdsa.RecoverCompact(converted[:], hash[:])
	if err != nil {
		return nil, isxerrors.Errorf("recovering pubkey: %w", err)
	}
	return pub, nil
}

func Encode(pubkey *secp256k1.PublicKey) [64]byte {
	// SerializeUncompressed returns:
	// 0x04 || 32-byte x coordinate || 32-byte y coordinate
	var b [64]byte
	ucpk := pubkey.SerializeUncompressed()
	copy(b[:], ucpk[1:])
	return b
}

func Decode(d [64]byte) (*secp256k1.PublicKey, error) {
	b := append([]byte{0x04}, d[:]...)
	return secp256k1.ParsePubKey(b)
}

func DecodeCompressed(d [33]byte) (*secp256k1.PublicKey, error) {
	return secp256k1.ParsePubKey(d[:])
}
