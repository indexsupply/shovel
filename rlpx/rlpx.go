package rlpx

import (
	"fmt"
	"net"

	"github.com/indexsupply/x/enr"
	"github.com/indexsupply/x/isxerrors"
	"github.com/indexsupply/x/rlp"
)

type session struct {
	Verbose bool

	self *enr.Record
	peer *enr.Record
	conn *net.TCPConn
}

func (s *session) log(format string, args ...any) {
	if s.Verbose {
		fmt.Printf(format, args...)
	}
}

func (s *session) Hello() error {
	item := rlp.List(
		rlp.Int(5),
		rlp.String("indexsupply/0"),
		rlp.List(
			rlp.String("p2p"),
			rlp.Int(5),
		),
		rlp.Uint16(s.self.TcpPort),
		rlp.Secp256k1PublicKey(s.self.PublicKey),
	)
	n, err := s.conn.Write(rlp.Encode(item))
	s.log(">hello %s (%d)", s.peer, n)
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
