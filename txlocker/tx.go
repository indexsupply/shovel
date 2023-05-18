// wrap pgx.Tx with a mutex
package txlocker

import (
	"context"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func NewTx(tx pgx.Tx) *Tx {
	return &Tx{tx: tx}
}

type Tx struct {
	sync.Mutex
	tx pgx.Tx
}

func (t *Tx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	t.Lock()
	defer t.Unlock()
	return t.tx.QueryRow(ctx, sql, args...)
}

func (t *Tx) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	t.Lock()
	defer t.Unlock()
	return t.tx.Exec(ctx, sql, args...)
}

func (t *Tx) CopyFrom(ctx context.Context, table pgx.Identifier, cols []string, source pgx.CopyFromSource) (int64, error) {
	t.Lock()
	defer t.Unlock()
	return t.tx.CopyFrom(ctx, table, cols, source)
}
