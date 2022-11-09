package ecies

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
)

// Overhead is the length in bytes of the ECIES message *excluding* the ciphertext length.
// It is 65 (uncompressed public key) + 16 (IV) + 32 (MAC).
const Overhead = 113

// Implements ECIES encrypt:
// The encrypted message is of the form R || iv || AES-encrypt(ke, msg) || HMAC(km, c | macShared)
// Where ke and km are derived using NIST Special Publication 800-56A Concatenation
// on the derived shared secret. The shared secret should be generated using a random point on
// the secp256k1 curve and the receiver's public key.
// Optional params kdfShared and macShared are as inputs into the KDF and HMAC operations, respectively.
func Encrypt(pubkey *secp256k1.PublicKey, msg, kdfShared, macShared []byte) ([]byte, error) {
	var ct bytes.Buffer
	ek, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		return nil, err
	}
	ct.Write(ek.PubKey().SerializeUncompressed())

	sharedSecret := secp256k1.GenerateSharedSecret(ek, pubkey)
	// Use NIST Special Publication 800-56A Concatenation KDF
	ke, km := deriveKeys(sharedSecret[:], kdfShared)

	// AES Encrypt
	c, err := aes.NewCipher(ke)
	if err != nil {
		return nil, err
	}

	iv := make([]byte, aes.BlockSize)
	rand.Read(iv)
	ct.Write(iv)

	ctr := cipher.NewCTR(c, iv)
	ciphertext := make([]byte, len(msg))
	ctr.XORKeyStream(ciphertext[:], msg)
	ct.Write(ciphertext)

	// Assemble MAC tag

	mac := hmac.New(sha256.New, km)
	mac.Write(ciphertext)
	mac.Write(macShared)
	tag := mac.Sum(nil)
	ct.Write(tag)

	return ct.Bytes(), nil
}

// deriveKeys returns the encryption and mac keys from z (shared key)
// and optional shared information s.
func deriveKeys(z, s []byte) (ke, km []byte) {
	kdLen := 2 * 16
	counterBytes := make([]byte, 4)
	hash := sha256.New()
	k := make([]byte, 0, (kdLen + hash.Size() - (kdLen % hash.Size())))
	for counter := uint32(1); len(k) < kdLen; counter++ {
		binary.BigEndian.PutUint32(counterBytes, counter)
		hash.Reset()
		hash.Write(counterBytes)
		hash.Write(z)
		hash.Write(nil)
		k = hash.Sum(k)
	}
	ke = k[:16]
	km = k[16:]
	hash.Reset()
	hash.Write(km)
	km = hash.Sum(km[:0])
	return
}
