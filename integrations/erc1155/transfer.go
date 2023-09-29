package erc1155

import (
	"context"
	"log/slog"

	erc1155abi "github.com/indexsupply/x/contrib/erc1155"
	"github.com/indexsupply/x/e2pg"
	"github.com/indexsupply/x/eth"

	"github.com/jackc/pgx/v5"
)

type integration struct {
	name string
}

var Integration = integration{
	name: "ERC1155 Transfer",
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
	return [][]byte{
		erc1155abi.TransferBatchSignatureHash,
		erc1155abi.TransferSingleSignatureHash,
	}
}

func (i integration) Insert(ctx context.Context, pg e2pg.PG, blocks []eth.Block) (int64, error) {
	var rows = make([][]any, 0, 1<<12)
	for bidx := range blocks {
		for ridx := range blocks[bidx].Receipts {
			for lidx := range blocks[bidx].Receipts[ridx].Logs {
				l := blocks[bidx].Receipts[ridx].Logs[lidx]
				xfrb, errb := erc1155abi.MatchTransferBatch(&l)
				xfrs, errs := erc1155abi.MatchTransferSingle(&l)
				switch {
				case errb == nil:
					if len(xfrb.Ids) != len(xfrb.Values) {
						continue
					}
					signer, err := blocks[bidx].Txs[ridx].Signer()
					if err != nil {
						slog.ErrorContext(ctx, "unable to derive signer")
					}
					for i := 0; i < len(xfrb.Ids); i++ {
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
							xfrb.Ids[i].String(),
							xfrb.Values[i].String(),
							xfrb.From[:],
							xfrb.To[:],
						})
					}
					xfrb.Done()
				case errs == nil:
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
						xfrs.Id.String(),
						xfrs.Value.String(),
						xfrs.From[:],
						xfrs.To[:],
					})
					xfrs.Done()
				default:
					continue
				}
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
			"quantity",
			"f",
			"t",
		},
		pgx.CopyFromRows(rows),
	)
}
