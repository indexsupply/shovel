package e2pg

import (
	"bytes"
	"fmt"

	"github.com/indexsupply/x/bint"
	"github.com/indexsupply/x/bloom"
	"github.com/indexsupply/x/eth"
	"github.com/indexsupply/x/freezer"
	"github.com/indexsupply/x/geth"
	"github.com/indexsupply/x/jrpc"
	"github.com/indexsupply/x/rlp"
)

func NewGeth(fc freezer.FileCache, rc *jrpc.Client) *Geth {
	return &Geth{fc: fc, rc: rc}
}

type Geth struct {
	fc freezer.FileCache
	rc *jrpc.Client
}

func (g *Geth) ChainID() uint64 { return 0 }

func (g *Geth) Hash(num uint64) ([]byte, error) {
	return geth.Hash(num, g.fc, g.rc)
}

func (g *Geth) Latest() (uint64, []byte, error) {
	n, h, err := geth.Latest(g.rc)
	if err != nil {
		return 0, nil, fmt.Errorf("getting last block hash: %w", err)
	}
	return bint.Uint64(n), h, nil
}

func skip(filter [][]byte, bf bloom.Filter) bool {
	if len(filter) == 0 {
		return false
	}
	for i := range filter {
		if !bf.Missing(filter[i]) {
			return false
		}
	}
	return true
}

func (g *Geth) LoadBlocks(filter [][]byte, blks []eth.Block) error {
	//TODO(r): this is a garbage factory and should be refactored
	bfs := make([]geth.Buffer, len(blks))
	for i := range blks {
		bfs[i].Number = blks[i].Num()
	}
	err := geth.Load(filter, bfs, g.fc, g.rc)
	if err != nil {
		return fmt.Errorf("loading data: %w", err)
	}
	for i := range blks {
		blks[i].Header.UnmarshalRLP(bfs[i].Header())
		if skip(filter, bloom.Filter(blks[i].Header.LogsBloom)) {
			continue
		}
		//rlp contains: [transactions,uncles]
		blks[i].Txs.UnmarshalRLP(rlp.Bytes(bfs[i].Bodies()))
		blks[i].Receipts.UnmarshalRLP(bfs[i].Receipts())
	}
	return validate(blks)
}

func validate(blocks []eth.Block) error {
	if len(blocks) <= 1 {
		return nil
	}
	for i := 1; i < len(blocks); i++ {
		prev, curr := blocks[i-1], blocks[i]
		if !bytes.Equal(curr.Header.Parent, prev.Hash()) {
			return fmt.Errorf("invalid batch. prev=%s curr=%s", prev, curr)
		}
	}
	return nil
}
