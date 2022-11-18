package enr

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/indexsupply/x/isxhash"
	"github.com/indexsupply/x/isxsecp256k1"
	"github.com/indexsupply/x/rlp"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
)

// An Ethereum Node Record contains network information
// about a node on the dsicv5 p2p network. The specification
// is detailed in https://eips.ethereum.org/EIPS/eip-778.
type Record struct {
	Signature []byte // Cryptographic signature of record contents
	Sequence  uint64 // The sequence number. Nodes should increase the number whenever the record changes and republish the record.
	IDScheme  string // name of identity scheme, e.g. “v4”

	PrivateKey *secp256k1.PrivateKey
	PublicKey  *secp256k1.PublicKey

	Ip      net.IP
	TcpPort uint16
	UdpPort uint16

	Ip6      net.IP
	Tcp6Port uint16 // IPv6-specific TCP port. If omitted, same as TcpPort.
	Udp6Port uint16 // IPv6-specific UDP port. If omitted, same as UdpPort.

	SentPing     time.Time
	SentPingHash [32]byte
	ReceivedPong time.Time
	ReceivedPing time.Time
}

func (r *Record) String() string {
	id := r.ID()
	return fmt.Sprintf("%s:%d %x", r.Ip.String(), r.UdpPort, id[:4])
}

func (r *Record) ID() [32]byte {
	pkb := isxsecp256k1.Encode(r.PublicKey)
	return isxhash.Keccak32(pkb[:])
}

func (r Record) UDPAddr() *net.UDPAddr {
	return &net.UDPAddr{
		IP:   r.Ip,
		Port: int(r.UdpPort),
	}
}

func (r Record) TCPAddr() *net.TCPAddr {
	return &net.TCPAddr{
		IP:   r.Ip,
		Port: int(r.TcpPort),
	}
}

func ParseV4(s string) (*Record, error) {
	nurl, err := url.Parse(s)
	if err != nil {
		return nil, err
	}
	if nurl.Scheme != "enode" {
		return nil, errors.New("missing enode scheme")
	}
	if nurl.User == nil {
		return nil, errors.New("missing node-id in user field")
	}
	nidb, err := hex.DecodeString(nurl.User.String())
	if err != nil {
		return nil, err
	}
	npubk, err := isxsecp256k1.Decode(*(*[64]byte)(nidb))
	if err != nil {
		return nil, err
	}
	nip := net.ParseIP(nurl.Hostname())
	if nip == nil {
		return nil, errors.New("unable to find ip in hostname")
	}
	nport, err := strconv.ParseUint(nurl.Port(), 10, 16)
	if err != nil {
		return nil, err
	}
	return &Record{
		PublicKey: npubk,
		Ip:        nip,
		TcpPort:   uint16(nport),
		UdpPort:   uint16(nport),
	}, nil
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
			rec.IDScheme, err = item.At(i + 1).String()
			if err != nil {
				return rec, err
			}
			if len(rec.IDScheme) == 0 {
				return rec, errors.New("missing id scheme eg v4")
			}
		case "secp256k1":
			rec.PublicKey, err = item.At(i + 1).Secp256k1PublicKey()
			if err != nil {
				return rec, err
			}
		case "ip":
			rec.Ip, err = item.At(i + 1).IP()
			if err != nil {
				return rec, err
			}
		case "ip6":
			rec.Ip6, err = item.At(i + 1).IP()
			if err != nil {
				return rec, err
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

func (r Record) MarshalText(prv *secp256k1.PrivateKey) ([]byte, error) {
	b, err := r.MarshalRLP(prv)
	if err != nil {
		return nil, err
	}
	var buf = new(bytes.Buffer)
	encoder := base64.NewEncoder(base64.RawURLEncoding, buf)
	_, err = encoder.Write(b)
	if err != nil {
		return nil, err
	}
	err = encoder.Close()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (r *Record) MarshalRLP(prv *secp256k1.PrivateKey) ([]byte, error) {
	// From the devp2p docs:
	//
	// The key/value pairs must be sorted by key and must be unique,
	// i.e. any key may be present only once. The keys can technically
	// be any byte sequence, but ASCII text is preferred. Key names in
	// the table below have pre-defined meaning.
	var items []rlp.Item
	items = append(items, rlp.Uint64(r.Sequence))
	items = append(items, rlp.String("id"))
	items = append(items, rlp.String(r.IDScheme))
	items = append(items, rlp.String("ip"))
	items = append(items, rlp.Bytes(r.Ip))
	if len(r.Ip6) != 0 {
		items = append(items, rlp.String("ip6"))
		items = append(items, rlp.Bytes(r.Ip6))
	}
	items = append(items, rlp.String("secp256k1"))
	items = append(items, rlp.Bytes(r.PublicKey.SerializeCompressed()))
	if r.TcpPort != 0 {
		items = append(items, rlp.String("tcp"))
		items = append(items, rlp.Uint16(r.TcpPort))
	}
	if r.Tcp6Port != 0 {
		items = append(items, rlp.String("tcp6"))
		items = append(items, rlp.Uint16(r.Tcp6Port))
	}
	items = append(items, rlp.String("udp"))
	items = append(items, rlp.Uint16(r.UdpPort))
	if r.Udp6Port != 0 {
		items = append(items, rlp.String("udp6"))
		items = append(items, rlp.Uint16(r.Udp6Port))
	}
	// From the devp2p docs:
	//
	// To sign record content with this scheme, apply the keccak256
	// hash function (as used by the EVM) to content, then create a
	// signature of the hash. The resulting 64-byte signature is
	// encoded as the concatenation of the r and s signature values
	// (the recovery ID v is omitted).
	hash := isxhash.Keccak32(rlp.Encode(rlp.List(items...)))
	sig, err := isxsecp256k1.Sign(prv, hash)
	if err != nil {
		return nil, err
	}
	sigNoFmt := sig[:len(sig)-1] // remove formatting byte
	items = append([]rlp.Item{rlp.Bytes(sigNoFmt)}, items...)
	return rlp.Encode(rlp.List(items...)), nil
}
