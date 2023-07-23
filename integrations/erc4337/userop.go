package erc4337

import (
	"context"

	"github.com/indexsupply/x/contrib/erc4337"
	"github.com/indexsupply/x/e2pg"

	"github.com/jackc/pgx/v5"
)

type integration struct {
	name string
}

var Integration = integration{
	name: "ERC4337 UserOperationEvent",
}

func (i integration) Events(ctx context.Context) [][]byte {
	return [][]byte{erc4337.UserOperationEventSignatureHash}
}

func (i integration) Delete(ctx context.Context, pg e2pg.PG, h []byte) error {
	const q = `
		delete from erc4337_userops
		where task_id = $1
		and chain_id = $2
		and block_hash = $3
	`
	_, err := pg.Exec(ctx, q, e2pg.TaskID(ctx), e2pg.ChainID(ctx), h)
	return err
}

func (i integration) Insert(ctx context.Context, pg e2pg.PG, blocks []e2pg.Block) (int64, error) {
	var rows = make([][]any, 0, 1<<12)
	for bidx := 0; bidx < len(blocks); bidx++ {
		for ridx := 0; ridx < blocks[bidx].Receipts.Len(); ridx++ {
			r := blocks[bidx].Receipts.At(ridx)
			for lidx := 0; lidx < r.Logs.Len(); lidx++ {
				l := r.Logs.At(lidx)
				event, err := erc4337.MatchUserOperationEvent(l)
				if err != nil {
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

					event.UserOpHash[:],
					event.Sender[:],
					event.Paymaster[:],
					event.Nonce.String(),
					event.Success,
					event.ActualGasCost.String(),
					event.ActualGasUsed.String(),
				})
				event.Done()
			}
		}
	}
	return pg.CopyFrom(
		context.Background(),
		pgx.Identifier{"erc4337_userops"},
		[]string{
			"task_id",
			"chain_id",
			"block_number",
			"block_hash",
			"transaction_hash",
			"transaction_index",
			"log_index",
			"contract",

			"op_hash",
			"op_sender",
			"op_paymaster",
			"op_nonce",
			"op_success",
			"op_actual_gas_cost",
			"op_actual_gas_used",
		},
		pgx.CopyFromRows(rows),
	)
}
