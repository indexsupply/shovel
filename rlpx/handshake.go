package rlpx

import (
	"crypto/rand"
	"errors"
	// mrand "math/rand"
	"net"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"

	"github.com/indexsupply/x/bint"
	"github.com/indexsupply/x/ecies"
	"github.com/indexsupply/x/enr"
	"github.com/indexsupply/x/isxsecp256k1"
	"github.com/indexsupply/x/rlp"
)

const (
	authVersion  = 4
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

func newHandshake(localPrvKey *secp256k1.PrivateKey, to *enr.Record) *handshake {
	return &handshake{
		localPrvKey:  localPrvKey,
		remotePubKey: to.PublicKey,
	}
}

// createAuthMsg assembles an Auth message according to the following:
//
// iv = initiator nonce (randomly generated)
// eph-k = ephemeral secp256k1 public key (randomly generated)
// pub-k = own ENR public key
// shared-secret = shared secret on secp256k1 curve derived from own private key and receiver's pub key
// sig-payload = shared-secret ^ iv
// sig = signature using the ephemeral key: Sign(eph-k, sig-payload)
// vsn = auth version (which is 4)
// msg = sig || pub-k || iv || vsn
func (h *handshake) createAuthMsg() (rlp.Item, error) {
	var err error
	// initialize random nonce
	h.initNonce = make([]byte, 32)
	_, err = rand.Read(h.initNonce[:])
	if err != nil {
		return rlp.Item{}, err
	}
	// Generate ephemeral ECDH key
	h.localEphPrvKey, err = secp256k1.GeneratePrivateKey()
	if err != nil {
		return rlp.Item{}, err
	}
	// Sig = Sign(Ephemeral Private Key, Shared Secret ^ Nonce)
	sig, err := isxsecp256k1.Sign(h.localEphPrvKey, signaturePayload(h.localPrvKey, h.remotePubKey, h.initNonce))
	if err != nil {
		return rlp.Item{}, err
	}

	// Auth Body = [Sig, Raw Pub Key, Nonce, Version]
	return rlp.List(
		rlp.Bytes(sig[:]),
		rlp.Secp256k1RawPublicKey(h.localPrvKey.PubKey()),
		rlp.Bytes(h.initNonce),
		rlp.Int(authVersion),
	), nil
}

func (h *handshake) sendAuth(conn net.Conn) error {
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

// createAckMsg assembles the Ack response to an Auth message under the following construction:
// iv = receiver's nonce (randomly generated)
// eph-k = receiver's ephemeral key on the secp256k1 curve (chosen at random)
// vsn = version (which equals 4)
// msg = eph-k || iv || vsn
// For more details, see https://eips.ethereum.org/EIPS/eip-8#specification
func (h *handshake) createAckMsg() (rlp.Item, error) {
	var err error
	if h.isInitiator {
		return rlp.Item{}, errors.New("cannot send ack message when you are the initiator")
	}
	h.receiverNonce = make([]byte, 32)
	_, err = rand.Read(h.receiverNonce[:])
	if err != nil {
		return rlp.Item{}, err
	}
	// Generate ephemeral ECDH key
	h.localEphPrvKey, err = secp256k1.GeneratePrivateKey()
	if err != nil {
		return rlp.Item{}, err
	}
	return rlp.List(
		rlp.Secp256k1RawPublicKey(h.localEphPrvKey.PubKey()),
		rlp.Bytes(h.receiverNonce),
		rlp.Int(authVersion),
	), nil
}

func (h *handshake) sendAck(conn net.Conn) error {
	ack, err := h.createAckMsg()
	if err != nil {
		return err
	}
	b, err := h.seal(ack)
	if err != nil {
		return err
	}
	_, err = conn.Write(b)
	return err
}

func (h *handshake) handleAuthMsg(sealedAuth []byte, conn net.Conn) error {
	h.isInitiator = false
	authPacket, err := h.unseal(sealedAuth)
	if err != nil {
		return err
	}
	rpk, err := authPacket.At(1).Secp256k1PublicKey()
	if err != nil {
		return err
	}
	iv, err := authPacket.At(2).Bytes()
	if err != nil {
		return err
	}
	sig, err := authPacket.At(0).Bytes()
	if err != nil {
		return err
	}
	if len(sig) != 65 {
		return errors.New("auth signature is not 65 bytes")
	}
	var sigBytes [65]byte
	copy(sigBytes[:], sig)
	// Recover the signature
	ephPk, err := isxsecp256k1.Recover(sigBytes, signaturePayload(h.localPrvKey, h.remotePubKey, iv))
	if err != nil {
		return err
	}
	h.remotePubKey = rpk
	h.remoteEphPubKey = ephPk
	h.initNonce = iv
	// send the ack
	return h.sendAck(conn)
}

// handleAckMsg unseals an ack message received over the wire
// and parses the remote pub key and
func (h *handshake) handleAckMsg(sealedAck []byte) error {
	ackPacket, err := h.unseal(sealedAck)
	if err != nil {
		return err
	}
	pk, err := ackPacket.At(0).Secp256k1PublicKey()
	if err != nil {
		return err
	}
	iv, err := ackPacket.At(1).Bytes()
	if err != nil {
		return err
	}
	h.remoteEphPubKey = pk
	h.receiverNonce = iv
	return err
}

func (h *handshake) unseal(b []byte) (rlp.Item, error) {
	if h.localPrvKey == nil {
		return rlp.Item{}, errors.New("unable to decrypt, missing local private key")
	}
	if len(b) <= 2+ecies.Overhead {
		return rlp.Item{}, errors.New("message must be at least 2 + ecies overhead bytes long")
	}
	prefix := b[:prefixLength] // first two bytes are the size
	size := bint.Decode(prefix)

	encBody := b[prefixLength : prefixLength+size]

	plainBody, err := ecies.Decrypt(h.localPrvKey, encBody, prefix)
	if err != nil {
		return rlp.Item{}, err
	}
	return rlp.Decode(plainBody)
}

func (h *handshake) seal(body rlp.Item) ([]byte, error) {
	if h.remotePubKey == nil {
		return nil, errors.New("unable to encrypt, missing remote pub key")
	}
	encBody := rlp.Encode(body)
	// pad with random data. needs at least 100 bytes to make it indistinguishable from pre-eip-8 handshakes
	// encBody = append(encBody, make([]byte, mrand.Intn(100)+100)...)

	prefix := make([]byte, prefixLength) // prefix is length of the ciphertext + overhead
	prefix = bint.Encode(prefix, uint64(len(encBody)+ecies.Overhead))
	encrypted, err := ecies.Encrypt(h.remotePubKey, encBody, prefix)
	return append(prefix, encrypted...), err
}

// forms a 32 byte signature payload for auth in the form of shared-secret ^ nonce.
// The shared secret is derived from the local private key and remote public key on the
// secp256k1 curve.
func signaturePayload(privKey *secp256k1.PrivateKey, remoteKey *secp256k1.PublicKey, nonce []byte) [32]byte {
	var ss [32]byte
	secret := secp256k1.GenerateSharedSecret(privKey, remoteKey)
	copy(ss[:], secret)
	var sigPayload [32]byte
	for i := 0; i < 32; i++ {
		sigPayload[i] = ss[i] ^ nonce[i]
	}
	return sigPayload
}
