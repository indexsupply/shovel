// ethereum types
package eth

import (
	"encoding/hex"
	"fmt"
	"sync"

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

type Byte byte

func (b *Byte) Write(p byte) (int, error) {
	*b = Byte(p)
	return 1, nil
}

func (b *Byte) UnmarshalJSON(data []byte) error {
	if len(data) < 4 {
		return fmt.Errorf("must be at leaset 4 bytes")
	}
	data = data[1 : len(data)-1] // remove quotes
	data = data[2:]              // remove 0x
	n, err := decode(string(data))
	*b = Byte(n)
	return err
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
	sync.Mutex

	Header
	Txs Txs `json:"transactions"`
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

func (b *Block) Tx(idx uint64) *Tx {
	for i := range b.Txs {
		if uint64(b.Txs[i].Idx) == idx {
			return &b.Txs[i]
		}
	}
	b.Txs = append(b.Txs, Tx{Idx: Uint64(idx)})
	return &b.Txs[len(b.Txs)-1]
}

type Log struct {
	BlockNumber Uint64  `json:"blockNumber"`
	TxHash      Bytes   `json:"transactionHash"`
	Idx         Uint64  `json:"logIndex"`
	TxIdx       Uint64  `json:"transactionIndex"`
	Address     Bytes   `json:"address"`
	Topics      []Bytes `json:"topics"`
	Data        Bytes   `json:"data"`
}

type Logs []Log

func (ls *Logs) Add(other *Log) {
	for i := range *ls {
		if (*ls)[i].Idx == other.Idx {
			return
		}
	}

	l := Log{}
	l.BlockNumber = other.BlockNumber
	l.TxHash.Write(other.TxHash)
	l.Idx = other.Idx
	l.TxIdx = other.TxIdx
	l.Address.Write(other.Address)
	l.Topics = make([]Bytes, len(other.Topics))
	for i := range other.Topics {
		l.Topics[i].Write(other.Topics[i])
	}
	l.Data.Write(other.Data)
	*ls = append(*ls, l)
}

type Receipt struct {
	Status              Byte
	GasUsed             Uint64
	EffectiveGasPrice   uint256.Int
	Logs                Logs
	ContractAddress     Bytes
	L1BaseFeeScalar     *uint256.Int `json:"l1BaseFeeScalar,omitempty"`
	L1BlobBaseFee       *uint256.Int `json:"l1BlobBaseFee,omitempty"`
	L1BlobBaseFeeScalar *uint256.Int `json:"l1BlobBaseFeeScalar,omitempty"`
	L1Fee               *uint256.Int `json:"l1Fee,omitempty"`
	L1GasPrice          *uint256.Int `json:"l1GasPrice,omitempty"`
	L1GasUsed           *Uint64      `json:"l1GasUsed,omitempty"`
}

type Header struct {
	Number    Uint64 `json:"number"`
	Hash      Bytes  `json:"hash"`
	Parent    Bytes  `json:"parentHash"`
	LogsBloom Bytes  `json:"logsBloom"`
	Time      Uint64 `json:"timestamp"`
}

type AccessTuple struct {
	Address     [20]byte
	StorageKeys [][32]byte
}

type AccessTuples []AccessTuple

type Txs []Tx

type TraceAction struct {
	Idx      uint64
	From     Bytes       `json:"from"`
	CallType string      `json:"callType"`
	To       Bytes       `json:"to"`
	Value    uint256.Int `json:"value"`
}

type Tx struct {
	Receipt
	Idx      Uint64      `json:"transactionIndex"`
	Type     Byte        `json:"type"`
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

	TraceActions []TraceAction

	// EIP-2930
	AccessList AccessTuples `json:"-"` // TODO

	// EIP-1559
	MaxPriorityFeePerGas uint256.Int `json:"maxPriorityFeePerGas"`
	MaxFeePerGas         uint256.Int `json:"maxFeePerGas"`

	PrecompHash  Bytes `json:"hash"`
	cacheMut     sync.Mutex
	rbuf, signer []byte
}

func (tx *Tx) Hash() []byte {
	tx.cacheMut.Lock()
	defer tx.cacheMut.Unlock()
	if len(tx.PrecompHash) == 0 {
		tx.PrecompHash = Keccak(tx.rbuf)
	}
	return tx.PrecompHash
}

func (tx *Tx) Signer() ([]byte, error) {
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
