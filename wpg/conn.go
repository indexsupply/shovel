package wpg

import (
	"context"
	"fmt"
	"hash/fnv"
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

var (
	lockCollisions    = map[int64]string{}
	lockCollisionsMut sync.Mutex
)

// Uses fnva to compute a hash
// This is an expensive function since it uses a global map
// and a mutex to check if there was a hash collision.
func LockHash(s string) int64 {
	f := fnv.New32a()
	if _, err := f.Write([]byte(s)); err != nil {
		panic(err)
	}
	n := int64(f.Sum32())

	lockCollisionsMut.Lock()
	defer lockCollisionsMut.Unlock()
	if prev, ok := lockCollisions[n]; ok {
		if prev != s {
			panic(fmt.Sprintf("fnva collision: %s %s %d", s, prev, n))
		}
	} else {
		lockCollisions[n] = s
	}
	return n
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
