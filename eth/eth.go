package eth

import (
	"fmt"

	"github.com/holiman/uint256"
	"github.com/indexsupply/x/bint"
	"github.com/indexsupply/x/rlp"
)

type Block struct {
	Number       uint64   // dup
	Hash         [32]byte // cache
	Header       Header
	Transactions Transactions
	Receipts     Receipts
	Uncles       []Header
}

type Header struct {
	Parent      [32]byte
	Uncle       [32]byte
	Coinbase    [20]byte
	StateRoot   [32]byte
	TxRoot      [32]byte
	ReceiptRoot [32]byte
	LogsBloom   [256]byte
	Difficulty  []byte
	Number      uint64
	GasLimit    uint64
	GasUsed     uint64
	Time        uint64
	Extra       []byte
	MixHash     [32]byte
	Nonce       uint64
	BaseFee     [32]byte
	Withdraw    [32]byte
	ExcessData  [32]byte
}

func (h *Header) Unmarshal(input []byte) {
	for i, itr := 0, rlp.Iter(input); itr.HasNext(); i++ {
		d := itr.Bytes()
		switch i {
		case 0:
			h.Parent = [32]byte(d)
		case 6:
			h.LogsBloom = [256]byte(d)
		case 8:
			h.Number = bint.Decode(d)
		case 11:
			h.Time = bint.Decode(d)
		}
	}
}

type Receipt struct {
	Status  []byte
	GasUsed uint64
	Bloom   [256]byte
	Logs    Logs
}

func (r *Receipt) Unmarshal(input []byte) {
	iter := rlp.Iter(input)
	r.Status = iter.Bytes()
	r.GasUsed = bint.Decode(iter.Bytes())
	r.Logs.Reset()
	for i, l := 0, rlp.Iter(iter.Bytes()); l.HasNext(); i++ {
		r.Logs.Insert(i, l.Bytes())
	}
}

type Receipts struct {
	d []Receipt
	n int
}

func (rs *Receipts) Reset()           { rs.n = 0 }
func (rs *Receipts) Len() int         { return rs.n }
func (rs *Receipts) At(i int) Receipt { return rs.d[i] }

func (rs *Receipts) Insert(i int, b []byte) {
	rs.n++
	switch {
	case i < len(rs.d):
		rs.d[i].Unmarshal(b)
	case len(rs.d) < cap(rs.d):
		r := Receipt{}
		r.Unmarshal(b)
		rs.d = append(rs.d, r)
	default:
		rs.d = append(rs.d, make([]Receipt, 512)...)
		r := Receipt{}
		r.Unmarshal(b)
		rs.d[i] = r
	}
}

type Topics struct {
	d [4][]byte
	n int
}

func (ts *Topics) Reset()          { ts.n = 0 }
func (ts *Topics) Len() int        { return ts.n }
func (ts *Topics) At(i int) []byte { return ts.d[i] }

func (ts *Topics) Insert(i int, b []byte) {
	ts.n++
	ts.d[i] = b
}

type Log struct {
	Address []byte
	Topics  Topics
	Data    []byte
}

func (l *Log) Unmarshal(input []byte) {
	iter := rlp.Iter(input)
	l.Address = iter.Bytes()
	l.Topics.Reset()
	for i, t := 0, rlp.Iter(iter.Bytes()); t.HasNext(); i++ {
		l.Topics.Insert(i, t.Bytes())
	}
	l.Data = iter.Bytes()
}

type Logs struct {
	d []Log
	n int
}

func (ls *Logs) Reset()       { ls.n = 0 }
func (ls *Logs) Len() int     { return ls.n }
func (ls *Logs) At(i int) Log { return ls.d[i] }

func (ls *Logs) Insert(i int, b []byte) {
	ls.n++
	switch {
	case i < len(ls.d):
		ls.d[i].Unmarshal(b)
	case len(ls.d) < cap(ls.d):
		l := Log{}
		l.Unmarshal(b)
		ls.d = append(ls.d, l)
	default:
		ls.d = append(ls.d, make([]Log, 8)...)
		l := Log{}
		l.Unmarshal(b)
		ls.d[i] = l
	}
}

type StorageKeys struct {
	d [][32]byte
	n int
}

func (sk *StorageKeys) Reset()            { sk.n = 0 }
func (sk *StorageKeys) Len() int          { return sk.n }
func (sk *StorageKeys) At(i int) [32]byte { return sk.d[i] }

func (sk *StorageKeys) Insert(i int, b []byte) {
	sk.n++
	switch {
	case i < len(sk.d):
		sk.d[i] = [32]byte(b)
	case len(sk.d) < cap(sk.d):
		sk.d = append(sk.d, [32]byte(b))
	default:
		sk.d = append(sk.d, make([][32]byte, 8)...)
		sk.d[i] = [32]byte(b)
	}
}

type AccessTuple struct {
	Address     [20]byte
	StorageKeys StorageKeys
}

func (at *AccessTuple) Unmarshal(b []byte) error {
	iter := rlp.Iter(b)
	at.Address = [20]byte(iter.Bytes())
	at.StorageKeys.Reset()
	for i, it := 0, rlp.Iter(iter.Bytes()); it.HasNext(); i++ {
		at.StorageKeys.Insert(i, it.Bytes())
	}
	return nil
}

type AccessList struct {
	d []AccessTuple
	n int
}

func (al *AccessList) Reset()               { al.n = 0 }
func (al *AccessList) Len() int             { return al.n }
func (al *AccessList) At(i int) AccessTuple { return al.d[i] }

func (al *AccessList) Insert(i int, b []byte) {
	al.n++
	switch {
	case i < len(al.d):
		al.d[i].Unmarshal(b)
	case len(al.d) < cap(al.d):
		t := AccessTuple{}
		t.Unmarshal(b)
		al.d = append(al.d, t)
	default:
		al.d = append(al.d, make([]AccessTuple, 4)...)
		t := AccessTuple{}
		t.Unmarshal(b)
		al.d[i] = t
	}
}

type Transaction struct {
	Hash     [32]byte // cache
	ChainID  uint256.Int
	Nonce    uint64
	GasPrice uint256.Int
	GasLimit uint64
	To       [20]byte
	Value    uint256.Int
	Data     []byte
	V, R, S  uint256.Int

	// EIP-2930
	AccessList AccessList

	// EIP-1559
	MaxPriorityFeePerGas uint256.Int
	MaxFeePerGas         uint256.Int
}

type Transactions struct {
	d []Transaction
	n int
}

func (txs *Transactions) Reset()               { txs.n = 0 }
func (txs *Transactions) Len() int             { return txs.n }
func (txs *Transactions) At(i int) Transaction { return txs.d[i] }

func (txs *Transactions) Insert(i int, b []byte) {
	txs.n++
	switch {
	case i < len(txs.d):
		txs.d[i].Unmarshal(b)
	case len(txs.d) < cap(txs.d):
		t := Transaction{}
		t.Unmarshal(b)
		txs.d = append(txs.d, t)
	default:
		txs.d = append(txs.d, make([]Transaction, 512)...)
		t := Transaction{}
		t.Unmarshal(b)
		txs.d[i] = t
	}
}

func (tx *Transaction) Unmarshal(b []byte) error {
	if len(b) < 1 {
		return fmt.Errorf("decoding empty transaction bytes")
	}
	//tx.Hash = isxhash.Keccak32(b)
	// Legacy Transaction
	if iter := rlp.Iter(b); iter.HasNext() {
		tx.Nonce = bint.Decode(iter.Bytes())
		tx.GasPrice.SetBytes(iter.Bytes())
		tx.GasLimit = bint.Decode(iter.Bytes())
		copy(tx.To[:], iter.Bytes())
		tx.Value.SetBytes(iter.Bytes())
		tx.Data = tx.Data[:0]
		tx.Data = append(tx.Data, iter.Bytes()...)
		tx.V.SetBytes(iter.Bytes())
		tx.R.SetBytes(iter.Bytes())
		tx.S.SetBytes(iter.Bytes())
		return nil
	}
	// EIP-2718: Typed Transaction
	// https://eips.ethereum.org/EIPS/eip-2718
	switch iter := rlp.Iter(b[1:]); b[0] {
	case 0x01:
		// EIP-2930: Access List
		// https://eips.ethereum.org/EIPS/eip-2930
		tx.ChainID.SetBytes(iter.Bytes())
		tx.Nonce = bint.Decode(iter.Bytes())
		tx.GasPrice.SetBytes(iter.Bytes())
		tx.GasLimit = bint.Decode(iter.Bytes())
		copy(tx.To[:], iter.Bytes())
		tx.Value.SetBytes(iter.Bytes())
		tx.Data = tx.Data[:0]
		tx.Data = append(tx.Data, iter.Bytes()...)
		tx.AccessList.Reset()
		for i, it := 0, rlp.Iter(iter.Bytes()); it.HasNext(); i++ {
			tx.AccessList.Insert(i, it.Bytes())
		}
		tx.V.SetBytes(iter.Bytes())
		tx.R.SetBytes(iter.Bytes())
		tx.S.SetBytes(iter.Bytes())
		return nil
	case 0x02:
		// EIP-1559: Dynamic Fee
		// https://eips.ethereum.org/EIPS/eip-1559
		tx.ChainID.SetBytes(iter.Bytes())
		tx.Nonce = bint.Decode(iter.Bytes())
		tx.MaxPriorityFeePerGas.SetBytes(iter.Bytes())
		tx.MaxFeePerGas.SetBytes(iter.Bytes())
		tx.GasLimit = bint.Decode(iter.Bytes())
		copy(tx.To[:], iter.Bytes())
		tx.Value.SetBytes(iter.Bytes())
		tx.Data = tx.Data[:0]
		tx.Data = append(tx.Data, iter.Bytes()...)
		tx.AccessList.Reset()
		for i, it := 0, rlp.Iter(iter.Bytes()); it.HasNext(); i++ {
			tx.AccessList.Insert(i, it.Bytes())
		}
		tx.V.SetBytes(iter.Bytes())
		tx.R.SetBytes(iter.Bytes())
		tx.S.SetBytes(iter.Bytes())
		return nil
	default:
		return fmt.Errorf("unsupported tx type: 0x%X", b[0])
	}
}
