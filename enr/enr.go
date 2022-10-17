package enr

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"strings"

	"github.com/indexsupply/lib/rlp"
)

// An Ethereum Node Record contains network information
// about a node on the dsicv5 p2p network. The specification
// is detailed in https://eips.ethereum.org/EIPS/eip-778.
type ENR struct {
	Signature []byte // Cryptographic signature of record contents
	Sequence  uint64 // The sequence number. Nodes should increase the number whenever the record changes and republish the record.

	ID        string // name of identity scheme, e.g. “v4”
	Secp256k1 []byte // compressed secp256k1 public key, 33 bytes

	Ip      [4]byte // IPv4 address
	TcpPort uint16  // TCP port
	UdpPort uint16  // UDP port

	Ip6      [16]byte // IPv6 address
	Tcp6Port uint16   // IPv6-specific TCP port. If omitted, same as TcpPort.
	Udp6Port uint16   // IPv6-specific UDP port. If omitted, same as UdpPort.
}

const enrTextPrefix = "enr:"

// Given a text encoding of an Ethereum Node Record, which is formatted
// as the base-64 encoding of its RLP content prefixed by enr:, this
// method decodes it into an ENR struct.
func UnmarshalFromText(str string) (*ENR, error) {
	if !strings.HasPrefix(str, enrTextPrefix) {
		return nil, errors.New("Invalid prefix for ENR text encoding.")
	}

	str = str[len(enrTextPrefix):]
	b, err := base64.RawURLEncoding.DecodeString(str)
	if err != nil {
		return nil, err
	}

	item, err := rlp.Decode(b)
	if err != nil {
		return nil, err
	}

	sig, seq, m, err := parseRLPEncoding(item)
	if err != nil {
		return nil, err
	}

	var (
		ip4 [4]byte
		ip6 [16]byte
	)
	copy(ip4[:], m["ip"])
	copy(ip6[:], m["ip6"])

	return &ENR{
		Signature: sig,
		Sequence:  seq,
		ID:        string(m["id"]),
		Secp256k1: m["secp256k1"][:33],
		Ip:        ip4,
		Ip6:       ip6,
		UdpPort:   binary.BigEndian.Uint16(leftPad(m["udp"], 2)),
		TcpPort:   binary.BigEndian.Uint16(leftPad(m["tcp"], 2)),
		Udp6Port:  binary.BigEndian.Uint16(leftPad(m["udp6"], 2)),
		Tcp6Port:  binary.BigEndian.Uint16(leftPad(m["tcp6"], 2)),
	}, nil
}

var (
	ErrMissingENRKey    = errors.New("ENR is missing required key ID")
	ErrMissingSignature = errors.New("ENR is missing a signature")
	ErrMissingSequence  = errors.New("ENR is missing a sequence number")
)

// parseRLPEncoding returns a triple representing the signature,
// sequence number, and a map of key-value pairs for the Node Record.
// i must be the "root" RLP list that is in [signature, seq, k, v, ...] format
// as specified in https://eips.ethereum.org/EIPS/eip-778.
func parseRLPEncoding(i *rlp.Item) ([]byte, uint64, map[string][]byte, error) {
	var (
		sig []byte
		seq uint64
		m   map[string][]byte
	)
	switch listSize := len(i.L); {
	case listSize < 1:
		return sig, seq, m, ErrMissingSignature
	case listSize < 2:
		return sig, seq, m, ErrMissingSequence
	case listSize < 4:
		return sig, seq, m, ErrMissingENRKey
	}
	sig = i.L[0].D
	// seq is expected to be a uint64 so we left pad to 8 bytes
	s := i.L[1].D
	seq = binary.BigEndian.Uint64(leftPad(s, 8))

	m = make(map[string][]byte)
	for idx := 2; idx < len(i.L); {
		key := string(i.L[idx].D)
		idx++
		value := i.L[idx].D
		idx++
		m[key] = value
	}

	// "id" must be present in the key-value map
	if _, ok := m["id"]; !ok {
		return sig, seq, m, ErrMissingENRKey
	}
	if secp256k1Value, ok := m["secp256k1"]; ok && len(secp256k1Value) != 33 {
		return sig, seq, m, errors.New("If secp256k1 is present, it must be exactly 33 bytes long.")
	}

	return sig, seq, m, nil
}

// left pads the provided byte array to the wantedLength, in bytes, using 0s.
// does nothing if b is already at the wanted length.
func leftPad(b []byte, wantedLength int) []byte {
	if len(b) >= wantedLength {
		return b
	}
	padded := make([]byte, wantedLength)
	bytesNeeded := wantedLength - len(b)
	copy(padded[bytesNeeded:], b)

	return padded
}
