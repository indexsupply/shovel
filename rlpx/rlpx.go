package rlpx

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"fmt"
	"net"

	"github.com/indexsupply/x/bint"
	"github.com/indexsupply/x/enr"
	"github.com/indexsupply/x/isxerrors"
	"github.com/indexsupply/x/rlp"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
)

type session struct {
	Verbose bool

	self *enr.Record
	peer *enr.Record
	conn *net.TCPConn

	ke, km *secp256k1.PrivateKey
}

func (s *session) log(format string, args ...any) {
	if s.Verbose {
		fmt.Printf(format, args...)
	}
}

func (s *session) Hello() error {
	err := s.write(0x00, rlp.List(
		rlp.Int(5),
		rlp.String("indexsupply/0"),
		rlp.List(
			rlp.String("p2p"),
			rlp.Int(5),
		),
		rlp.Uint16(s.self.TcpPort),
		rlp.Secp256k1PublicKey(s.self.PublicKey),
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

func (s *session) write(id byte, item rlp.Item) error {
	b, err := s.frame(id, rlp.Encode(item))
	if err != nil {
		return isxerrors.Errorf("creating frame: %w", err)
	}
	n, err := s.conn.Write(b)
	s.log(">%x (%d)", id, n)
	return isxerrors.Errorf("tcp write: %w", err)
}

func (s *session) frame(msgID byte, data []byte) ([]byte, error) {
	frameSize, n := bint.Encode(uint64(len(data) + 1)) //include msgID
	if n > 3 {
		return nil, errors.New("data is too large")
	}

	iv := make([]byte, 16) // 0 iv for ephemeral key
	block, err := aes.NewCipher(s.ke.Serialize())
	if err != nil {
		return nil, err
	}

	// header only contains size since:
	// header-data = [capability-id, context-id]
	// is a zero array.
	header := make([]byte, 16)
	copy(header[:], frameSize)
	hct := make([]byte, len(header))
	cipher.NewCTR(block, iv).XORKeyStream(hct, header)

	var hm []byte
	mac := hmac.New(sha256.New, s.km.Serialize())
	mac.Write(header)
	hm = mac.Sum(nil)

	//TODO(ryan): ensure len(frame) % 16 == 0
	var frame []byte
	frame = append(frame, rlp.Encode(rlp.Byte(msgID))...)
	frame = append(frame, data...)
	fct := make([]byte, len(frame))
	cipher.NewCTR(block, iv).XORKeyStream(fct, frame)

	var fm []byte
	mac.Reset()
	mac.Write(frame)
	fm = mac.Sum(nil)

	var f []byte
	f = append(f, hct...)
	f = append(f, hm...)
	f = append(f, fct...)
	f = append(f, fm...)
	return f, nil
}
