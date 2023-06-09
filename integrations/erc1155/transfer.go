package erc1155

import (
	"context"

	"github.com/indexsupply/x/bloom"
	erc1155abi "github.com/indexsupply/x/contrib/erc1155"
	"github.com/indexsupply/x/g2pg"

	"github.com/jackc/pgx/v5"
)

type integration struct {
	name string
}

var Integration = integration{
	name: "ERC1155 Transfer",
}

func (i integration) Delete(pg g2pg.PG, h []byte) error {
	return nil
}

func (i integration) Skip(bf bloom.Filter) bool {
	return bf.Missing(erc1155abi.TransferBatchSignatureHash) &&
		bf.Missing(erc1155abi.TransferSingleSignatureHash)
}

func (i integration) Insert(pg g2pg.PG, blocks []g2pg.Block) (int64, error) {
	var rows = make([][]any, 0, 1<<12)
	for bidx := 0; bidx < len(blocks); bidx++ {
		for ridx := 0; ridx < blocks[bidx].Receipts.Len(); ridx++ {
			r := blocks[bidx].Receipts.At(ridx)
			t := blocks[bidx].Transactions.At(ridx)
			for lidx := 0; lidx < r.Logs.Len(); lidx++ {
				l := r.Logs.At(lidx)
				xfrb, errb := erc1155abi.MatchTransferBatch(l)
				xfrs, errs := erc1155abi.MatchTransferSingle(l)
				switch {
				case errb == nil:
					if len(xfrb.Ids) != len(xfrb.Values) {
						continue
					}
					for i := 0; i < len(xfrb.Ids); i++ {
						rows = append(rows, []any{
							blocks[bidx].Header.Number,
							blocks[bidx].Hash,
							t.Hash(),
							ridx,
							lidx,
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
						blocks[bidx].Header.Number,
						blocks[bidx].Hash,
						t.Hash(),
						ridx,
						lidx,
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
			"block_number",
			"block_hash",
			"transaction_hash",
			"transaction_index",
			"log_index",
			"contract",
			"token_id",
			"quantity",
			"f",
			"t",
		},
		pgx.CopyFromRows(rows),
	)
}
