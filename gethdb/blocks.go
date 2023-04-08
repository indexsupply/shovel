package gethdb

import (
	"encoding/binary"
	"fmt"
	"os"

	"github.com/indexsupply/x/eth"
	"github.com/indexsupply/x/isxhash"
	"github.com/indexsupply/x/jrpc"
	"github.com/indexsupply/x/rlp"
)

func New(fpath string, rc *jrpc.Client) Handle {
	return Handle{
		rc: rc,
		fz: &Freezer{
			dir:   fpath,
			files: map[fname]*os.File{},
		},
	}
}

type Handle struct {
	rc *jrpc.Client
	fz *Freezer
}

func (h Handle) Latest() (uint64, [32]byte, error) {
	res, err := h.rc.GetDB1([]byte("LastBlock"))
	if err != nil {
		return 0, [32]byte{}, fmt.Errorf("getting last block hash: %w", err)
	}
	res, err = h.rc.GetDB1(append([]byte("H"), res...))
	if err != nil {
		return 0, [32]byte{}, fmt.Errorf("getting last block hash: %w", err)
	}
	return binary.BigEndian.Uint64(res), [32]byte{}, nil
}

func (h Handle) Blocks(blocks []eth.Block) error {
	if len(blocks) == 0 {
		return fmt.Errorf("no blocks to query")
	}
	fmax, err := h.fz.Max("headers")
	if err != nil {
		return fmt.Errorf("loading max freezer: %w", err)
	}
	var first, last = blocks[0], blocks[len(blocks)-1]
	switch {
	case last.Number <= fmax:
		if err := freezerBlocks(blocks, h.fz); err != nil {
			return fmt.Errorf("loading freezer: %w", err)
		}
	case first.Number > fmax:
		if err := dbBlocks(blocks, h.rc); err != nil {
			return fmt.Errorf("loading db: %w", err)
		}
	case first.Number <= fmax:
		var split int
		for i, b := range blocks {
			if b.Number == fmax+1 {
				split = i
				break
			}
		}
		if err := freezerBlocks(blocks[:split], h.fz); err != nil {
			return fmt.Errorf("loading freezer: %w", err)
		}
		if err := dbBlocks(blocks[split:], h.rc); err != nil {
			return fmt.Errorf("loading db: %w", err)
		}
	default:
		panic("corrupt blocks query")
	}
	return validate(blocks)
}

func validate(blocks []eth.Block) error {
	if len(blocks) <= 1 {
		return nil
	}
	for i := 1; i < len(blocks); i++ {
		prev, curr := blocks[i-1], blocks[i]
		if curr.Header.Parent != prev.Hash {
			return fmt.Errorf(
				"invalid batch: %d %x != %d %x",
				prev.Number,
				prev.Hash[:4],
				curr.Number,
				curr.Header.Parent[:4],
			)
		}
	}
	return nil
}

func freezerBlocks(dst []eth.Block, frz *Freezer) error {
	var (
		buf []byte
		err error
	)
	for i := range dst {
		buf, err = frz.Read(buf, "headers", dst[i].Number)
		if err != nil {
			return fmt.Errorf("unable to load headers: %w", err)
		}
		dst[i].Hash = isxhash.Keccak32(buf)
		dst[i].Header.Unmarshal(buf)
		buf, err = frz.Read(buf, "bodies", dst[i].Number)
		if err != nil {
			return fmt.Errorf("unable to load bodies: %w", err)
		}
		bi := rlp.Iter(buf) //block iter contains: [transactions,uncles]
		dst[i].Transactions.Reset()
		for j, r := 0, rlp.Iter(bi.Read()); r.HasNext(); j++ {
			dst[i].Transactions.Insert(j, r.Read())
		}
		buf, err = frz.Read(buf, "receipts", dst[i].Number)
		if err != nil {
			return fmt.Errorf("unable to load receipts: %w", err)
		}
		dst[i].Receipts.Reset()
		for j, r := 0, rlp.Iter(buf); r.HasNext(); j++ {
			dst[i].Receipts.Insert(j, r.Read())
		}
	}
	return nil
}

func dbBlocks(blocks []eth.Block, rpc *jrpc.Client) error {
	keys := make([][]byte, len(blocks))
	for i := range blocks {
		keys[i] = headerHashKey(blocks[i].Number)
	}
	res, err := rpc.GetDB(keys)
	if err != nil {
		return fmt.Errorf("getting hashes: %w", err)
	}
	for i := 0; i < len(res); i++ {
		blocks[i].Hash = [32]byte(res[i])
	}
	for i := range blocks {
		keys[i] = headerKey(blocks[i].Number, blocks[i].Hash)
	}
	res, err = rpc.GetDB(keys)
	if err != nil {
		return fmt.Errorf("unable to load headers: %w", err)
	}
	for i, h := range res {
		if blocks[i].Hash != isxhash.Keccak32(h) {
			return fmt.Errorf("block hash mismatch")
		}
		blocks[i].Header.Unmarshal(h)
	}
	for i := range blocks {
		keys[i] = blockKey(blocks[i].Number, blocks[i].Hash)
	}
	res, err = rpc.GetDB(keys)
	if err != nil {
		return fmt.Errorf("unable to load bodies: %w", err)
	}
	for i, body := range res {
		bi := rlp.Iter(body) //block iter contains: [transactions,uncles]
		blocks[i].Transactions.Reset()
		for j, r := 0, rlp.Iter(bi.Read()); r.HasNext(); j++ {
			blocks[i].Transactions.Insert(j, r.Read())
		}
	}
	for i := range blocks {
		keys[i] = receiptsKey(blocks[i].Number, blocks[i].Hash)
	}
	res, err = rpc.GetDB(keys)
	if err != nil {
		return fmt.Errorf("unable to load receipts: %w", err)
	}
	for i, rs := range res {
		blocks[i].Receipts.Reset()
		for j, r := 0, rlp.Iter(rs); r.HasNext(); j++ {
			blocks[i].Receipts.Insert(j, r.Read())
		}
	}
	return nil
}

func headerHashKey(num uint64) (key []byte) {
	key = append(key, 'h')
	key = append(key, binary.BigEndian.AppendUint64([]byte{}, num)...)
	key = append(key, 'n')
	return
}

func headerKey(num uint64, hash [32]byte) (key []byte) {
	key = append(key, 'h')
	key = append(key, binary.BigEndian.AppendUint64([]byte{}, num)...)
	key = append(key, hash[:]...)
	return
}

func blockKey(num uint64, hash [32]byte) (key []byte) {
	key = append(key, 'b')
	key = append(key, binary.BigEndian.AppendUint64([]byte{}, num)...)
	key = append(key, hash[:]...)
	return
}

func receiptsKey(num uint64, hash [32]byte) (key []byte) {
	key = append(key, 'r')
	key = append(key, binary.BigEndian.AppendUint64([]byte{}, num)...)
	key = append(key, hash[:]...)
	return
}
