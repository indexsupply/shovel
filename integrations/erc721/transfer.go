package erc721

import (
	"context"

	"github.com/indexsupply/x/bloom"
	"github.com/indexsupply/x/contrib/erc721"
	"github.com/indexsupply/x/g2pg"

	"github.com/jackc/pgx/v5"
)

type integration struct {
	name string
}

var Integration = integration{
	name: "ERC721 Transfer",
}

func (i integration) Delete(pg g2pg.PG, h []byte) error {
	return nil
}

func (i integration) Skip(bf bloom.Filter) bool {
	return bf.Missing(erc721.TransferSignatureHash)
}

func (i integration) Insert(pg g2pg.PG, blocks []g2pg.Block) (int64, error) {
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
				rows = append(rows, []any{
					blocks[bidx].Header.Number,
					blocks[bidx].Hash,
					blocks[bidx].Transactions.At(ridx).Hash(),
					ridx,
					lidx,
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
			"block_number",
			"block_hash",
			"transaction_hash",
			"transaction_index",
			"log_index",
			"contract",
			"token_id",
			"f",
			"t",
		},
		pgx.CopyFromRows(rows),
	)
}
