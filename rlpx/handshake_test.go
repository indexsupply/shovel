package rlpx

import (
	"bytes"
	"encoding/hex"
	"net"
	"testing"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"

	"github.com/indexsupply/x/isxsecp256k1"
	"github.com/indexsupply/x/tc"
)

func TestHandshake(t *testing.T) {
	tv := map[string]string{
		"initiator_private_key":           "49a7b37aa6f6645917e7b807e9d1c00d4fa71f18343b0d4122a4d2df64dd6fee",
		"receiver_private_key":            "b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291",
		"initiator_ephemeral_private_key": "869d6ecf5211f1cc60418a13b9d870b22959d0c16f02bec714c960dd2298a32d",
		"receiver_ephemeral_private_key":  "e238eb8e04fee6511ab04c6dd3c89ce097b11f25d584863ac2b6d5b35b1847e4",
		"initiator_nonce":                 "7e968bba13b6c50e2c4cd7f241cc0d64d1ac25c7f5952df231ac6a2bda8ee5d6",
		"receiver_nonce":                  "b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291",
	}
	ih, rh := setupHandshakes(t, tv)
	initConn, recConn := net.Pipe()
	ch := make(chan struct{})

	// launch the receiver
	go func(t *testing.T) {
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
	}(t)

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
	initEphPrvKey := secp256k1.PrivKeyFromBytes(decodeHexString(t, tv["initiator_ephemeral_private_key"]))
	initNonce := decodeHexString(t, tv["initiator_nonce"])
	if len(initNonce) != 32 {
		t.Errorf("initiator nonce is not 32 bytes")
	}
	inonce := make([]byte, 32)
	copy(inonce[:], initNonce)
	recPrvKey := secp256k1.PrivKeyFromBytes(decodeHexString(t, tv["receiver_private_key"]))
	recEphPrvKey := secp256k1.PrivKeyFromBytes(decodeHexString(t, tv["receiver_ephemeral_private_key"]))
	recNonce := decodeHexString(t, tv["receiver_nonce"])
	if len(recNonce) != 32 {
		t.Errorf("receiver nonce is not 32 bytes")
	}
	rnonce := make([]byte, 32)
	copy(rnonce[:], recNonce)
	ih = &handshake{
		isInitiator:    true,
		remotePubKey:   recPrvKey.PubKey(),
		localPrvKey:    initPrvKey,
		localEphPrvKey: initEphPrvKey,
		initNonce:      inonce,
	}
	rh = &handshake{
		remotePubKey:   initPrvKey.PubKey(),
		localPrvKey:    recPrvKey,
		localEphPrvKey: recEphPrvKey,
		receiverNonce:  rnonce,
	}
	return
}
