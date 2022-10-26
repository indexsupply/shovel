package enr

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/netip"
	"strings"

	"github.com/indexsupply/x/rlp"
	"golang.org/x/crypto/sha3"
)

// An Ethereum Node Record contains network information
// about a node on the dsicv5 p2p network. The specification
// is detailed in https://eips.ethereum.org/EIPS/eip-778.
type Record struct {
	Signature []byte // Cryptographic signature of record contents
	Sequence  uint64 // The sequence number. Nodes should increase the number whenever the record changes and republish the record.

	ID        string   // name of identity scheme, e.g. “v4”
	Secp256k1 [33]byte // compressed secp256k1 public key, 33 bytes

	Ip      netip.Addr
	TcpPort uint16
	UdpPort uint16

	Ip6      netip.Addr
	Tcp6Port uint16 // IPv6-specific TCP port. If omitted, same as TcpPort.
	Udp6Port uint16 // IPv6-specific UDP port. If omitted, same as UdpPort.
}

// Returns the address of the node, which is the keccak256 hash of its Secp256k1 key.
func (enr Record) NodeAddr() []byte {
	return keccak(enr.Secp256k1[:])
}

// Returns the address of the node as a hex encoded string.
func (enr Record) NodeAddrHex() string {
	return fmt.Sprintf("%x", enr.NodeAddr())
}

func keccak(d []byte) []byte {
	h := sha3.NewLegacyKeccak256()
	h.Write(d)
	return h.Sum(nil)
}

// Given a text encoding of an Ethereum Node Record, which is formatted
// as the base-64 encoding of its RLP content prefixed by enr:, this
// method decodes it into an Record struct.
func UnmarshalText(str string) (Record, error) {
	const enrTextPrefix = "enr:"
	if !strings.HasPrefix(str, enrTextPrefix) {
		return Record{}, errors.New("Invalid prefix for ENR text encoding.")
	}

	str = str[len(enrTextPrefix):]
	b, err := base64.RawURLEncoding.DecodeString(str)
	if err != nil {
		return Record{}, err
	}

	item, err := rlp.Decode(b)
	if err != nil {
		return Record{}, err
	}

	return decode(item)
}

func decode(item rlp.Item) (Record, error) {
	var (
		rec = Record{}
		err error
	)
	rec.Signature, err = item.At(0).Bytes()
	if err != nil {
		return Record{}, errors.New("missing signature")
	}
	rec.Sequence, err = item.At(1).Uint64()
	if err != nil {
		return Record{}, errors.New("missing sequence")
	}

	for i := 2; i < len(item.List()); i += 2 {
		k, _ := item.At(i).String()
		switch k {
		case "id":
			rec.ID, err = item.At(i + 1).String()
			if err != nil {
				return rec, err
			}
			if len(rec.ID) == 0 {
				return rec, errors.New("missing id")
			}
		case "secp256k1":
			rec.Secp256k1, err = item.At(i + 1).Bytes33()
			if err != nil {
				return rec, err
			}
		case "ip":
			rec.Ip, err = item.At(i + 1).NetIPAddr()
			if err != nil {
				return rec, err
			}
			if !rec.Ip.Is4() {
				return rec, errors.New("ip must be v4 ip")
			}
		case "ip6":
			rec.Ip6, err = item.At(i + 1).NetIPAddr()
			if err != nil {
				return rec, err
			}
			if !rec.Ip.Is6() {
				return rec, errors.New("ip6 must be v6 ip")
			}
		case "tcp":
			rec.TcpPort, err = item.At(i + 1).Uint16()
			if err != nil {
				return rec, err
			}
		case "udp":
			rec.UdpPort, err = item.At(i + 1).Uint16()
			if err != nil {
				return rec, err
			}
		case "tcp6":
			rec.Tcp6Port, err = item.At(i + 1).Uint16()
			if err != nil {
				return rec, err
			}
		case "udp6":
			rec.Udp6Port, err = item.At(i + 1).Uint16()
			if err != nil {
				return rec, err
			}
		}
	}

	return rec, nil
}
