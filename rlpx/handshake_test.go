package rlpx

import (
	"encoding/hex"
	"fmt"
	"net"
	"testing"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"

	_ "github.com/indexsupply/x/ecies"
	"github.com/indexsupply/x/tc"
)

func TestSendAuth(t *testing.T) {
	tv := map[string]string{
		"initiator_private_key":           "49a7b37aa6f6645917e7b807e9d1c00d4fa71f18343b0d4122a4d2df64dd6fee",
		"receiver_private_key":            "b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291",
		"initiator_ephemeral_private_key": "869d6ecf5211f1cc60418a13b9d870b22959d0c16f02bec714c960dd2298a32d",
		"receiver_ephemeral_private_key":  "e238eb8e04fee6511ab04c6dd3c89ce097b11f25d584863ac2b6d5b35b1847e4",
		"initiator_nonce":                 "7e968bba13b6c50e2c4cd7f241cc0d64d1ac25c7f5952df231ac6a2bda8ee5d6",
		"receiver_nonce":                  "b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291",
	}
	initPrvKey := secp256k1.PrivKeyFromBytes(decodeHexString(t, tv["initiator_private_key"]))
	initEphPrvKey := secp256k1.PrivKeyFromBytes(decodeHexString(t, tv["initiator_ephemeral_private_key"]))
	initNonce := decodeHexString(t, tv["initiator_nonce"])
	if len(initNonce) != 32 {
		t.Errorf("initNonce is not 32 bytes")
	}
	nonce := make([]byte, 32)
	copy(nonce[:], initNonce)
	recPrvKey := secp256k1.PrivKeyFromBytes(decodeHexString(t, tv["receiver_private_key"]))
	recEphPrvKey := secp256k1.PrivKeyFromBytes(decodeHexString(t, tv["receiver_ephemeral_private_key"]))
	h := &handshake{
		isInitiator:     true,
		remotePubKey:    recPrvKey.PubKey(),
		localPrvKey:     initPrvKey,
		localEphPrvKey:  initEphPrvKey,
		remoteEphPubKey: recEphPrvKey.PubKey(),
		initNonce:       nonce,
	}
	initConn, respConn := net.Pipe()
	go func() {
		err := h.sendAuth(initConn)
		tc.NoErr(t, err)
	}()
	b := make([]byte, 1024)
	respConn.Read(b)
	// this test is WIP. Will complete it after we have implemented
	// logic to respond to an auth message with an ack.
}

func decodeHexString(t *testing.T, s string) []byte {
	b, err := hex.DecodeString(s)
	tc.NoErr(t, err)
	return b
}
