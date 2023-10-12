package wpg

import (
	"context"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type Conn interface {
	CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error)
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func NewTxLocker(tx pgx.Tx) *TxLocker {
	return &TxLocker{tx: tx}
}

type TxLocker struct {
	sync.Mutex
	tx pgx.Tx
}

func (t *TxLocker) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	t.Lock()
	defer t.Unlock()
	return t.tx.QueryRow(ctx, sql, args...)
}

func (t *TxLocker) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	t.Lock()
	defer t.Unlock()
	return t.tx.Query(ctx, sql, args...)
}

func (t *TxLocker) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	t.Lock()
	defer t.Unlock()
	return t.tx.Exec(ctx, sql, args...)
}

func (t *TxLocker) CopyFrom(ctx context.Context, table pgx.Identifier, cols []string, source pgx.CopyFromSource) (int64, error) {
	t.Lock()
	defer t.Unlock()
	return t.tx.CopyFrom(ctx, table, cols, source)
}
