package rlpx

import (
	"net/netip"
	"testing"

	"github.com/indexsupply/x/enr"
	"github.com/indexsupply/x/tc"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
)

func TestHandShake(t *testing.T) {
	k1 := prv(t)
	k2 := prv(t)
	h1 := Initiator(k1, k2.PubKey())
	h2 := Recipient(k2)

	auth, err := h1.Auth()
	tc.NoErr(t, err)
	tc.NoErr(t, h2.HandleAuth(auth))
	ack, err := h2.Ack()
	tc.NoErr(t, err)
	tc.NoErr(t, h1.HandleAck(ack))

	if !h1.remotePubKey.IsEqual(h2.localPrvKey.PubKey()) {
		t.Errorf(
			"pub keys not equal h1: %x h2: %x",
			h1.remotePubKey.SerializeCompressed(),
			h2.localPrvKey.PubKey().SerializeCompressed(),
		)
	}
	if !h1.remoteEphPubKey.IsEqual(h2.localEphPrvKey.PubKey()) {
		t.Errorf(
			"eph pub keys not equal h1: %x h2: %x",
			h1.remoteEphPubKey.SerializeCompressed(),
			h2.localEphPrvKey.PubKey().SerializeCompressed(),
		)
	}
	if h1.initNonce != h2.initNonce {
		t.Errorf(
			"expected init nonces to be equal h1: %x h2: %x",
			h1.initNonce,
			h2.initNonce,
		)
	}
	if h1.recipientNonce != h2.recipientNonce {
		t.Errorf(
			"expected recipient nonces to be equal h1: %x h2: %x",
			h1.recipientNonce,
			h2.recipientNonce,
		)
	}
}

func TestSession(t *testing.T) {
	n1 := testNode(t)
	n2 := testNode(t)

	k1 := n1.PrivateKey
	k2 := n2.PrivateKey

	h1 := Initiator(k1, k2.PubKey())
	h2 := Recipient(k2)

	auth, err := h1.Auth()
	tc.NoErr(t, err)
	tc.NoErr(t, h2.HandleAuth(auth))
	ack, err := h2.Ack()
	tc.NoErr(t, err)
	tc.NoErr(t, h1.HandleAck(ack))

	s1, err := Session(n1, h1)
	tc.NoErr(t, err)
	s2, err := Session(n2, h2)
	tc.NoErr(t, err)

	m1, _ := s1.Hello()
	tc.NoErr(t, s2.HandleMessage(m1))

	m2, _ := s1.EthStatus()
	tc.NoErr(t, s2.HandleMessage(m2))
}

func prv(t *testing.T) *secp256k1.PrivateKey {
	prv, err := secp256k1.GeneratePrivateKey()
	tc.NoErr(t, err)
	return prv
}

func testNode(t *testing.T) *enr.Record {
	prv, err := secp256k1.GeneratePrivateKey()
	tc.NoErr(t, err)
	ap := netip.MustParseAddrPort("127.0.0.1:30303")
	return &enr.Record{
		Ip:         ap.Addr().AsSlice(),
		PrivateKey: prv,
		PublicKey:  prv.PubKey(),
		UdpPort:    ap.Port(),
		TcpPort:    ap.Port(),
	}
}
