package rlpx

import (
	"bytes"
	"encoding/hex"
	"net"
	"testing"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"

	"github.com/indexsupply/x/enr"
	"github.com/indexsupply/x/isxsecp256k1"
	"github.com/indexsupply/x/tc"
)

func TestHandshake(t *testing.T) {
	tv := map[string]string{
		"initiator_private_key": "49a7b37aa6f6645917e7b807e9d1c00d4fa71f18343b0d4122a4d2df64dd6fee",
		"receiver_private_key":  "b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291",
	}
	ih, rh := setupHandshakes(t, tv)
	initConn, recConn := net.Pipe()
	ch := make(chan struct{})

	// launch the receiver
	go func() {
		sealedAuth := make([]byte, 1024)
		br, err := recConn.Read(sealedAuth)
		tc.NoErr(t, err)
		err = rh.handleAuthMsg(sealedAuth[:br], recConn)
		tc.NoErr(t, err)
		if rh.isInitiator {
			t.Errorf("isInitiator should be false, got true")
		}
		// ensure that the nonce, remote pub key and remote ephemeral pub key are set
		if isxsecp256k1.Encode(rh.remotePubKey) != isxsecp256k1.Encode(ih.localPrvKey.PubKey()) {
			t.Errorf("initiator pub keys do not match: receiver got %x. expected %x", isxsecp256k1.Encode(rh.remotePubKey), isxsecp256k1.Encode(ih.localPrvKey.PubKey()))
		}
		if isxsecp256k1.Encode(rh.remoteEphPubKey) != isxsecp256k1.Encode(ih.localEphPrvKey.PubKey()) {
			t.Errorf("initiator ephemeral pub keys do not match: receiver got %x. expected %x", isxsecp256k1.Encode(rh.remoteEphPubKey), isxsecp256k1.Encode(ih.localEphPrvKey.PubKey()))
		}
		if !bytes.Equal(rh.initNonce, ih.initNonce) {
			t.Errorf("initiator nonces do not match: receiver got %d and initiator got %d", rh.initNonce, ih.initNonce)
		}
		close(ch)
	}()

	err := ih.sendAuth(initConn)
	tc.NoErr(t, err)
	sealedAck := make([]byte, 1024)
	_, err = initConn.Read(sealedAck)
	tc.NoErr(t, err)
	err = ih.handleAckMsg(sealedAck)
	tc.NoErr(t, err)
	if !bytes.Equal(ih.receiverNonce, rh.receiverNonce) {
		t.Errorf("receiver nonces do not match: receiver got %d and initiator got %d", rh.receiverNonce, ih.receiverNonce)
	}
	if isxsecp256k1.Encode(ih.remoteEphPubKey) != isxsecp256k1.Encode(rh.localEphPrvKey.PubKey()) {
		t.Errorf("receiver ephemeral pub keys do not match: initiator got %x. expected %x", isxsecp256k1.Encode(ih.remoteEphPubKey), isxsecp256k1.Encode(rh.localEphPrvKey.PubKey()))
	}
	<-ch
}

func decodeHexString(t *testing.T, s string) []byte {
	b, err := hex.DecodeString(s)
	tc.NoErr(t, err)
	return b
}

func setupHandshakes(t *testing.T, tv map[string]string) (ih, rh *handshake) {
	initPrvKey := secp256k1.PrivKeyFromBytes(decodeHexString(t, tv["initiator_private_key"]))
	recPrvKey := secp256k1.PrivKeyFromBytes(decodeHexString(t, tv["receiver_private_key"]))
	ih = newHandshake(initPrvKey, &enr.Record{PublicKey: recPrvKey.PubKey()})
	rh = newHandshake(recPrvKey, &enr.Record{PublicKey: initPrvKey.PubKey()})
	return
}
