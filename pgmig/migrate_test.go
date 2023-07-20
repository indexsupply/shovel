package pgmig

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"blake.io/pqx/pqxtest"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"kr.dev/diff"
)

func TestMain(m *testing.M) {
	sql.Register("postgres", stdlib.GetDefaultDriver())
	pqxtest.TestMain(m)
}

func TestMigrateLock(t *testing.T) {
	pqxtest.CreateDB(t, "")
	pg, err := pgxpool.New(context.Background(), pqxtest.DSNForTest(t))
	diff.Test(t, t.Fatalf, err, nil)
	wait := make(chan error)
	go func() {
		wait <- Migrate(pg, Migrations{0: Migration{SQL: "select pg_sleep(5)"}})
	}()
	go func() {
		wait <- Migrate(pg, Migrations{0: Migration{SQL: "select pg_sleep(5)"}})
	}()
	err = <-wait
	if err == nil {
		t.Fatal("expected lock error")
	}
	if !strings.Contains(err.Error(), "locking db for migrations") {
		t.Fatalf("expected lock error. got: %s", err)
	}
}

func TestMigrate(t *testing.T) {
	ctx := context.Background()
	pqxtest.CreateDB(t, "")
	pg, err := pgxpool.New(ctx, pqxtest.DSNForTest(t))
	diff.Test(t, t.Fatalf, err, nil)

	reset := func() {
		if _, err := pg.Exec(ctx, "drop schema public cascade"); err != nil {
			t.Fatalf("dropping schema: %s", err)
		}
		if _, err := pg.Exec(ctx, "create schema public"); err != nil {
			t.Fatalf("dropping schema: %s", err)
		}
	}

	cases := []struct {
		migs     Migrations
		check    string
		errCheck func(error) bool
	}{
		{
			migs: Migrations{
				0: Migration{
					SQL: "create table x(id int)",
				},
			},
			check:    `select true from pg_tables where tablename = 'x'`,
			errCheck: nil,
		},
		{
			migs: Migrations{
				0: Migration{
					SQL: "create table x(id int)",
				},
				1: Migration{
					SQL: "create table x(id int)",
				},
			},
			check: `select true from pg_tables where tablename = 'x'`,
			errCheck: func(err error) bool {
				return strings.Contains(err.Error(), `"x" already exists`)
			},
		},
		{
			migs: Migrations{
				0: Migration{
					SQL: "create table t(x int)",
				},
				1: Migration{
					SQL: "create index concurrently on t(x)",
				},
			},
			errCheck: func(err error) bool {
				msg := `CREATE INDEX CONCURRENTLY cannot run inside a transaction block`
				return strings.Contains(err.Error(), msg)
			},
		},
		{
			migs: Migrations{
				0: Migration{
					SQL: "create table t(x int)",
				},
				1: Migration{
					DisableTX: true,
					SQL:       "create index concurrently on t(x)",
				},
			},
			errCheck: nil,
		},
	}
	for _, tc := range cases {
		reset()
		switch err := Migrate(pg, tc.migs); {
		case tc.errCheck == nil:
			if err != nil {
				t.Errorf("expected err to be nil. got: %s", err)
			}
		default:
			if !tc.errCheck(err) {
				t.Errorf("unexpected error: %s", err)
			}
		}

		if tc.check != "" {
			var check bool
			err = pg.QueryRow(ctx, tc.check).Scan(&check)
			diff.Test(t, t.Fatalf, err, nil)
		}
	}
}
