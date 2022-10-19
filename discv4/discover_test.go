package discv4

import (
	"net/netip"
	"testing"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/indexsupply/x/enr"
	"github.com/indexsupply/x/tc"
	"golang.org/x/net/nettest"
)

func TestPing(t *testing.T) {
	p1 := testProcess(t)
	p2 := testProcess(t)

	tc.NoErr(t, p1.Ping(p2.self))
	tc.NoErr(t, p2.read())
	tc.NoErr(t, p1.read())
	tc.NoErr(t, p2.Ping(p1.self))
	tc.NoErr(t, p1.read())

	if _, ok := p1.peers[p2.self.ID()]; !ok {
		t.Fatal("expected p2 to be in p1's peer map")
	}
	if p1.peers[p2.self.ID()].SentPing.IsZero() {
		t.Errorf("expected ping to be sent")
	}
	if p1.peers[p2.self.ID()].ReceivedPong.IsZero() {
		t.Errorf("expected p1 to have received a pong")
	}
	if p2.peers[p1.self.ID()].ReceivedPing.IsZero() {
		t.Errorf("expected p2 to have received a ping")
	}
	if p2.peers[p1.self.ID()].SentPing.IsZero() {
		t.Errorf("expected p2 to have sent a ping")
	}
}

func TestFindNode(t *testing.T) {
	p1 := testProcess(t)
	p2 := testProcess(t)
	p3 := testProcess(t)

	tc.NoErr(t, p1.Ping(p2.self))
	tc.NoErr(t, p2.read()) //read ping
	tc.NoErr(t, p1.read()) //read pong
	tc.NoErr(t, p1.read()) //read ping
	tc.NoErr(t, p2.read()) //read pong

	tc.NoErr(t, p3.Ping(p2.self))
	tc.NoErr(t, p2.read()) //read ping
	tc.NoErr(t, p3.read()) //read pong
	tc.NoErr(t, p3.read()) //read ping
	tc.NoErr(t, p2.read()) //read pong

	if len(p3.peers) != 1 {
		t.Error("expected p3 should only know about p2")
	}

	tc.NoErr(t, p3.FindNode(p3.self.PublicKey, p2.self))
	tc.NoErr(t, p2.read()) //read find
	tc.NoErr(t, p3.read()) //read neighbors

	if len(p3.peers) != 2 {
		t.Errorf("expected p3 to find p1 %d", len(p3.peers))
	}
}

func testProcess(t *testing.T) *process {
	prv, err := secp256k1.GeneratePrivateKey()
	tc.NoErr(t, err)
	c, err := nettest.NewLocalPacketListener("udp4")
	tc.NoErr(t, err)
	ap := netip.MustParseAddrPort(c.LocalAddr().String())
	return New(c, prv, &enr.Record{
		Ip:        ap.Addr().AsSlice(),
		PublicKey: prv.PubKey(),
		UdpPort:   ap.Port(),
		TcpPort:   ap.Port(),
	})
}
