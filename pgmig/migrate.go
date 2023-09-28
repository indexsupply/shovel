// migration system for E2PG
package pgmig

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Migration struct {
	SQL string
	// Some types of DDL cannot run inside a transaction.
	// EG CREATE INDEX CONCURRENTLY
	// For these cases callers should disable transactions
	DisableTX bool
}

func normalizeSQL(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func (m Migration) Hash() []byte {
	s := normalizeSQL(m.SQL)
	h := sha256.Sum256([]byte(s))
	return h[:]
}

type execer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type Migrations map[int]Migration

// Runs the set of migrations (migs) against the database (pgp)
//
// migs is a map because the keys represent the order of migrations
// and so they should be unique. They keys are written to the idx
// columne of the e2pg.migrations table.
//
// The e2pg.migrations table will be created if it doesn't
// already exist.
//
// Migrate will use pg_try_advisory_lock to ensure that only
// one process is migrating the database at a time. An error
// is returned if another process has the lock.
func Migrate(pgp *pgxpool.Pool, migs Migrations) error {
	ctx := context.Background()
	// crc32(migrate) == -1924339791
	var locked bool
	err := pgp.QueryRow(ctx, `select pg_try_advisory_lock(-1924339791)`).Scan(&locked)
	if err != nil || !locked {
		return fmt.Errorf("locking db for migrations: %w", err)
	}
	const q1 = `create schema if not exists e2pg`
	_, err = pgp.Exec(ctx, q1)
	if err != nil {
		return fmt.Errorf("creating e2pg schema: %w", err)
	}
	const q2 = `
		create table if not exists e2pg.migrations (
			idx int not null,
			hash bytea not null,
			inserted_at timestamptz default now() not null,
			primary key (idx, hash)
		);
	`
	_, err = pgp.Exec(ctx, q2)
	if err != nil {
		return fmt.Errorf("creating migrations table: %w", err)
	}

	var keys []int
	for k, _ := range migs {
		keys = append(keys, int(k))
	}
	sort.Ints(keys)

	for _, i := range keys {
		var (
			db   execer = pgp
			dbtx pgx.Tx
		)
		if !migs[i].DisableTX {
			dbtx, err = pgp.Begin(ctx)
			if err != nil {
				return fmt.Errorf("opening a tx: %w", err)
			}
			defer dbtx.Rollback(ctx)
			db = dbtx
		}
		ok, err := exists(ctx, db, i, migs[i])
		if err != nil {
			return fmt.Errorf("checking migration existence: %w", err)
		}
		if ok {
			continue
		}
		err = migrate(ctx, db, i, migs[i])
		if err != nil {
			return fmt.Errorf("running migration: %w", err)
		}
		if !migs[i].DisableTX {
			err = dbtx.Commit(ctx)
			if err != nil {
				return fmt.Errorf("commiting migration tx: %w", err)
			}
		}
		h := migs[i].Hash()
		fmt.Printf("migrated %d %x\n", i, h[:4])
	}
	return nil
}

func migrate(ctx context.Context, db execer, i int, m Migration) error {
	_, err := db.Exec(ctx, m.SQL)
	if err != nil {
		return fmt.Errorf("migration %d %x exec error: %w", i, m.Hash(), err)
	}
	const q = `insert into e2pg.migrations(idx, hash) values ($1, $2)`
	_, err = db.Exec(ctx, q, i, m.Hash())
	if err != nil {
		return fmt.Errorf("migrations table %d %x insert error: %w", i, m.Hash(), err)
	}
	return nil
}

func exists(ctx context.Context, db execer, i int, m Migration) (bool, error) {
	const q = `select true from e2pg.migrations where idx = $1 and hash = $2`
	var found bool
	err := db.QueryRow(ctx, q, i, m.Hash()).Scan(&found)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("querying for existing migration: %w", err)
	}
	return found, nil
}
