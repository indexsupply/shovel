package erc20

import (
	"bytes"
	"context"
	"log/slog"

	"github.com/indexsupply/x/e2pg"
	"github.com/indexsupply/x/isxhash"

	"github.com/holiman/uint256"
	"github.com/jackc/pgx/v5"
)

var sig, sigHash []byte

func init() {
	sig = isxhash.Keccak([]byte("Transfer(address,address,uint256)"))
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

func (i integration) Delete(ctx context.Context, pg e2pg.PG, n uint64) error {
	const q = `
		delete from erc20_transfers
		where task_id = $1
		and chain_id = $2
		and block_number >= $3
	`
	_, err := pg.Exec(ctx, q, e2pg.TaskID(ctx), e2pg.ChainID(ctx), n)
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
				signer, err := blocks[bidx].Transactions.At(ridx).Signer()
				if err != nil {
					slog.ErrorContext(ctx, "unable to derive signer")
				}
				rows = append(rows, []any{
					e2pg.TaskID(ctx),
					e2pg.ChainID(ctx),
					blocks[bidx].Num(),
					blocks[bidx].Hash(),
					blocks[bidx].Transactions.At(ridx).Hash(),
					ridx,
					lidx,
					signer,
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
		"tx_signer",
		"contract",
		"f",
		"t",
		"value",
	}, pgx.CopyFromRows(rows))
}
