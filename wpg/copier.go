package wpg

import (
	"context"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Useful for connection or transaction implementations
type Limited interface {
	CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error)
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func NewTxLocker(tx pgx.Tx) *TxLocker {
	return &TxLocker{tx: tx}
}

type TxLocker struct {
	sync.Mutex
	tx pgx.Tx
}

func (tl *TxLocker) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	tl.Lock()
	defer tl.Unlock()
	return tl.tx.QueryRow(ctx, sql, args...)
}

func (tl *TxLocker) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	tl.Lock()
	defer tl.Unlock()
	return tl.tx.Exec(ctx, sql, args...)
}

func (tl *TxLocker) CopyFrom(ctx context.Context, table pgx.Identifier, cols []string, src pgx.CopyFromSource) (int64, error) {
	tl.Lock()
	defer tl.Unlock()
	return tl.tx.CopyFrom(ctx, table, cols, src)
}
