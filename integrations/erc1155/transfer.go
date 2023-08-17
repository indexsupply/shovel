package erc1155

import (
	"context"
	"log/slog"

	erc1155abi "github.com/indexsupply/x/contrib/erc1155"
	"github.com/indexsupply/x/e2pg"

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

func (i integration) Insert(ctx context.Context, pg e2pg.PG, blocks []e2pg.Block) (int64, error) {
	var rows = make([][]any, 0, 1<<12)
	for bidx := 0; bidx < len(blocks); bidx++ {
		for ridx := 0; ridx < blocks[bidx].Receipts.Len(); ridx++ {
			r := blocks[bidx].Receipts.At(ridx)
			t := blocks[bidx].Transactions.At(ridx)
			for lidx := 0; lidx < r.Logs.Len(); lidx++ {
				l := r.Logs.At(lidx)
				signer, err := blocks[bidx].Transactions.At(ridx).Signer()
				if err != nil {
					slog.ErrorContext(ctx, "unable to derive signer")
				}
				xfrb, errb := erc1155abi.MatchTransferBatch(l)
				xfrs, errs := erc1155abi.MatchTransferSingle(l)
				switch {
				case errb == nil:
					if len(xfrb.Ids) != len(xfrb.Values) {
						continue
					}
					for i := 0; i < len(xfrb.Ids); i++ {
						rows = append(rows, []any{
							e2pg.TaskID(ctx),
							e2pg.ChainID(ctx),
							blocks[bidx].Num(),
							blocks[bidx].Hash(),
							t.Hash(),
							ridx,
							lidx,
							signer,
							l.Address,
							xfrb.Ids[i].String(),
							xfrb.Values[i].String(),
							xfrb.From[:],
							xfrb.To[:],
						})
					}
					xfrb.Done()
				case errs == nil:
					rows = append(rows, []any{
						e2pg.TaskID(ctx),
						e2pg.ChainID(ctx),
						blocks[bidx].Num(),
						blocks[bidx].Hash(),
						t.Hash(),
						ridx,
						lidx,
						signer,
						l.Address,
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
