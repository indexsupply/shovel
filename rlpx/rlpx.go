// Implements the RLPx protocol. Code is based off the devp2p spec defined here:
// https://github.com/ethereum/devp2p/blob/master/rlpx.md
package rlpx

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"errors"
	"fmt"
	"hash"
	mrand "math/rand"
	"net"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"golang.org/x/crypto/sha3"

	"github.com/indexsupply/x/bint"
	"github.com/indexsupply/x/ecies"
	"github.com/indexsupply/x/enr"
	"github.com/indexsupply/x/isxerrors"
	"github.com/indexsupply/x/isxhash"
	"github.com/indexsupply/x/isxsecp256k1"
	"github.com/indexsupply/x/rlp"
)

type session struct {
	Verbose bool

	conn   net.Conn
	local  *enr.Record
	ig, eg *mstate
}

func (s *session) log(format string, args ...any) {
	if s.Verbose {
		fmt.Printf(format, args...)
	}
}

func Session(l *enr.Record, hs *handshake) (*session, error) {
	err := hs.complete()
	if err != nil {
		return nil, err
	}
	s := &session{local: l}

	//static-shared-secret = ecdh.agree(privkey, remote-pubk)
	//ephemeral-key = ecdh.agree(ephemeral-privkey, remote-ephemeral-pubk)
	//shared-secret = keccak256(ephemeral-key || keccak256(nonce || initiator-nonce))
	//aes-secret = keccak256(ephemeral-key || shared-secret)
	//mac-secret = keccak256(ephemeral-key || aes-secret)
	ephKey := secp256k1.GenerateSharedSecret(
		hs.localEphPrvKey,
		hs.remoteEphPubKey,
	)
	sharedSecret := isxhash.Keccak(append(
		ephKey,
		isxhash.Keccak(append(
			hs.recipientNonce[:],
			hs.initNonce[:]...,
		))...,
	))
	aesSecret := isxhash.Keccak(append(ephKey, sharedSecret...))
	macSecret := isxhash.Keccak(append(ephKey, aesSecret...))

	s.ig, err = newmstate(aesSecret, macSecret)
	if err != nil {
		return nil, err
	}
	s.eg, err = newmstate(aesSecret, macSecret)
	if err != nil {
		return nil, err
	}

	var inonce, rnonce [16]byte
	for i := 0; i < 16; i++ {
		inonce[i] = macSecret[i] ^ hs.initNonce[i]
		rnonce[i] = macSecret[i] ^ hs.recipientNonce[i]
	}

	if hs.initiator {
		//egress-mac = keccak256.init((mac-secret ^ recipient-nonce) || auth)
		//ingress-mac = keccak256.init((mac-secret ^ initiator-nonce) || ack)
		s.eg.hash = sha3.NewLegacyKeccak256()
		s.eg.hash.Write(rnonce[:])
		s.eg.hash.Write(hs.auth)
		s.ig.hash = sha3.NewLegacyKeccak256()
		s.ig.hash.Write(inonce[:])
		s.ig.hash.Write(hs.ack)
	} else {
		//egress-mac = keccak256.init((mac-secret ^ initiator-nonce) || ack)
		//ingress-mac = keccak256.init((mac-secret ^ recipient-nonce) || auth)
		s.eg.hash = sha3.NewLegacyKeccak256()
		s.eg.hash.Write(inonce[:])
		s.eg.hash.Write(hs.ack)
		s.ig.hash = sha3.NewLegacyKeccak256()
		s.ig.hash.Write(rnonce[:])
		s.ig.hash.Write(hs.auth)
	}

	return s, nil
}

type mstate struct {
	block  cipher.Block
	hash   hash.Hash
	stream cipher.Stream
}

func newmstate(aesSecret, macSecret []byte) (*mstate, error) {
	eb, err := aes.NewCipher(aesSecret)
	if err != nil {
		return nil, err
	}
	es := cipher.NewCTR(eb, make([]byte, 16))

	mb, err := aes.NewCipher(macSecret)
	if err != nil {
		return nil, err
	}
	h := sha3.NewLegacyKeccak256()
	return &mstate{
		hash:   h,
		block:  mb,
		stream: es,
	}, nil
}

// updates hash using the following devp2p construction:
//
// header-mac-seed = aes(mac-secret, keccak256.digest(egress-mac)[:16]) ^ header-ciphertext
// egress-mac = keccak256.update(egress-mac, header-mac-seed)
// header-mac = keccak256.digest(egress-mac)[:16]
func (ms *mstate) header(h []byte) []byte {
	prev := ms.hash.Sum(nil)
	dest := make([]byte, 16)
	ms.block.Encrypt(dest, prev[:16])
	for i := range dest {
		dest[i] ^= h[i]
	}
	ms.hash.Write(dest)
	return ms.hash.Sum(nil)[:16]
}

// updates hash using the following devp2p construction:
//
// egress-mac = keccak256.update(egress-mac, frame-ciphertext)
// frame-mac-seed = aes(mac-secret, keccak256.digest(egress-mac)[:16]) ^ keccak256.digest(egress-mac)[:16]
// egress-mac = keccak256.update(egress-mac, frame-mac-seed)
// frame-mac = keccak256.digest(egress-mac)[:16]
func (ms *mstate) frame(fct []byte) []byte {
	ms.hash.Write(fct)
	prev := ms.hash.Sum(nil)
	dest := make([]byte, 16)
	ms.block.Encrypt(dest, prev[:16])
	for i := range dest {
		dest[i] ^= prev[i]
	}
	ms.hash.Write(dest)
	return ms.hash.Sum(nil)[:16]
}

func (s *session) decode(buf []byte) (byte, rlp.Item, error) {
	if len(buf) < 32 {
		return 0, rlp.Item{}, errors.New("buf too small for 32byte header")
	}
	want := s.ig.header(buf[:16])
	if !hmac.Equal(want, buf[16:32]) {
		return 0, rlp.Item{}, errors.New("invalid header mac")
	}
	s.ig.stream.XORKeyStream(buf[:16], buf[:16])
	var (
		frameSize = bint.Decode(buf[:3])
		frameEnd  = 32 + frameSize + (16 - frameSize%16)
		frame     = buf[32:frameEnd]
		frameMac  = buf[frameEnd : frameEnd+16]
	)
	if !hmac.Equal(s.ig.frame(frame), frameMac) {
		return 0, rlp.Item{}, errors.New("invalid frame mac")
	}
	s.ig.stream.XORKeyStream(frame, frame)
	code := frame[0:1]
	item, err := rlp.Decode(frame[1:frameSize])
	return code[0], item, isxerrors.Errorf("rlp decoding frame: %w", err)
}

// Encodees data into an RLPx frame
//
// frame = header-ciphertext || header-mac || frame-ciphertext || frame-mac
// header-ciphertext = aes(aes-secret, header)
// header = frame-size || header-data || header-padding
// header-data = [capability-id, context-id]
// capability-id = integer, always zero
// context-id = integer, always zero
// header-padding = zero-fill header to 16-byte boundary
// frame-ciphertext = aes(aes-secret, frame-data || frame-padding)
// frame-padding = zero-fill frame-data to 16-byte boundary
func (s *session) encode(id byte, data []byte) []byte {
	// Per the spec, the header contains: size, data, and padding.
	// However, the data (eg [capability-id, context-id]) is unused.
	// Therefore, we leave the header-data as a list of zero bytes.
	// Padding is addressed by pre-allocating a 16 byte header.
	header := make([]byte, 16)
	bint.Encode(header[:3], uint64(len(data)+1)) //include id
	s.eg.stream.XORKeyStream(header, header)
	var (
		headerCiphertext = header
		headerMac        = s.eg.header(headerCiphertext)
	)

	var (
		frameData       []byte
		frameCiphertext []byte
		frameMac        []byte
	)
	frameData = append(frameData, rlp.Encode(rlp.Byte(id))...)
	frameData = append(frameData, data...)
	padding := 16 - len(frameData)%16
	if padding != 0 {
		frameData = append(frameData, make([]byte, padding)...)
	}
	frameCiphertext = make([]byte, len(frameData))
	s.eg.stream.XORKeyStream(frameCiphertext, frameData)
	frameMac = s.eg.frame(frameCiphertext)

	var frame []byte
	frame = append(frame, headerCiphertext...)
	frame = append(frame, headerMac...)
	frame = append(frame, frameCiphertext...)
	frame = append(frame, frameMac...)
	return frame
}

func (s *session) Hello() ([]byte, error) {
	return s.encode(0x00, rlp.Encode(rlp.List(
		rlp.Int(5),
		rlp.String("indexsupply/0"),
		rlp.List(
			rlp.String("p2p"),
			rlp.Int(5),
		),
		rlp.Uint16(s.local.TcpPort),
		rlp.Secp256k1PublicKey(s.local.PublicKey),
	))), nil
}

func (s *session) HandleHello(d []byte) error {
	_, item, err := s.decode(d)
	if err != nil {
		return isxerrors.Errorf("decoding hello frame: %w", err)
	}
	id, err := item.At(1).String()
	if err != nil {
		return isxerrors.Errorf("reading id: %w", err)
	}
	s.log("hello from %s", id)
	return nil
}

type handshake struct {
	initiator bool

	localPrvKey, localEphPrvKey   *secp256k1.PrivateKey
	remotePubKey, remoteEphPubKey *secp256k1.PublicKey

	initNonce, recipientNonce [32]byte

	// store results for session initialization
	auth, ack []byte
}

func (h *handshake) complete() error {
	if h.initNonce == [32]byte{} {
		return errors.New("missing init nonce")
	}
	if h.recipientNonce == [32]byte{} {
		return errors.New("missing recipient nonce")
	}
	if h.remotePubKey == nil {
		return errors.New("missing remote public key")
	}
	if h.remoteEphPubKey == nil {
		return errors.New("missing remote eph public key")
	}
	return nil
}

func Initiator(l *secp256k1.PrivateKey, r *secp256k1.PublicKey) *handshake {
	return &handshake{
		initiator:    true,
		localPrvKey:  l,
		remotePubKey: r,
	}
}

func Recipient(l *secp256k1.PrivateKey) *handshake {
	return &handshake{
		initiator:   false,
		localPrvKey: l,
	}
}

// Assembles an Auth message according to the following:
//
// auth = auth-size || enc-auth-body
// auth-size = size of enc-auth-body, encoded as a big-endian 16-bit integer
// auth-vsn = 4
// auth-body = [sig, initiator-pubk, initiator-nonce, auth-vsn, ...]
// shared-secret = ecdh.agree(privkey, remote-pubk)
// sig = secp256k1.sign(ephemeral-privkey , shared-secret ^ initiator-nonce)
// enc-auth-body = ecies.encrypt(recipient-pubk, auth-body || auth-padding, auth-size)
// auth-padding = arbitrary data
func (h *handshake) Auth() ([]byte, error) {
	_, err := rand.Read(h.initNonce[:])
	if err != nil {
		return nil, err
	}
	h.localEphPrvKey, err = secp256k1.GeneratePrivateKey()
	if err != nil {
		return nil, err
	}

	var (
		ss = secp256k1.GenerateSharedSecret(h.localPrvKey, h.remotePubKey)
		b  [32]byte
	)
	for i := 0; i < 32; i++ {
		b[i] = ss[i] ^ h.initNonce[i]
	}
	sig, err := isxsecp256k1.Sign(h.localEphPrvKey, b)
	if err != nil {
		return nil, err
	}
	const authVersion = 4
	authBody := rlp.Encode(rlp.List(
		rlp.Bytes(sig[:]),
		rlp.Secp256k1PublicKey(h.localPrvKey.PubKey()),
		rlp.Bytes(h.initNonce[:]),
		rlp.Int(authVersion),
	))
	authBody = append(authBody, make([]byte, mrand.Intn(100)+100)...)

	var authSize [2]byte
	bint.Encode(authSize[:], uint64(len(authBody)+ecies.Overhead))
	encAuthBody, err := ecies.Encrypt(h.remotePubKey, authBody, authSize[:])
	if err != nil {
		return nil, err
	}

	h.auth = append(authSize[:], encAuthBody...)
	return h.auth, nil
}

func (h *handshake) HandleAuth(d []byte) error {
	h.auth = d
	if len(d) <= 2+ecies.Overhead {
		return errors.New("message must be at least 2 + ecies overhead bytes long")
	}
	size := bint.Decode(d[:2])
	authBody, err := ecies.Decrypt(h.localPrvKey, d[2:2+size], d[:2])
	if err != nil {
		return isxerrors.Errorf("decrypting auth: %w", err)
	}
	authItem, err := rlp.Decode(authBody)
	if err != nil {
		return err
	}

	sig, err := authItem.At(0).Bytes65()
	if err != nil {
		return err
	}
	h.remotePubKey, err = authItem.At(1).Secp256k1PublicKey()
	if err != nil {
		return err
	}
	h.initNonce, err = authItem.At(2).Bytes32()
	if err != nil {
		return err
	}
	var (
		ss = secp256k1.GenerateSharedSecret(h.localPrvKey, h.remotePubKey)
		b  [32]byte
	)
	for i := 0; i < 32; i++ {
		b[i] = ss[i] ^ h.initNonce[i]
	}
	h.remoteEphPubKey, err = isxsecp256k1.Recover(sig, b)
	if err != nil {
		return err
	}
	return nil
}

// ack = ack-size || enc-ack-body
// ack-size = size of enc-ack-body, encoded as a big-endian 16-bit integer
// ack-vsn = 4
// ack-body = [recipient-ephemeral-pubk, recipient-nonce, ack-vsn, ...]
// enc-ack-body = ecies.encrypt(initiator-pubk, ack-body || ack-padding, ack-size)
// ack-padding = arbitrary data
func (h *handshake) Ack() ([]byte, error) {
	if h.initiator {
		return nil, errors.New("cannot send ack message when you are the initiator")
	}
	_, err := rand.Read(h.recipientNonce[:])
	if err != nil {
		return nil, err
	}
	h.localEphPrvKey, err = secp256k1.GeneratePrivateKey()
	if err != nil {
		return nil, err
	}
	const authVersion = 4
	ackBody := rlp.Encode(rlp.List(
		rlp.Secp256k1PublicKey(h.localEphPrvKey.PubKey()),
		rlp.Bytes(h.recipientNonce[:]),
		rlp.Int(authVersion),
	))
	ackBody = append(ackBody, make([]byte, mrand.Intn(100)+100)...)
	var ackSize [2]byte
	bint.Encode(ackSize[:], uint64(len(ackBody)+ecies.Overhead))
	encAckBody, err := ecies.Encrypt(h.remotePubKey, ackBody, ackSize[:])
	if err != nil {
		return nil, err
	}
	h.ack = append(ackSize[:], encAckBody...)
	return h.ack, nil
}

func (h *handshake) HandleAck(d []byte) error {
	h.ack = d
	if len(d) <= 2+ecies.Overhead {
		return errors.New("message must be at least 2 + ecies overhead bytes long")
	}
	size := bint.Decode(d[:2])
	ackBody, err := ecies.Decrypt(h.localPrvKey, d[2:2+size], d[:2])
	if err != nil {
		return isxerrors.Errorf("decrypting ack: %w", err)
	}
	ackItem, err := rlp.Decode(ackBody)
	if err != nil {
		return err
	}
	h.remoteEphPubKey, err = ackItem.At(0).Secp256k1PublicKey()
	if err != nil {
		return err
	}
	h.recipientNonce, err = ackItem.At(1).Bytes32()
	return err
}
