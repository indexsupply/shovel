// Provides ECIES implementation using:
// sha256 hashing , secp256k1 curves, and aes 128 ctr (16 byte key) encryption.
//
// Code in this package was built from the following 2 docs:
// 1. https://csrc.nist.gov/CSRC/media/Publications/sp/800-56a/archive/2006-05-03/documents/sp800-56-draft-jul2005.pdf
// 2. https://github.com/ethereum/devp2p/blob/master/rlpx.md#ecies-encryption
package ecies

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"errors"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
)

// Overhead is the length in bytes of the ECIES message
// *excluding* the ciphertext length.
// It is 65 (uncompressed public key) + 16 (IV) + 32 (MAC).
const Overhead = 113

// Encrypts msg to destPubKey using the following construction:
//
// r = random number (private key)
// R = r * G (public key)
// S = Px where (Px, Py) = r * KB
// kE || kM = KDF(S, 32)
// iv = random initialization vector
// c = AES(kE, iv , m)
// d = MAC(sha256(kM), iv || c)
// msg = R || iv || c || d
//
// For more details, see the ECIES Encryption docs defined by Eth's DevP2P:
// https://github.com/ethereum/devp2p/blob/master/rlpx.md#ecies-encryption
func Encrypt(destPubKey *secp256k1.PublicKey, msg []byte) ([]byte, error) {
	r, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		return nil, err
	}

	var (
		s  = secp256k1.GenerateSharedSecret(r, destPubKey)
		k  = kdf(s)
		ke = k[:16]
		km = sha256.Sum256(k[16:])
	)

	iv := make([]byte, aes.BlockSize)
	rand.Read(iv)

	block, err := aes.NewCipher(ke)
	if err != nil {
		return nil, err
	}
	c := make([]byte, len(msg))
	cipher.NewCTR(block, iv).XORKeyStream(c, msg)

	mac := hmac.New(sha256.New, km[:])
	mac.Write(iv)
	mac.Write(c)
	d := mac.Sum(nil)

	var res []byte
	res = append(res, r.PubKey().SerializeUncompressed()...)
	res = append(res, iv...)
	res = append(res, c...)
	res = append(res, d...)
	return res, nil
}

// Decrypts ciphertext using prvKey using the following construction:
//
// ciphertext = R || iv || c || d
// S = Px where (Px, Py) = kB * R
// kE || kM = KDF(S, 32)
// d == MAC(sha256(kM), iv || c)
// msg = AES(kE, iv || c)
func Decrypt(prvKey *secp256k1.PrivateKey, ciphertext []byte) ([]byte, error) {
	const (
		pubKeyLen = 65
		ivLen     = 16
		macSize   = 32
	)
	if len(ciphertext) <= pubKeyLen+ivLen+macSize {
		return nil, errors.New("ciphertext is too small")
	}
	var (
		msgStart = pubKeyLen + ivLen
		msgEnd   = len(ciphertext) - macSize
	)

	r, err := secp256k1.ParsePubKey(ciphertext[:pubKeyLen])
	if err != nil {
		return nil, err
	}
	var (
		s  = secp256k1.GenerateSharedSecret(prvKey, r)
		k  = kdf(s)
		ke = k[:16]
		km = sha256.Sum256(k[16:])
	)

	mac := hmac.New(sha256.New, km[:])
	mac.Write(ciphertext[pubKeyLen:msgEnd]) // iv || c
	if subtle.ConstantTimeCompare(ciphertext[msgEnd:], mac.Sum(nil)) != 1 {
		return nil, errors.New("invalid hmac")
	}

	block, err := aes.NewCipher(ke)
	if err != nil {
		return nil, err
	}
	iv := ciphertext[pubKeyLen : pubKeyLen+ivLen]
	msg := make([]byte, len(ciphertext[msgStart:msgEnd]))
	cipher.NewCTR(block, iv).XORKeyStream(msg, ciphertext[msgStart:msgEnd])
	return msg, nil
}

// Modified NIST SP 800-56: Concatenation Key Derivation Function.
// This function avoids incrementing the counter, and therefore
// looping, since:
// reps = ceil(keydatalen / hashlen)
// and keydatalen = 32 and hashlen = 32
// therefore reps is always 1
func kdf(z []byte) [32]byte {
	h := sha256.New()
	h.Write([]byte{0, 0, 0, 1})
	h.Write(z)
	h.Write([]byte{})
	return *(*[32]byte)(h.Sum(nil))
}
