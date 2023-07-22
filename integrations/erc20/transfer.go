package erc20

import (
	"bytes"
	"context"
	"encoding/hex"

	"github.com/indexsupply/x/e2pg"
	"github.com/indexsupply/x/isxhash"

	"github.com/holiman/uint256"
	"github.com/jackc/pgx/v5"
)

var sig, sigHash []byte

func init() {
	var err error
	// Obtained by inputing Transfer(address,address,uint256) into https://emn178.github.io/online-tools/keccak_256.html
	sig, err = hex.DecodeString("ddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef")
	if err != nil {
		panic("unable to decode erc20 transfer sig")
	}
	sigHash = isxhash.Keccak(sig)
}

type integration struct {
	name string
}

var Integration = integration{
	name: "ERC20 Transfer",
}

func (i integration) Events(ctx context.Context) [][]byte {
	return [][]byte{sigHash}
}

func (i integration) Delete(ctx context.Context, pg e2pg.PG, h []byte) error {
	const q = `
		delete from erc20_transfers
		where task_id = $1
		and chain_id = $2
		and block_hash = $3
	`
	_, err := pg.Exec(ctx, q, e2pg.TaskID(ctx), e2pg.ChainID(ctx), h)
	return err
}

func addr(b []byte) []byte {
	if len(b) < 32 {
		return nil
	}
	return b[12:]
}

func u256(b []byte) string {
	n := new(uint256.Int)
	n.SetBytes(b)
	return n.Dec()
}

func (i integration) Insert(ctx context.Context, pg e2pg.PG, blocks []e2pg.Block) (int64, error) {
	var rows = make([][]any, 0, 1<<12)
	for bidx := 0; bidx < len(blocks); bidx++ {
		for ridx := 0; ridx < blocks[bidx].Receipts.Len(); ridx++ {
			r := blocks[bidx].Receipts.At(ridx)
			for lidx := 0; lidx < r.Logs.Len(); lidx++ {
				l := r.Logs.At(lidx)
				if !bytes.Equal(l.Topics.At(0), sig) {
					continue
				}
				rows = append(rows, []any{
					e2pg.TaskID(ctx),
					e2pg.ChainID(ctx),
					blocks[bidx].Num(),
					blocks[bidx].Hash(),
					blocks[bidx].Transactions.At(ridx).Hash(),
					ridx,
					lidx,
					l.Address,
					addr(l.Topics.At(1)),
					addr(l.Topics.At(2)),
					u256(l.Data),
				})
			}
		}
	}
	return pg.CopyFrom(ctx, pgx.Identifier{"erc20_transfers"}, []string{
		"task_id",
		"chain_id",
		"block_number",
		"block_hash",
		"transaction_hash",
		"transaction_index",
		"log_index",
		"contract",
		"f",
		"t",
		"value",
	}, pgx.CopyFromRows(rows))
}
