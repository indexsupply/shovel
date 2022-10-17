package enr

import (
	"bytes"
	"fmt"
	"testing"
)

func TestFromTextEncoding(t *testing.T) {
	// ENR Sample is from https://eips.ethereum.org/EIPS/eip-778
	enrSample := "enr:-IS4QHCYrYZbAKWCBRlAy5zzaDZXJBGkcnh4MHcBFZntXNFrdvJjX04jRzjzCBOonrkTfj499SZuOh8R33Ls8RRcy5wBgmlkgnY0gmlwhH8AAAGJc2VjcDI1NmsxoQPKY0yuDUmstAHYpMa2_oxVtw0RW_QAdpzBQA8yWM0xOIN1ZHCCdl8"

	enr, err := FromTextEncoding(enrSample)
	if err != nil {
		t.Fatal(err)
	}

	var (
		expectedSignatureHex = "7098ad865b00a582051940cb9cf36836572411a47278783077011599ed5cd16b76f2635f4e234738f30813a89eb9137e3e3df5266e3a1f11df72ecf1145ccb9c"
		expectedSequence     = uint64(1)
		expectedId           = "v4"
		expectedSecp256k1Hex = "03ca634cae0d49acb401d8a4c6b6fe8c55b70d115bf400769cc1400f3258cd3138"
		expectedIp           = []byte{0x7f, 0x00, 0x00, 0x01}
		expectedIp6          = make([]byte, 16)
		expectedUdpPort      = uint16(30303)
		expectedTcpPort      = uint16(0)
		expectedUdp6Port     = uint16(0)
		expectedTcp6Port     = uint16(0)
	)
	if gotSignatureHex := fmt.Sprintf("%x", enr.Signature); gotSignatureHex != expectedSignatureHex {
		t.Errorf("wrong signature: expected %s, got %s", expectedSignatureHex, gotSignatureHex)
	}
	if gotSecp256k1Hex := fmt.Sprintf("%x", enr.Secp256k1); gotSecp256k1Hex != expectedSecp256k1Hex {
		t.Errorf("wrong signature: expected %s, got %s", expectedSecp256k1Hex, gotSecp256k1Hex)
	}
	if gotSequence := enr.Sequence; gotSequence != expectedSequence {
		t.Errorf("wrong seq: expected %d, got %d", expectedSequence, gotSequence)
	}
	if gotId := enr.ID; gotId != expectedId {
		t.Errorf("wrong id: expected %s, got %s", expectedId, gotId)
	}
	if gotIp := enr.Ip; !bytes.Equal(gotIp[:], expectedIp) {
		t.Errorf("wrong ip: expected %v, got %v", expectedIp, gotIp)
	}
	if gotIp6 := enr.Ip6; !bytes.Equal(gotIp6[:], expectedIp6) {
		t.Errorf("wrong ip6: expected %v, got %v", expectedIp6, gotIp6)
	}
	if gotUdpPort := enr.UdpPort; gotUdpPort != expectedUdpPort {
		t.Errorf("wrong udp port: expected %d, got %d", expectedUdpPort, gotUdpPort)
	}
	if gotTcpPort := enr.TcpPort; gotTcpPort != expectedTcpPort {
		t.Errorf("wrong tcp port: expected %d, got %d", expectedTcpPort, gotTcpPort)
	}
	if gotUdp6Port := enr.Udp6Port; gotUdp6Port != expectedUdp6Port {
		t.Errorf("wrong udp6 port: expected %d, got %d", expectedUdp6Port, gotUdp6Port)
	}
	if gotTcp6Port := enr.Tcp6Port; gotTcp6Port != expectedTcp6Port {
		t.Errorf("wrong tcp6 port: expected %d, got %d", expectedTcp6Port, gotTcp6Port)
	}
}
