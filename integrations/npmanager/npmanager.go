package npmanager

import (
	"context"

	"github.com/indexsupply/x/contrib/npmanager"
	"github.com/indexsupply/x/e2pg"

	"github.com/jackc/pgx/v5"
)

type integration struct {
	name string
}

var Integration = integration{
	name: "Nouns Protocol Manager",
}

func (i integration) Delete(pg e2pg.PG, h []byte) error {
	return nil
}

func (i integration) Events() [][]byte {
	return [][]byte{npmanager.DAODeployedSignatureHash}
}

func (i integration) Insert(pg e2pg.PG, blocks []e2pg.Block) (int64, error) {
	var rows = make([][]any, 0, 1<<12)
	for bidx := 0; bidx < len(blocks); bidx++ {
		for ridx := 0; ridx < blocks[bidx].Receipts.Len(); ridx++ {
			r := blocks[bidx].Receipts.At(ridx)
			for lidx := 0; lidx < r.Logs.Len(); lidx++ {
				l := r.Logs.At(lidx)
				event, err := npmanager.MatchDAODeployed(l)
				if err != nil {
					continue
				}
				rows = append(rows, []any{
					blocks[bidx].Num(),
					blocks[bidx].Hash(),
					blocks[bidx].Transactions.At(ridx).Hash(),
					ridx,
					lidx,
					l.Address,
					event.Token[:],
					event.Metadata[:],
					event.Auction[:],
					event.Treasury[:],
					event.Governor[:],
				})
				event.Done()
			}
		}
	}
	return pg.CopyFrom(
		context.Background(),
		pgx.Identifier{"npmanager_dao_deployed"},
		[]string{
			"block_number",
			"block_hash",
			"transaction_hash",
			"transaction_index",
			"log_index",
			"contract",
			"token",
			"metadata",
			"auction",
			"treasury",
			"governor",
		},
		pgx.CopyFromRows(rows),
	)
}
