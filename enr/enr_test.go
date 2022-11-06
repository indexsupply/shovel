package enr

import (
	"encoding/hex"
	"reflect"
	"testing"

	"github.com/indexsupply/x/isxsecp256k1"
	"github.com/indexsupply/x/tc"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
)

func TestUnmarshalText(t *testing.T) {
	// ENR Sample is from https://eips.ethereum.org/EIPS/eip-778
	const enrSample = "enr:-IS4QHCYrYZbAKWCBRlAy5zzaDZXJBGkcnh4MHcBFZntXNFrdvJjX04jRzjzCBOonrkTfj499SZuOh8R33Ls8RRcy5wBgmlkgnY0gmlwhH8AAAGJc2VjcDI1NmsxoQPKY0yuDUmstAHYpMa2_oxVtw0RW_QAdpzBQA8yWM0xOIN1ZHCCdl8"

	got, err := UnmarshalText(enrSample)
	if err != nil {
		t.Fatal(err)
	}

	want := Record{}
	want.Signature, _ = hex.DecodeString("7098ad865b00a582051940cb9cf36836572411a47278783077011599ed5cd16b76f2635f4e234738f30813a89eb9137e3e3df5266e3a1f11df72ecf1145ccb9c")
	want.Sequence = uint64(1)
	want.IDScheme = "v4"

	var spk [33]byte
	secp256k1, _ := hex.DecodeString("03ca634cae0d49acb401d8a4c6b6fe8c55b70d115bf400769cc1400f3258cd3138")
	copy(spk[:], secp256k1)
	want.PublicKey, _ = isxsecp256k1.DecodeCompressed(spk)

	want.Ip = []byte{0x7f, 0x00, 0x00, 0x01}
	want.UdpPort = uint16(30303)

	if !reflect.DeepEqual(want, got) {
		t.Errorf("\nwant:\n%v\ngot:\n%v\n", want, got)
	}
}

func TestMarshalText(t *testing.T) {
	const tv = "-IS4QHCYrYZbAKWCBRlAy5zzaDZXJBGkcnh4MHcBFZntXNFrdvJjX04jRzjzCBOonrkTfj499SZuOh8R33Ls8RRcy5wBgmlkgnY0gmlwhH8AAAGJc2VjcDI1NmsxoQPKY0yuDUmstAHYpMa2_oxVtw0RW_QAdpzBQA8yWM0xOIN1ZHCCdl8"
	kb, _ := hex.DecodeString("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
	prvk := secp256k1.PrivKeyFromBytes(kb)

	r := &Record{
		PublicKey: prvk.PubKey(),
		Sequence:  uint64(1),
		IDScheme:  "v4",
		Ip:        []byte{0x7f, 0x00, 0x00, 0x01},
		UdpPort:   uint16(30303),
	}
	u, err := r.MarshalText(prvk)
	tc.NoErr(t, err)
	if string(u) != tv {
		t.Error("expected marshalled text to match test vector")
	}
}
