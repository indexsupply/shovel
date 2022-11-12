// Implements the RLPx protocol. Code is based off the devp2p spec defined here:
// https://github.com/ethereum/devp2p/blob/master/rlpx.md
package rlpx

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"errors"
	"fmt"
	"hash"
	"net"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"golang.org/x/crypto/sha3"

	"github.com/indexsupply/x/bint"
	"github.com/indexsupply/x/enr"
	"github.com/indexsupply/x/isxerrors"
	"github.com/indexsupply/x/isxhash"
	"github.com/indexsupply/x/rlp"
)

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

type session struct {
	Verbose bool

	conn          net.Conn
	local, remote *enr.Record
	ig, eg        *mstate
}

func (s *session) log(format string, args ...any) {
	if s.Verbose {
		fmt.Printf(format, args...)
	}
}

type handshake struct {
	initiator bool

	local, remote        *enr.Record
	auth, ack            []byte
	initNonce, respNonce [16]byte

	localEphPrvKey  *secp256k1.PrivateKey
	remoteEphPubKey *secp256k1.PublicKey
}

func New(conn net.Conn, hs *handshake) (*session, error) {
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
			hs.respNonce[:],
			hs.initNonce[:]...,
		))...,
	))
	aesSecret := isxhash.Keccak(append(ephKey, sharedSecret...))
	macSecret := isxhash.Keccak(append(ephKey, aesSecret...))

	var (
		s   = &session{conn: conn}
		err error
	)
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
		rnonce[i] = macSecret[i] ^ hs.respNonce[i]
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

func (s *session) read(buf []byte) ([]byte, error) {
	if len(buf) < 32 {
		return nil, errors.New("buf too small for 32byte header")
	}
	if !hmac.Equal(s.ig.header(buf[:16]), buf[16:32]) {
		return nil, errors.New("invalid header mac")
	}
	s.ig.stream.XORKeyStream(buf[:16], buf[:16])
	var (
		frameSize = bint.Decode(buf[:3])
		frameEnd  = 32 + 16
		frame     = buf[32:frameEnd]
		frameMac  = buf[frameEnd : frameEnd+16]
	)
	if !hmac.Equal(s.ig.frame(frame), frameMac) {
		return nil, errors.New("invalid frame mac")
	}
	s.ig.stream.XORKeyStream(frame, frame)
	return frame[:frameSize], nil
}

// Assembles item into an RLPx frame and writes it to
// the session's TCP connection.
func (s *session) write(id byte, data []byte) error {
	frameSize, n := bint.Encode(nil, uint64(len(data)+1)) //include id
	if n > 3 {
		return errors.New("data is too large")
	}

	// header only contains size since:
	// header-data = [capability-id, context-id]
	// is a zero array.
	var (
		header           = make([]byte, 16)
		headerCiphertext []byte
		headerMac        []byte
	)
	copy(header[:], frameSize)
	headerCiphertext = make([]byte, len(header))
	s.eg.stream.XORKeyStream(headerCiphertext, header)
	headerMac = s.eg.header(headerCiphertext)

	//TODO(ryan): ensure len(frame) % 16 == 0
	var (
		frameData       = make([]byte, 14)
		frameCiphertext []byte
		frameMac        []byte
	)
	frameData = append(frameData, rlp.Encode(rlp.Byte(id))...)
	frameData = append(frameData, data...)
	frameCiphertext = make([]byte, len(frameData))
	s.eg.stream.XORKeyStream(frameCiphertext, frameData)
	frameMac = s.eg.frame(frameCiphertext)

	var frame []byte
	frame = append(frame, headerCiphertext...)
	frame = append(frame, headerMac...)
	frame = append(frame, frameCiphertext...)
	frame = append(frame, frameMac...)

	m, err := s.conn.Write(frame)
	s.log(">%x (%d)", id, m)
	return isxerrors.Errorf("write frame: %w", err)
}

func (s *session) Hello() error {
	err := s.write(0x00, rlp.Encode(rlp.List(
		rlp.Int(5),
		rlp.String("indexsupply/0"),
		rlp.List(
			rlp.String("p2p"),
			rlp.Int(5),
		),
		rlp.Uint16(s.local.TcpPort),
		rlp.Secp256k1PublicKey(s.local.PublicKey),
	)))
	return isxerrors.Errorf("writing hello msg: %w", err)
}

func (s *session) handleHello() {
}

var disconnectReasons = map[byte]string{
	0x00: "Disconnect requested",
	0x01: "TCP sub-system error",
	//...
}

func (s *session) Disconnect() {
}

func (s *session) handleDisconnect() {
}
