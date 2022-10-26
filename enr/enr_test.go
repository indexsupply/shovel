package enr

import (
	"encoding/hex"
	"net/netip"
	"reflect"
	"testing"
)

func TestUnmarshalFromText(t *testing.T) {
	// ENR Sample is from https://eips.ethereum.org/EIPS/eip-778
	const enrSample = "enr:-IS4QHCYrYZbAKWCBRlAy5zzaDZXJBGkcnh4MHcBFZntXNFrdvJjX04jRzjzCBOonrkTfj499SZuOh8R33Ls8RRcy5wBgmlkgnY0gmlwhH8AAAGJc2VjcDI1NmsxoQPKY0yuDUmstAHYpMa2_oxVtw0RW_QAdpzBQA8yWM0xOIN1ZHCCdl8"

	got, err := UnmarshalText(enrSample)
	if err != nil {
		t.Fatal(err)
	}

	want := Record{}
	want.Signature, _ = hex.DecodeString("7098ad865b00a582051940cb9cf36836572411a47278783077011599ed5cd16b76f2635f4e234738f30813a89eb9137e3e3df5266e3a1f11df72ecf1145ccb9c")
	want.Sequence = uint64(1)
	want.ID = "v4"

	var spk [33]byte
	secp256k1, _ := hex.DecodeString("03ca634cae0d49acb401d8a4c6b6fe8c55b70d115bf400769cc1400f3258cd3138")
	copy(spk[:], secp256k1)
	want.Secp256k1 = spk

	want.Ip = netip.AddrFrom4([4]byte{0x7f, 0x00, 0x00, 0x01})
	want.UdpPort = uint16(30303)

	if !reflect.DeepEqual(want, got) {
		t.Errorf("\nwant:\n%v\ngot:\n%v\n", want, got)
	}
}
