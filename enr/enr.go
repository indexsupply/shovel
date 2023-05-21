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

	"github.com/indexsupply/x/bint"
	"github.com/indexsupply/x/isxhash"
	"github.com/indexsupply/x/rlp"
	"github.com/indexsupply/x/wsecp256k1"

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
	return [32]byte(isxhash.Keccak(wsecp256k1.Encode(r.PublicKey)))
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
	npubk, err := wsecp256k1.Decode(nidb)
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
func (r *Record) UnmarshalText(str string) error {
	const enrTextPrefix = "enr:"
	if !strings.HasPrefix(str, enrTextPrefix) {
		return errors.New("Invalid prefix for ENR text encoding.")
	}

	str = str[len(enrTextPrefix):]
	b, err := base64.RawURLEncoding.DecodeString(str)
	if err != nil {
		return err
	}

	return r.UnmarshalRLP(b)
}

func (r *Record) UnmarshalRLP(b []byte) error {
	var (
		itr = rlp.Iter(b)
		err error
	)
	r.Signature = itr.Bytes()
	r.Sequence = bint.Uint64(itr.Bytes())
	for {
		k, v := itr.Bytes(), itr.Bytes()
		if k == nil || v == nil {
			break
		}
		switch string(k) {
		case "id":
			r.IDScheme = string(v)
			if len(r.IDScheme) == 0 {
				return errors.New("missing id scheme eg v4")
			}
		case "secp256k1":
			r.PublicKey, err = wsecp256k1.DecodeCompressed(v)
			if err != nil {
				return err
			}
		case "ip":
			r.Ip = net.IP(v)
		case "ip6":
			r.Ip6 = net.IP(v)
		case "tcp":
			r.TcpPort = bint.Uint16(v)
		case "udp":
			r.UdpPort = bint.Uint16(v)
		case "tcp6":
			r.Tcp6Port = bint.Uint16(v)
		case "udp6":
			r.Udp6Port = bint.Uint16(v)
		}
	}
	return nil
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
	var buf [][]byte
	buf = append(buf, bint.Encode(nil, r.Sequence))
	buf = append(buf, []byte("id"))
	buf = append(buf, []byte(r.IDScheme))
	buf = append(buf, []byte("ip"))
	buf = append(buf, r.Ip)
	if len(r.Ip6) != 0 {
		buf = append(buf, []byte("ip6"))
		buf = append(buf, r.Ip6)
	}
	buf = append(buf, []byte("secp256k1"))
	buf = append(buf, r.PublicKey.SerializeCompressed())
	if r.TcpPort != 0 {
		buf = append(buf, []byte("tcp"))
		buf = append(buf, bint.Encode(nil, uint64(r.TcpPort)))
	}
	if r.Tcp6Port != 0 {
		buf = append(buf, []byte("tcp6"))
		buf = append(buf, bint.Encode(nil, uint64(r.Tcp6Port)))
	}
	buf = append(buf, []byte("udp"))
	buf = append(buf, bint.Encode(nil, uint64(r.UdpPort)))
	if r.Udp6Port != 0 {
		buf = append(buf, []byte("udp6"))
		buf = append(buf, bint.Encode(nil, uint64(r.Udp6Port)))
	}
	// From the devp2p docs:
	//
	// To sign record content with this scheme, apply the keccak256
	// hash function (as used by the EVM) to content, then create a
	// signature of the hash. The resulting 64-byte signature is
	// encoded as the concatenation of the r and s signature values
	// (the recovery ID v is omitted).
	sig, err := wsecp256k1.Sign(prv, isxhash.Keccak(rlp.List(rlp.Encode(buf...))))
	if err != nil {
		return nil, err
	}
	sigNoFmt := sig[:len(sig)-1] // remove formatting byte
	buf = append([][]byte{sigNoFmt}, buf...)
	return rlp.List(rlp.Encode(buf...)), nil
}
