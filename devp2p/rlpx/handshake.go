package rlpx

import (
	"crypto/rand"
	"encoding/binary"
	mrand "math/rand"
	"net"
	"errors"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"

	"github.com/indexsupply/x/ecies"
	"github.com/indexsupply/x/enr"
	"github.com/indexsupply/x/isxsecp256k1"
	"github.com/indexsupply/x/rlp"
)

const (
	authVersion = 4
	prefixLength = 2
)

type handshake struct {
	isInitiator     bool
	localPrvKey     *secp256k1.PrivateKey
	localEphPrvKey  *secp256k1.PrivateKey
	remotePubKey    *secp256k1.PublicKey
	remoteEphPubKey *secp256k1.PublicKey
	initNonce       []byte
	receiverNonce   []byte
}

func newHandshake(localPrvKey *secp256k1.PrivateKey, to *enr.Record) (*handshake, error) {
	h := &handshake{
		remotePubKey: to.PublicKey,
		localPrvKey:  localPrvKey,
	}

	return h, nil
}

func (h *handshake) createAuthMsg() (rlp.Item, error) {
	var err error
	// initialize random nonce
	if h.initNonce == nil {
		h.initNonce = make([]byte, 32)
		_, err := rand.Read(h.initNonce[:])
		if err != nil {
			return rlp.Bytes(nil), err
		}
	}
	// Generate ephemeral ECDH key
	if h.localEphPrvKey == nil {
		h.localEphPrvKey, err = secp256k1.GeneratePrivateKey()
		if err != nil {
			return rlp.Bytes(nil), err
		}
	}
	// Create shared secret using ephemeral key and remote pub key
	var sharedSecretBytes [32]byte
	copy(sharedSecretBytes[:], secp256k1.GenerateSharedSecret(h.localPrvKey, h.remotePubKey))
	var signedPayload [32]byte
	for i := 0; i < 32; i++ {
		signedPayload[i] = sharedSecretBytes[i] ^ h.initNonce[i]
	}
	// Sig = Sign(Ephemeral Private Key, Shared Secret ^ Nonce)
	sig, err := isxsecp256k1.Sign(h.localEphPrvKey, signedPayload)
	if err != nil {
		return rlp.Bytes(nil), err
	}

	rawPubKey := isxsecp256k1.Encode(h.localPrvKey.PubKey())
	// Auth Body = [Sig, Raw Pub Key, Nonce, Version]
	return rlp.List(
		rlp.Bytes(sig[:]),
		rlp.Bytes(rawPubKey[:]),
		rlp.Bytes(h.initNonce),
		rlp.Int(authVersion),
	), nil
}

func (h *handshake) handleAckMsg(sealedAck []byte) error { 
	ackPacket, err := h.unseal(sealedAck)
	if err != nil{
		return err
	}
	remotePubKey, err := ackPacket.At(0).Secp256k1PublicKey()
	if err != nil {
		return err
	}
	remoteNonce, err := ackPacket.At(1).Bytes()
	if err != nil {
		return err
	}
	h.remotePubKey = remotePubKey
	h.receiverNonce = remoteNonce
	return err
}

func (h *handshake) unseal(b []byte) (rlp.Item, error) { 
	if len(b) <= 2 + ecies.Overhead {
		return rlp.Bytes(nil), errors.New("message must be at least 2 + ecies overhead bytes long")
	}
	prefix := b[:prefixLength] // first two bytes are the size
	size := binary.BigEndian.Uint16(prefix)

	encBody := b[prefixLength:prefixLength+size]

	plainBody, err := ecies.Decrypt(h.localPrvKey, encBody, prefix)
	if err != nil {
		return rlp.Bytes(nil), err
	}
	return rlp.Decode(plainBody)
}

func (h *handshake) seal(body rlp.Item) ([]byte, error) {
	encBody := rlp.Encode(body)
	// pad with random data. needs at least 100 bytes to make it indistinguishable from pre-eip-8 handshakes
	encBody = append(encBody, make([]byte, mrand.Intn(100)+100)...)

	prefix := make([]byte, prefixLength) // prefix is length of the ciphertext + overhead
	binary.BigEndian.PutUint16(prefix, uint16(len(encBody)+ecies.Overhead))
	encrypted, err := ecies.Encrypt(h.remotePubKey, encBody, prefix)
	return append(prefix, encrypted...), err
}

func (h *handshake) sendAuth(conn *net.TCPConn) error {
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
