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
)

type session struct {
	Verbose bool

	local  *enr.Record
	remote *enr.Record
	conn   *net.TCPConn

	ke, km []byte
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

// Assembles item into an RLPx frame and writes it to
// the session's TCP connection.
func (s *session) write(id byte, item rlp.Item) error {
	data := rlp.Encode(item)
	frameSize, n := bint.Encode(uint64(len(data) + 1)) //include id
	if n > 3 {
		return errors.New("data is too large")
	}

	iv := make([]byte, 16) // 0 iv for ephemeral key
	block, err := aes.NewCipher(s.ke)
	if err != nil {
		return err
	}

	// header only contains size since:
	// header-data = [capability-id, context-id]
	// is a zero array.
	header := make([]byte, 16)
	copy(header[:], frameSize)
	hct := make([]byte, len(header))
	cipher.NewCTR(block, iv).XORKeyStream(hct, header)

	var hm []byte
	mac := hmac.New(sha256.New, s.km)
	mac.Write(header)
	hm = mac.Sum(nil)

	//TODO(ryan): ensure len(frame) % 16 == 0
	var frameData []byte
	frameData = append(frameData, rlp.Encode(rlp.Byte(id))...)
	frameData = append(frameData, data...)
	fct := make([]byte, len(frameData))
	cipher.NewCTR(block, iv).XORKeyStream(fct, frameData)

	var fm []byte
	mac.Reset()
	mac.Write(fct)
	fm = mac.Sum(nil)

	var frame []byte
	frame = append(frame, hct...)
	frame = append(frame, hm...)
	frame = append(frame, fct...)
	frame = append(frame, fm...)

	m, err := s.conn.Write(frame)
	s.log(">%x (%d)", id, m)
	return isxerrors.Errorf("write frame: %w", err)
}
