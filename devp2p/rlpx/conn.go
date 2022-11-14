package rlpx

import (
	"net"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/indexsupply/x/enr"
)

// Represents an RLPX connection which wraps a TCPConn to the
// remote and contains state management of the handshake.
type Conn struct {
	h       *handshake
	tcpConn *net.TCPConn
}

func (c *Conn) Listen() error {
	buf := make([]byte, 1280)
	_, err := c.tcpConn.Read(buf)
	if err != nil {
		return err
	}

	err = c.h.handleAckMsg(buf)
	return err
}

func Dial(pk *secp256k1.PrivateKey, to *enr.Record) (*Conn, error) {
	tcp, err := net.DialTCP("tcp", nil, to.TCPAddr())
	if err != nil {
		return nil, err
	}
	h, err := newHandshake(pk, to)
	return &Conn{h: h, tcpConn: tcp}, err
}

func (c *Conn) Handshake() error {
	return c.h.sendAuth(c.tcpConn)
}
