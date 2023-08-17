package erc721

import (
	"context"
	"log/slog"

	"github.com/indexsupply/x/contrib/erc721"
	"github.com/indexsupply/x/e2pg"

	"github.com/jackc/pgx/v5"
)

type integration struct {
	name string
}

var Integration = integration{
	name: "ERC721 Transfer",
}

func (i integration) Delete(ctx context.Context, pg e2pg.PG, n uint64) error {
	const q = `
		delete from nft_transfers
		where task_id = $1
		and chain_id = $2
		and block_number >= $3
	`
	_, err := pg.Exec(ctx, q, e2pg.TaskID(ctx), e2pg.ChainID(ctx), n)
	return err
}

func (i integration) Events(ctx context.Context) [][]byte {
	return [][]byte{erc721.TransferSignatureHash}
}

func (i integration) Insert(ctx context.Context, pg e2pg.PG, blocks []e2pg.Block) (int64, error) {
	var rows = make([][]any, 0, 1<<12)
	for bidx := 0; bidx < len(blocks); bidx++ {
		for ridx := 0; ridx < blocks[bidx].Receipts.Len(); ridx++ {
			r := blocks[bidx].Receipts.At(ridx)
			for lidx := 0; lidx < r.Logs.Len(); lidx++ {
				l := r.Logs.At(lidx)
				xfr, err := erc721.MatchTransfer(l)
				if err != nil {
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
					xfr.TokenId.String(),
					xfr.From[:],
					xfr.To[:],
				})
				xfr.Done()
			}
		}
	}
	return pg.CopyFrom(
		context.Background(),
		pgx.Identifier{"nft_transfers"},
		[]string{
			"task_id",
			"chain_id",
			"block_number",
			"block_hash",
			"transaction_hash",
			"transaction_index",
			"log_index",
			"tx_signer",
			"contract",
			"token_id",
			"f",
			"t",
		},
		pgx.CopyFromRows(rows),
	)
}
