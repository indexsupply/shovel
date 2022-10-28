package rlpx

import (
	"crypto/rand"
	"fmt"
	"net"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/indexsupply/x/enr"
	"github.com/indexsupply/x/eth"
	// "github.com/indexsupply/x/rlp"
)

const (
	authVersion = 4
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
func (h *handshake) createAuthMsg() ([]byte, error) {
	var err error
	// Generate ephemeral ECDH key
	if h.ephPrvKey == nil {
		h.ephPrvKey, err = secp256k1.GeneratePrivateKey()
		if err != nil {
			return nil, err
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
	sig, err := eth.Secp256k1Sign(h.ephPrvKey, signedPayload)
	if err != nil {
		return nil, err
	}

	ephPubKey := eth.Secp256k1PubKeyBytes(h.ephPrvKey.PubKey())
	hashedEphPubKey := eth.Keccak(ephPubKey[:])
	rawPubKey := eth.Secp256k1PubKeyBytes(h.pk.PubKey())
	// Auth Body = [S, Raw Pub Key, Nonce, Version]
	return append(append(append(append(sig[:], hashedEphPubKey[:]...), rawPubKey[:]...), h.initNonce[:]...), byte(0)), nil
	// authBody := rlp.List(
	// 	rlp.Bytes(sig),
	// 	rlp.Bytes(hashedEphPubKey),
	// 	rlp.Bytes(rawPubKey[:]),
	// 	rlp.Bytes(h.initNonce[:]),
	// 	rlp.Int(authVersion),
	// )
	// TODO: packet = encrypt authBody
	// _, err = conn.Write(packet)
	// return rlp.Encode(authBody), err
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
