package rlpx

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	mrand "math/rand"
	"net"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"

	"github.com/indexsupply/x/enr"
	"github.com/indexsupply/x/isxecies"
	"github.com/indexsupply/x/isxhash"
	"github.com/indexsupply/x/isxsecp256k1"
	"github.com/indexsupply/x/rlp"
)

const (
	authVersion   = 4
	eciesOverhead = 65 + 16 + 32 // Pubkey + Nonce + MAC
)

type handshake struct {
	isInitiator bool
	to          *enr.Record
	pk          *secp256k1.PrivateKey
	ephPrvKey   *secp256k1.PrivateKey
	initNonce   [32]byte

	// State management
	sentAuthMsg bool
	receivedAck bool
}

func newHandshake(pk *secp256k1.PrivateKey, to *enr.Record) (*handshake, error) {
	h := &handshake{
		isInitiator: true,
		to:          to,
		pk:          pk,
	}
	// initialize random nonce
	_, err := rand.Read(h.initNonce[:])
	if err != nil {
		return nil, err
	}
	return h, nil
}

// func (h *handshake) sendAuth(conn net.TCPConn) ([]byte, error) {
func (h *handshake) createAuthMsg() (rlp.Item, error) {
	var err error
	// Generate ephemeral ECDH key
	if h.ephPrvKey == nil {
		h.ephPrvKey, err = secp256k1.GeneratePrivateKey()
		if err != nil {
			return rlp.Bytes(nil), err
		}
	}
	// Create shared secret using ephemeral key and remote pub key
	var sharedSecretBytes [32]byte
	copy(sharedSecretBytes[:], secp256k1.GenerateSharedSecret(h.pk, h.to.PublicKey))
	var signedPayload [32]byte
	for i := 0; i < 32; i++ {
		signedPayload[i] = sharedSecretBytes[i] ^ h.initNonce[i]
	}
	// Sig = Sign(Ephemeral Private Key, Shared Secret ^ Nonce)
	sig, err := isxsecp256k1.Sign(h.ephPrvKey, signedPayload)
	if err != nil {
		return rlp.Bytes(nil), err
	}

	ephPubKey := isxsecp256k1.Encode(h.ephPrvKey.PubKey())
	hashedEphPubKey := isxhash.Keccak(ephPubKey[:])
	rawPubKey := isxsecp256k1.Encode(h.pk.PubKey())
	// Auth Body = [Sig, Raw Pub Key, Nonce, Version]
	// return append(append(append(append(sig[:], hashedEphPubKey[:]...), rawPubKey[:]...), h.initNonce[:]...), byte(0)), nil
	return rlp.List(
		rlp.Bytes(sig[:]),
		rlp.Bytes(hashedEphPubKey),
		rlp.Bytes(rawPubKey[:]),
		rlp.Bytes(h.initNonce[:]),
		rlp.Int(authVersion),
	), nil
}

func (h *handshake) seal(body rlp.Item) ([]byte, error) {
	encBody := rlp.Encode(body)
	// pad with random data. needs at least 100 bytes to make it indistinguishable from pre-eip-8 handshakes
	encBody = append(encBody, make([]byte, mrand.Intn(100)+100)...)

	prefix := make([]byte, 2) // prefix is length of the body
	binary.BigEndian.PutUint16(prefix, uint16(len(encBody)+isxecies.Overhead))
	encrypted, err := isxecies.Encrypt(h.to.PublicKey, encBody, prefix)
	return append(prefix, encrypted...), err
}

func (h *handshake) Listen(conn net.TCPConn) error {
	var buf []byte
	_, err := conn.Read(buf)
	if err != nil {
		return err
	}
	fmt.Println(buf)
	return nil
}

func (h *handshake) SendAuth(conn net.TCPConn) error {
	auth, err := h.createAuthMsg()
	if err != nil {
		return err
	}
	b, err := h.seal(auth)
	if err != nil {
		return err
	}
	_, err = conn.Write(b)
	return err
}
