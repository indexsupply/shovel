package discv4

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/indexsupply/x/bint"
	"github.com/indexsupply/x/discv4/kademlia"
	"github.com/indexsupply/x/enr"
	"github.com/indexsupply/x/isxerrors"
	"github.com/indexsupply/x/isxhash"
	"github.com/indexsupply/x/rlp"
	"github.com/indexsupply/x/wsecp256k1"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
)

func (p *process) Update() {
	for ; ; time.Sleep(5 * time.Second) {
		fmt.Printf("peer-count: %d\n", len(p.peers))
		if len(p.peers) >= 16 {
			continue
		}
		for _, peer := range p.peers {
			err := p.FindNode(p.prv.PubKey(), peer)
			if err != nil {
				fmt.Printf("error: find-node: %s\n", err)
				continue
			}
			break
		}
	}
}

type process struct {
	Verbose bool

	conn     net.PacketConn
	prv      *secp256k1.PrivateKey
	self     *enr.Record
	writeMut sync.Mutex
	peers    map[[32]byte]*enr.Record
	ktable   *kademlia.Table
}

func (p *process) log(format string, args ...any) {
	if p.Verbose {
		fmt.Printf(format, args...)
	}
}

func New(
	conn net.PacketConn,
	prv *secp256k1.PrivateKey,
	self *enr.Record,
) *process {
	return &process{
		conn:   conn,
		prv:    prv,
		self:   self,
		peers:  map[[32]byte]*enr.Record{},
		ktable: kademlia.New(self),
	}
}

func (p *process) Serve() {
	for {
		err := p.read()
		if err != nil {
			p.log("read error: %s\n", err)
		}
	}
}

func (p *process) read() error {
	buf := make([]byte, 1280)
	n, addr, err := p.conn.ReadFrom(buf)
	if err != nil {
		return isxerrors.Errorf("reading from conn: %w", err)
	}
	uaddr, ok := addr.(*net.UDPAddr)
	if !ok {
		return isxerrors.Errorf("converting udp address: %w", err)
	}
	return p.serve(uaddr, buf[:n])
}

const (
	// packet = hash || sig || pt || pd
	// hash = keccak256(sig || pt || pd)
	hashSize   = 32
	sigSize    = 65
	kindSize   = 1
	headerSize = hashSize + sigSize + kindSize
)

func (p *process) serve(uaddr *net.UDPAddr, packet []byte) error {
	if len(packet) <= headerSize {
		return errors.New("discv4 packet too small")
	}
	if !bytes.Equal(packet[:hashSize], isxhash.Keccak(packet[hashSize:])) {
		return errors.New("packet contains invalid hash")
	}
	fromPubkey, err := wsecp256k1.Recover(
		packet[hashSize:hashSize+sigSize],
		isxhash.Keccak(packet[hashSize+sigSize:]),
	)
	if err != nil {
		return errors.New("unable to extract pubkey from packet")
	}
	req := &enr.Record{
		PublicKey: fromPubkey,
		Ip:        uaddr.IP,
		UdpPort:   uint16(uaddr.Port),
	}

	kind := packet[hashSize+sigSize : headerSize][0]
	switch kind {
	case 0x01:
		err = p.handlePing(req, packet)
	case 0x02:
		err = p.handlePong(req, packet)
	case 0x03:
		err = p.handleFindNode(req, packet)
	case 0x04:
		err = p.handleNeighbors(req, packet)
	case 0x05:
		err = p.handleENRRequest(req, packet)
	default:
		p.log("< %x\n", packet)
	}
	return isxerrors.Errorf("serving %x: %w", kind, err)
}

func (p *process) handleENRRequest(req *enr.Record, packet []byte) error {
	// packet-data = [request-hash, ENR]
	t := bint.Uint32(rlp.Bytes(packet[headerSize:]))
	expiration := time.Unix(int64(t), 0)
	if expiration.Before(time.Now()) {
		return errors.New("expired enr request")
	}
	rec, err := p.self.MarshalRLP(p.prv)
	if err != nil {
		return err
	}
	_, err = p.write(0x06, req.UDPAddr(), rlp.EncodeList(
		isxhash.Keccak(packet),
		rec,
	))
	return err
}

func (p *process) handleFindNode(req *enr.Record, packet []byte) error {
	// packet-data = [target, expiration, ...]
	var (
		target = rlp.Bytes(packet[headerSize:])
		recs   = p.ktable.FindClosest(isxhash.Keccak32(target), 16)
		nodes  [][]byte
	)
	for _, rec := range recs {
		id := wsecp256k1.Encode(rec.PublicKey)
		nodes = append(nodes, rlp.EncodeList(
			rec.Ip,
			bint.Encode(nil, uint64(rec.UdpPort)),
			bint.Encode(nil, uint64(rec.TcpPort)),
			id,
		))
	}
	_, err := p.write(0x04, req.UDPAddr(), rlp.EncodeList(
		rlp.EncodeList(nodes...),
		bint.Encode(nil, uint64(time.Now().Add(time.Hour).Unix())),
	))
	return err
}

func (p *process) handleNeighbors(req *enr.Record, packet []byte) error {
	// packet-data = [nodes, expiration, ...]
	// nodes = [[ip, udp-port, tcp-port, node-id], ...]
	var (
		pd      = packet[headerSize:]
		itr     = rlp.Iter(pd)
		records []*enr.Record
	)
	for s := rlp.Iter(itr.Bytes()); s.HasNext(); {
		var (
			node = rlp.Iter(s.Bytes())
			rec  = &enr.Record{}
			err  error
		)
		rec.Ip = net.IP(node.Bytes())
		rec.UdpPort = bint.Uint16(node.Bytes())
		rec.TcpPort = bint.Uint16(node.Bytes())
		rec.PublicKey, err = wsecp256k1.Decode(node.Bytes())
		if err != nil {
			return isxerrors.Errorf("reading pubkey: %w", err)
		}
		if rec.ID() == p.self.ID() {
			continue
		}
		records = append(records, rec)
	}

	for _, rec := range records {
		err := p.Ping(rec)
		if err != nil {
			return err
		}
	}
	p.log("<neighbors: %d\n", len(records))
	return nil
}

func (p *process) handlePing(req *enr.Record, packet []byte) error {
	// packet-data = [version, from, to, expiration, enr-seq ...]
	// version = 4
	// from = [sender-ip, sender-udp-port, sender-tcp-port]
	// to = [recipient-ip, recipient-udp-port, 0]
	var (
		hash = packet[:hashSize]
		pd   = packet[headerSize:]
	)
	itr := rlp.Iter(pd)
	itr.Bytes() //skip
	frm := rlp.Iter(itr.Bytes())

	reqFrom := net.IP(frm.Bytes())
	if !reqFrom.Equal(req.Ip) {
		return errors.New("packet ip address doesn't match udp")
	}
	reqFromPort := bint.Uint16(frm.Bytes())
	if reqFromPort != req.UdpPort {
		return errors.New("mismatch ping from-port with udp packet")
	}
	p.log("<ping: %s %x\n", req, hash[:4])

	err := p.Pong(hash, req)
	if err != nil {
		return err
	}

	p.writeMut.Lock()
	peer, ok := p.peers[req.ID()]
	if !ok {
		peer = req
		p.peers[peer.ID()] = peer
	}
	peer.ReceivedPing = time.Now()
	if !peer.ReceivedPing.IsZero() && !peer.ReceivedPong.IsZero() {
		p.ktable.Insert(peer)
	}
	p.writeMut.Unlock()

	if time.Since(peer.SentPing) > time.Hour {
		return p.Ping(peer)
	}

	return nil
}

func (p *process) handlePong(req *enr.Record, packet []byte) error {
	// packet-data = [to, ping-hash, expiration, enr-seq, ...]
	pd := packet[headerSize:]
	itr := rlp.Iter(pd)
	itr.Bytes() //skip
	hash := itr.Bytes()

	p.log("<pong: %s %x\n", req, hash[:4])

	p.writeMut.Lock()
	defer p.writeMut.Unlock()
	peer := p.peers[req.ID()]
	switch {
	case peer == nil:
		return errors.New("missing peer")
	case !bytes.Equal(peer.SentPingHash[:], hash[:]):
		return errors.New("invalid ping hash")
	case time.Since(peer.SentPing) > time.Minute:
		return errors.New("expired ping hash")
	}

	peer.ReceivedPong = time.Now()
	if !peer.ReceivedPing.IsZero() && !peer.ReceivedPong.IsZero() {
		p.ktable.Insert(peer)
	}
	return nil
}

// Assembles an Item for transmission. Steps include:
// 1. rlp encoding item
// 2. assembling a signature
// 3. hashing the contents
// 4. combining all of that data into a packet
// 5. sending the packet
// The packet composition is as follows:
// - packet = packet-header || packet-data
// - packet-header = hash || signature || packet-type
// - hash = keccak256(signature || packet-type || packet-data)
// - signature = sign(packet-type || packet-data)
func (p *process) write(pt byte, to *net.UDPAddr, data []byte) ([]byte, error) {
	var ts []byte
	ts = append(ts, pt)
	ts = append(ts, data...)
	sig, err := wsecp256k1.Sign(p.prv, isxhash.Keccak(ts))
	if err != nil {
		return nil, err
	}

	var th, hash []byte
	th = append(th, sig[:]...)
	th = append(th, pt)
	th = append(th, data...)
	hash = isxhash.Keccak(th)

	var header []byte
	header = append(header, hash...)
	header = append(header, sig[:]...)
	header = append(header, pt)

	packet := append(header, data...)
	_, err = p.conn.WriteTo(packet, to)
	return hash, err
}

func (p *process) FindNode(target *secp256k1.PublicKey, dest *enr.Record) error {
	_, err := p.write(0x03, dest.UDPAddr(), rlp.EncodeList(
		wsecp256k1.Encode(target),
		bint.Encode(nil, uint64(time.Now().Add(time.Hour).Unix())),
	))
	p.log(">find: %s\n", dest)
	return err
}

func (p *process) Pong(pingHash []byte, dest *enr.Record) error {
	_, err := p.write(0x02, dest.UDPAddr(), rlp.EncodeList(
		rlp.EncodeList(
			dest.Ip,
			bint.Encode(nil, uint64(dest.UdpPort)),
			bint.Encode(nil, uint64(dest.TcpPort)),
		),
		pingHash,
		bint.Encode(nil, uint64(time.Now().Add(time.Hour).Unix())),
	))
	p.log(">pong: %s\n", dest)
	return err
}

func (p *process) Ping(dest *enr.Record) error {
	p.writeMut.Lock()
	defer p.writeMut.Unlock()

	if pr, ok := p.peers[dest.ID()]; ok && time.Since(pr.SentPing) < time.Hour {
		p.log("skip-ping %s\n", pr)
		return nil
	}

	h, err := p.write(0x01, dest.UDPAddr(), rlp.EncodeList(
		[]byte{4},
		rlp.EncodeList(
			p.self.Ip,
			bint.Encode(nil, uint64(p.self.UdpPort)),
			bint.Encode(nil, uint64(p.self.TcpPort)),
		),
		rlp.EncodeList(
			dest.Ip,
			bint.Encode(nil, uint64(dest.UdpPort)),
			bint.Encode(nil, uint64(dest.TcpPort)),
		),
		bint.Encode(nil, uint64(time.Now().Add(time.Hour).Unix())),
	))
	if err != nil {
		return err
	}

	p.log(">ping: %s %x\n", dest, h[:4])
	dest.SentPing = time.Now()
	dest.SentPingHash = *(*[32]byte)(h)
	p.peers[dest.ID()] = dest
	return nil
}
