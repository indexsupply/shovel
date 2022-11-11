package rlpx

import (
	"crypto/aes"
	"crypto/cipher"
	"errors"
	"fmt"
	"hash"
	"net"

	"github.com/indexsupply/x/bint"
	"github.com/indexsupply/x/enr"
	"github.com/indexsupply/x/isxerrors"
	"github.com/indexsupply/x/rlp"
)

type session struct {
	Verbose bool

	conn *net.TCPConn

	local  *enr.Record
	remote *enr.Record

	ke       []byte
	keStream cipher.Stream

	km         []byte
	kmBlock    cipher.Block
	imac, emac hash.Hash
}

func (s *session) log(format string, args ...any) {
	if s.Verbose {
		fmt.Printf(format, args...)
	}
}

func New(ke, km []byte) (*session, error) {
	s := new(session)

	s.ke = ke
	keblock, err := aes.NewCipher(s.ke)
	if err != nil {
		return nil, err
	}
	iv := make([]byte, 16) // 0 iv for ephemeral key
	s.keStream = cipher.NewCTR(keblock, iv)

	s.km = km
	s.kmBlock, err = aes.NewCipher(s.km)
	if err != nil {
		return nil, err
	}

	return s, nil
}

func (s *session) egressHeaderMac(header []byte) []byte {
	prev := s.emac.Sum(nil)
	dest := make([]byte, 16)
	s.kmBlock.Encrypt(dest, prev[:16])
	for i := range dest {
		dest[i] ^= header[i]
	}
	s.emac.Write(dest)
	return s.emac.Sum(nil)[:16]
}

func (s *session) egressFrameMac(fct []byte) []byte {
	s.emac.Write(fct)
	prev := s.emac.Sum(nil)
	dest := make([]byte, 16)
	s.kmBlock.Encrypt(dest, prev[:16])
	for i := range dest {
		dest[i] ^= prev[i]
	}
	s.emac.Write(dest)
	return s.emac.Sum(nil)[:16]
}

// Assembles item into an RLPx frame and writes it to
// the session's TCP connection.
func (s *session) write(id byte, item rlp.Item) error {
	data := rlp.Encode(item)
	frameSize, n := bint.Encode(uint64(len(data) + 1)) //include id
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
	s.keStream.XORKeyStream(headerCiphertext, header)
	headerMac = s.egressHeaderMac(headerCiphertext)

	//TODO(ryan): ensure len(frame) % 16 == 0
	var (
		frameData       []byte
		frameCiphertext []byte
		frameMac        []byte
	)
	frameData = append(frameData, rlp.Encode(rlp.Byte(id))...)
	frameData = append(frameData, data...)
	frameCiphertext = make([]byte, len(frameData))
	s.keStream.XORKeyStream(frameCiphertext, frameData)
	frameMac = s.egressFrameMac(frameCiphertext)

	var frame []byte
	frame = append(frame, header...)
	frame = append(frame, headerMac...)
	frame = append(frame, frameCiphertext...)
	frame = append(frame, frameMac...)

	m, err := s.conn.Write(frame)
	s.log(">%x (%d)", id, m)
	return isxerrors.Errorf("write frame: %w", err)
}

func (s *session) Hello() error {
	err := s.write(0x00, rlp.List(
		rlp.Int(5),
		rlp.String("indexsupply/0"),
		rlp.List(
			rlp.String("p2p"),
			rlp.Int(5),
		),
		rlp.Uint16(s.local.TcpPort),
		rlp.Secp256k1PublicKey(s.local.PublicKey),
	))
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

func (s *session) read(frame []byte) error {
	return nil
}
