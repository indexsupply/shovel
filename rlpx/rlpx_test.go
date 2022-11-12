package rlpx

import (
	"net"
	"net/netip"
	"testing"

	"github.com/indexsupply/x/enr"
	"github.com/indexsupply/x/rlp"
	"github.com/indexsupply/x/tc"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
)

func TestReadWrite(t *testing.T) {
	c1, c2 := testConn(t)
	p1 := prv(t)
	p2 := prv(t)
	n1 := testNode(t, c1)
	n2 := testNode(t, c2)
	h1 := &handshake{
		initiator:       true,
		local:           n1,
		remote:          n2,
		localEphPrvKey:  p1,
		remoteEphPubKey: p2.PubKey(),
	}
	h2 := &handshake{
		local:           n2,
		remote:          n1,
		localEphPrvKey:  p2,
		remoteEphPubKey: p1.PubKey(),
	}
	s1, err := New(c1, h1)
	tc.NoErr(t, err)
	s2, err := New(c2, h2)
	tc.NoErr(t, err)

	tc.NoErr(t, s1.write(0x00, rlp.Encode(rlp.Byte(1))))

	buf := make([]byte, 1024)
	_, err = c2.Read(buf)
	tc.NoErr(t, err)
	_, err = s2.read(buf)
	tc.NoErr(t, err)
}

func prv(t *testing.T) *secp256k1.PrivateKey {
	prv, err := secp256k1.GeneratePrivateKey()
	tc.NoErr(t, err)
	return prv
}

func testConn(t *testing.T) (net.Conn, net.Conn) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	tc.NoErr(t, err)
	var (
		c2   net.Conn
		done = make(chan struct{}, 1)
	)
	go func() {
		defer ln.Close()
		c2, err = ln.Accept()
		tc.NoErr(t, err)
		done <- struct{}{}
	}()
	c1, err := net.Dial("tcp", ln.Addr().String())
	tc.NoErr(t, err)
	<-done
	return c1, c2
}

func testNode(t *testing.T, conn net.Conn) *enr.Record {
	prv, err := secp256k1.GeneratePrivateKey()
	tc.NoErr(t, err)
	ap := netip.MustParseAddrPort(conn.LocalAddr().String())
	return &enr.Record{
		Ip:        ap.Addr().AsSlice(),
		PublicKey: prv.PubKey(),
		UdpPort:   ap.Port(),
		TcpPort:   ap.Port(),
	}
}
