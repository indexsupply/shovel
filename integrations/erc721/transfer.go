package erc721

import (
	"context"
	"log/slog"

	"github.com/indexsupply/x/contrib/erc721"
	"github.com/indexsupply/x/e2pg"
	"github.com/indexsupply/x/eth"

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

func (i integration) Insert(ctx context.Context, pg e2pg.PG, blocks []eth.Block) (int64, error) {
	var rows = make([][]any, 0, 1<<12)
	for bidx := range blocks {
		for ridx := range blocks[bidx].Receipts {
			for lidx := range blocks[bidx].Receipts[ridx].Logs {
				l := blocks[bidx].Receipts[ridx].Logs[lidx]
				xfr, err := erc721.MatchTransfer(&l)
				if err != nil {
					continue
				}
				signer, err := blocks[bidx].Txs[ridx].Signer()
				if err != nil {
					slog.ErrorContext(ctx, "unable to derive signer")
				}
				rows = append(rows, []any{
					e2pg.TaskID(ctx),
					e2pg.ChainID(ctx),
					blocks[bidx].Num(),
					blocks[bidx].Hash(),
					blocks[bidx].Txs[ridx].Hash(),
					ridx,
					lidx,
					signer,
					l.Address.Bytes(),
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
