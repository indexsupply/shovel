package txinputs

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"

	"github.com/indexsupply/x/e2pg"
	"github.com/indexsupply/x/eth"

	"github.com/jackc/pgx/v5"
)

var filterFrom, filterTo []byte

func init() {
	if f := os.Getenv("TX_FROM"); len(f) > 0 {
		b, err := hex.DecodeString(f)
		if err != nil {
			fmt.Println("Unable to hex decode $TX_FROM")
			os.Exit(1)
		}
		filterFrom = b
	}
	if t := os.Getenv("TX_TO"); len(t) > 0 {
		b, err := hex.DecodeString(t)
		if err != nil {
			fmt.Println("Unable to hex decode $TX_TO")
			os.Exit(1)
		}
		filterTo = b
	}
}

type integration struct {
	name string
}

var Integration = integration{
	name: "Tx Inputs",
}

func (i integration) Events(ctx context.Context) [][]byte { return [][]byte{} }

func (i integration) Delete(ctx context.Context, pg e2pg.PG, n uint64) error {
	const q = `
		delete from tx_inputs
		where task_id = $1
		and chain_id = $2
		and block_number >= $3
	`
	_, err := pg.Exec(ctx, q, e2pg.TaskID(ctx), e2pg.ChainID(ctx), n)
	return err
}

func (i integration) Insert(ctx context.Context, pg e2pg.PG, blocks []eth.Block) (int64, error) {
	var rows = make([][]any, 0, 1<<8)
	for bidx := range blocks {
		for tidx := range blocks[bidx].Txs {
			signer, err := blocks[bidx].Txs[tidx].Signer()
			if err != nil {
				slog.ErrorContext(ctx, "unable to deriver signer", err)
				continue
			}
			if len(filterFrom) > 0 && !bytes.Equal(filterFrom, signer) {
				continue
			}
			if len(filterTo) > 0 && !bytes.Equal(filterTo, blocks[bidx].Txs[tidx].To) {
				continue
			}
			rows = append(rows, []any{
				e2pg.TaskID(ctx),
				e2pg.ChainID(ctx),
				blocks[bidx].Num(),
				blocks[bidx].Hash(),
				blocks[bidx].Txs[tidx].Hash(),
				tidx,
				signer,
				blocks[bidx].Txs[tidx].To.Bytes(),
				blocks[bidx].Txs[tidx].Data.Bytes(),
			})
		}
	}
	return pg.CopyFrom(ctx, pgx.Identifier{"tx_inputs"}, []string{
		"task_id",
		"chain_id",
		"block_number",
		"block_hash",
		"tx_hash",
		"tx_index",
		"tx_signer",
		"tx_to",
		"tx_input",
	}, pgx.CopyFromRows(rows))
}
