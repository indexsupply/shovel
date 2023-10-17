// ethereum types
package eth

import (
	"encoding/hex"
	"fmt"
	"sync"

	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/indexsupply/x/bint"
	"github.com/indexsupply/x/isxhash"
	"github.com/indexsupply/x/rlp"

	"github.com/holiman/uint256"
)

// returns pointer to x[i]. extends x if i >= len(x)
func get[X ~[]E, E any](x X, i int) (X, *E) {
	switch {
	case i < 0:
		panic("cannot be negative")
	case i < len(x):
		return x, &x[i]
	case i >= len(x):
		n := max(1, i+1-len(x))
		x = append(x, make(X, n)...)
		return x, &x[i]
	default:
		panic("default")
	}
}

type Uint64 uint64

func decode(b string) (uint64, error) {
	var res uint64
	for i := range b {
		var nibble uint64
		switch {
		case b[i] >= '0' && b[i] <= '9':
			nibble = uint64(b[i] - '0')
		case b[i] >= 'a' && b[i] <= 'f':
			nibble = uint64(b[i] - 'a' + 10)
		case b[i] >= 'A' && b[i] <= 'F':
			nibble = uint64(b[i] - 'A' + 10)
		default:
			return 0, fmt.Errorf("invalid hex %x", b)
		}
		res = (res << 4) | nibble
		if i == 15 {
			break
		}
	}
	return res, nil
}

func (hn *Uint64) UnmarshalJSON(data []byte) error {
	if len(data) < 4 {
		return fmt.Errorf("must be at leaset 4 bytes")
	}
	data = data[1 : len(data)-1] // remove quotes
	data = data[2:]              // remove 0x
	n, err := decode(string(data))
	*hn = Uint64(n)
	return err
}

type hbyte byte

func (hb *hbyte) UnmarshalJSON(data []byte) error {
	if len(data) < 4 {
		return fmt.Errorf("must be at leaset 4 bytes")
	}
	data = data[1 : len(data)-1] // remove quotes
	data = data[2:]              // remove 0x
	*hb = hbyte(data[0])
	return nil
}

type Bytes []byte

func (hb *Bytes) Bytes() []byte {
	return []byte(*hb)
}

func (hb *Bytes) UnmarshalJSON(data []byte) error {
	if len(data) < 4 {
		return fmt.Errorf("must be at leaset 4 bytes")
	}
	data = data[1 : len(data)-1] // remove quotes
	data = data[2:]              // remove 0x
	if len(*hb) < len(data)/2 {
		n := len(data)/2 - len(*hb)
		*hb = append(*hb, make(Bytes, n)...)
	}
	*hb = (*hb)[:len(data)/2]
	_, err := hex.Decode(*hb, data)
	return err
}

func (hb Bytes) MarshalJSON() ([]byte, error) {
	return []byte(`"` + fmt.Sprintf("0x%x", hb) + `"`), nil
}

func (hb *Bytes) Write(p []byte) (int, error) {
	if len(*hb) < len(p) {
		*hb = append(*hb, make([]byte, len(p)-len(*hb))...)
	}
	*hb = (*hb)[:len(p)]
	return copy(*hb, p), nil
}

type Block struct {
	Header
	Txs      Txs `json:"transactions"`
	Receipts Receipts
}

func (b *Block) Reset() {
	for i := range b.Txs {
		b.Txs[i].Reset()
	}
	b.Receipts = b.Receipts[:0]
}

func (b *Block) SetNum(n uint64) { b.Header.Number = Uint64(n) }
func (b Block) Num() uint64      { return uint64(b.Header.Number) }
func (b Block) Hash() []byte     { return b.Header.Hash }

func (b Block) String() string {
	return fmt.Sprintf("{%d %.4x %.4x}",
		b.Header.Number,
		b.Header.Parent,
		b.Header.Hash,
	)
}

type Log struct {
	Address Bytes   `json:"address"`
	Topics  []Bytes `json:"topics"`
	Data    Bytes   `json:"data"`
}

func (l *Log) UnmarshalRLP(b []byte) {
	var (
		iter    = rlp.Iter(b)
		ntopics int
	)
	l.Address.Write(iter.Bytes())
	for i, it := 0, rlp.Iter(iter.Bytes()); it.HasNext(); i++ {
		var t *Bytes
		l.Topics, t = get(l.Topics, i)
		t.Write(it.Bytes())
		ntopics++
	}
	l.Topics = l.Topics[:ntopics]
	l.Data.Write(iter.Bytes())
}

type Logs []Log

func (ls *Logs) UnmarshalRLP(b []byte) {
	var i int
	for it := rlp.Iter(b); it.HasNext(); i++ {
		var l *Log
		*ls, l = get(*ls, i)
		l.UnmarshalRLP(it.Bytes())
	}
	*ls = (*ls)[:i]
}

type Receipt struct {
	Status  Bytes
	GasUsed Uint64
	Logs    Logs
}

func (r *Receipt) UnmarshalRLP(b []byte) {
	iter := rlp.Iter(b)
	r.Status.Write(iter.Bytes())
	r.GasUsed = Uint64(bint.Uint64(iter.Bytes()))
	r.Logs.UnmarshalRLP(iter.Bytes())
}

type Receipts []Receipt

func (rs *Receipts) UnmarshalRLP(b []byte) {
	var j int
	for it := rlp.Iter(b); it.HasNext(); j++ {
		var r *Receipt
		*rs, r = get(*rs, j)
		r.UnmarshalRLP(it.Bytes())
	}
	*rs = (*rs)[:j]
}

type Header struct {
	Number    Uint64 `json:"number"`
	Hash      Bytes  `json:"hash"`
	Parent    Bytes  `json:"parentHash"`
	LogsBloom Bytes  `json:"logsBloom"`
	Time      Uint64 `json:"timestamp"`
}

func (h *Header) UnmarshalRLP(b []byte) {
	h.Hash = isxhash.Keccak(b)
	for i, it := 0, rlp.Iter(b); it.HasNext(); i++ {
		d := it.Bytes()
		switch i {
		case 0:
			h.Parent.Write(d)
		case 6:
			h.LogsBloom.Write(d)
		case 8:
			h.Number = Uint64(bint.Uint64(d))
		case 11:
			h.Time = Uint64(bint.Uint64(d))
		}
	}
}

type AccessTuple struct {
	Address     [20]byte
	StorageKeys [][32]byte
}

func (at *AccessTuple) MarshalRLP() []byte {
	var keys [][]byte
	for i := range at.StorageKeys {
		keys = append(keys, rlp.Encode(at.StorageKeys[i][:]))
	}
	return rlp.List(
		rlp.Encode(at.Address[:]),
		rlp.List(keys...),
	)
}

func (at *AccessTuple) UnmarshalRLP(b []byte) error {
	it := rlp.Iter(b)
	copy(at.Address[:], it.Bytes())
	for i, it := 0, rlp.Iter(it.Bytes()); it.HasNext(); i++ {
		at.StorageKeys = append(at.StorageKeys, [32]byte(it.Bytes()))
	}
	return nil
}

type AccessTuples []AccessTuple

func (ats *AccessTuples) MarshalRLP() []byte {
	var res [][]byte
	for i := range *ats {
		res = append(res, (*ats)[i].MarshalRLP())
	}
	return rlp.List(res...)
}

func (ats *AccessTuples) UnmarshalRLP(b []byte) error {
	var i int
	for it := rlp.Iter(b); it.HasNext(); i++ {
		var at *AccessTuple
		*ats, at = get(*ats, i)
		if err := at.UnmarshalRLP(it.Bytes()); err != nil {
			return fmt.Errorf("unmarshaling access tuple: %w", err)
		}
	}
	*ats = (*ats)[:i]
	return nil
}

type Txs []Tx

func (txs *Txs) UnmarshalRLP(b []byte) {
	var i int
	for it := rlp.Iter(b); it.HasNext(); i++ {
		var tx *Tx
		*txs, tx = get(*txs, i)
		tx.Reset()
		tx.UnmarshalRLP(it.Bytes())
	}
	*txs = (*txs)[:i]
}

type Tx struct {
	Type     hbyte       `json:"type"`
	ChainID  uint256.Int `json:"chainID"`
	Nonce    Uint64      `json:"nonce"`
	GasPrice uint256.Int `json:"gasPrice"`
	GasLimit Uint64      `json:"gas"`
	From     Bytes       `json:"from"`
	To       Bytes       `json:"to"`
	Value    uint256.Int `json:"value"`
	Data     Bytes       `json:"input"`
	V        uint256.Int `json:"v"`
	R        uint256.Int `json:"r"`
	S        uint256.Int `json:"s"`

	// EIP-2930
	AccessList AccessTuples `json:"-"` // TODO

	// EIP-1559
	MaxPriorityFeePerGas uint256.Int
	MaxFeePerGas         uint256.Int

	PrecompHash  Bytes `json:"hash"`
	cacheMut     sync.Mutex
	rbuf, signer []byte
}

func (tx *Tx) Reset() {
	tx.cacheMut.Lock()
	tx.PrecompHash = tx.PrecompHash[:0]
	tx.rbuf = tx.rbuf[:0]
	tx.signer = tx.signer[:0]
	tx.cacheMut.Unlock()
}

func (tx *Tx) Hash() []byte {
	tx.cacheMut.Lock()
	defer tx.cacheMut.Unlock()
	if len(tx.PrecompHash) == 0 {
		tx.PrecompHash = isxhash.Keccak(tx.rbuf)
	}
	return tx.PrecompHash
}

func (tx *Tx) UnmarshalRLP(b []byte) error {
	if len(b) < 1 {
		return fmt.Errorf("decoding empty transaction bytes")
	}
	tx.rbuf = append(tx.rbuf[:0], b...)
	// Legacy Transaction
	if it := rlp.Iter(b); it.HasNext() {
		tx.Type = 0x00
		tx.Nonce = Uint64(bint.Decode(it.Bytes()))
		tx.GasPrice.SetBytes(it.Bytes())
		tx.GasLimit = Uint64(bint.Decode(it.Bytes()))
		tx.To.Write(it.Bytes())
		tx.Value.SetBytes(it.Bytes())
		tx.Data.Write(it.Bytes())
		tx.V.SetBytes(it.Bytes())
		tx.R.SetBytes(it.Bytes())
		tx.S.SetBytes(it.Bytes())
		return nil
	}
	// EIP-2718: Typed Transaction
	// https://eips.ethereum.org/EIPS/eip-2718
	switch iter := rlp.Iter(b[1:]); b[0] {
	case 0x01:
		// EIP-2930: Access List
		// https://eips.ethereum.org/EIPS/eip-2930
		tx.Type = 0x01
		tx.ChainID.SetBytes(iter.Bytes())
		tx.Nonce = Uint64(bint.Decode(iter.Bytes()))
		tx.GasPrice.SetBytes(iter.Bytes())
		tx.GasLimit = Uint64(bint.Decode(iter.Bytes()))
		tx.To.Write(iter.Bytes())
		tx.Value.SetBytes(iter.Bytes())
		tx.Data.Write(iter.Bytes())
		tx.AccessList.UnmarshalRLP(iter.Bytes())
		tx.V.SetBytes(iter.Bytes())
		tx.R.SetBytes(iter.Bytes())
		tx.S.SetBytes(iter.Bytes())
		return nil
	case 0x02:
		// EIP-1559: Dynamic Fee
		// https://eips.ethereum.org/EIPS/eip-1559
		tx.Type = 0x02
		tx.ChainID.SetBytes(iter.Bytes())
		tx.Nonce = Uint64(bint.Decode(iter.Bytes()))
		tx.MaxPriorityFeePerGas.SetBytes(iter.Bytes())
		tx.MaxFeePerGas.SetBytes(iter.Bytes())
		tx.GasLimit = Uint64(bint.Decode(iter.Bytes()))
		tx.To.Write(iter.Bytes())
		tx.Value.SetBytes(iter.Bytes())
		tx.Data.Write(iter.Bytes())
		tx.AccessList.UnmarshalRLP(iter.Bytes())
		tx.V.SetBytes(iter.Bytes())
		tx.R.SetBytes(iter.Bytes())
		tx.S.SetBytes(iter.Bytes())
		return nil
	default:
		return fmt.Errorf("unsupported tx type: 0x%X", b[0])
	}
}

func (tx *Tx) Signer() ([]byte, error) {
	tx.cacheMut.Lock()
	defer tx.cacheMut.Unlock()
	if len(tx.From) > 0 {
		return tx.From, nil
	}
	var sig [65]byte
	sig[0] = tx.v()
	copy(sig[33-len(tx.R.Bytes()):33], tx.R.Bytes())
	copy(sig[65-len(tx.S.Bytes()):65], tx.S.Bytes())
	pubk, _, err := ecdsa.RecoverCompact(sig[:], tx.SigHash())
	if err != nil {
		return nil, fmt.Errorf("recovering pubkey: %w", err)
	}
	var (
		cpk  = pubk.SerializeUncompressed()
		addr = isxhash.Keccak(cpk[1:])
	)
	tx.From = append(tx.From[:0], addr[12:]...)
	return tx.From, nil
}

func (tx *Tx) v() byte {
	switch v := tx.V.Uint64(); {
	case v >= 35:
		return byte(27 + ((v - 35) % 2))
	case v == 27 || v == 28:
		return byte(v)
	case v == 0 || v == 1:
		return byte(27 + v)
	default:
		panic(fmt.Sprintf("unkown v: %d", v))
	}
}

func (tx *Tx) eip155() bool {
	switch v := tx.V.Uint64(); {
	case v == 27 || v == 28:
		return false
	default:
		return true
	}
}

func (tx *Tx) chainid() uint64 {
	switch v := tx.V.Uint64(); {
	case v >= 35:
		return (v - 35) >> 1
	default:
		return 0
	}
}

func (t *Tx) SigHash() []byte {
	switch t.Type {
	case 0x00:
		switch {
		case t.eip155():
			return isxhash.Keccak(rlp.List(
				rlp.Encode(bint.Encode(nil, uint64(t.Nonce))),
				rlp.Encode(t.GasPrice.Bytes()),
				rlp.Encode(bint.Encode(nil, uint64(t.GasLimit))),
				rlp.Encode(t.To),
				rlp.Encode(t.Value.Bytes()),
				rlp.Encode(t.Data),
				rlp.Encode(bint.Encode(nil, t.chainid())),
				rlp.Encode([]byte{0}),
				rlp.Encode([]byte{0}),
			))
		default:
			return isxhash.Keccak(rlp.List(
				rlp.Encode(bint.Encode(nil, uint64(t.Nonce))),
				rlp.Encode(t.GasPrice.Bytes()),
				rlp.Encode(bint.Encode(nil, uint64(t.GasLimit))),
				rlp.Encode(t.To),
				rlp.Encode(t.Value.Bytes()),
				rlp.Encode(t.Data),
			))
		}
	case 0x01:
		return isxhash.Keccak(append([]byte{0x01}, rlp.List(
			rlp.Encode(t.ChainID.Bytes()),
			rlp.Encode(bint.Encode(nil, uint64(t.Nonce))),
			rlp.Encode(t.GasPrice.Bytes()),
			rlp.Encode(bint.Encode(nil, uint64(t.GasLimit))),
			rlp.Encode(t.To),
			rlp.Encode(t.Value.Bytes()),
			rlp.Encode(t.Data),
			t.AccessList.MarshalRLP(),
		)...))
	case 0x02:
		return isxhash.Keccak(append([]byte{0x02}, rlp.List(
			rlp.Encode(t.ChainID.Bytes()),
			rlp.Encode(bint.Encode(nil, uint64(t.Nonce))),
			rlp.Encode(t.MaxPriorityFeePerGas.Bytes()),
			rlp.Encode(t.MaxFeePerGas.Bytes()),
			rlp.Encode(bint.Encode(nil, uint64(t.GasLimit))),
			rlp.Encode(t.To),
			rlp.Encode(t.Value.Bytes()),
			rlp.Encode(t.Data),
			t.AccessList.MarshalRLP(),
		)...))
	default:
		return nil
	}
}
