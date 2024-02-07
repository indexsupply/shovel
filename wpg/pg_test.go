package wpg

import (
	"context"
	"database/sql"
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

func TestDDL(t *testing.T) {
	cases := []struct {
		table Table
		want  []string
	}{
		{
			Table{},
			nil,
		},
		{
			Table{
				Name: "foo",
				Columns: []Column{
					{Name: "a", Type: "int"},
					{Name: "b", Type: "int"},
				},
				Unique: [][]string{{"a", "b"}},
			},
			[]string{
				"create table if not exists foo(a int, b int)",
				"create unique index if not exists u_foo on foo (a, b)",
			},
		},
	}
	for _, tc := range cases {
		diff.Test(t, t.Errorf, tc.table.DDL(), tc.want)
	}
}

func TestMigrate(t *testing.T) {
	cases := []struct {
		old    Table
		new    Table
		before DiffDetails
		after  DiffDetails
	}{
		{
			Table{},
			Table{
				Name:    "x",
				Columns: []Column{{Name: "x", Type: "integer"}, {Name: "from", Type: "integer"}},
			},
			DiffDetails{
				Add: []Column{{Name: "x", Type: "integer"}, {Name: "from", Type: "integer"}},
			},
			DiffDetails{},
		},
	}
	ctx := context.Background()
	pqxtest.CreateDB(t, "")
	pg, err := pgxpool.New(ctx, pqxtest.DSNForTest(t))
	diff.Test(t, t.Fatalf, nil, err)

	for _, tc := range cases {
		tc.old.Migrate(ctx, pg)
		before, err := Diff(ctx, pg, tc.new.Name, tc.new.Columns)
		diff.Test(t, t.Fatalf, nil, err)
		diff.Test(t, t.Fatalf, tc.before, before)

		tc.new.Migrate(ctx, pg)
		after, err := Diff(ctx, pg, tc.new.Name, tc.new.Columns)
		diff.Test(t, t.Fatalf, nil, err)
		diff.Test(t, t.Fatalf, tc.after, after)

		_, err = pg.Exec(ctx, "drop schema public cascade; create schema public;")
		diff.Test(t, t.Errorf, nil, err)
	}
}

func TestDiff(t *testing.T) {
	cases := []struct {
		table Table
		input []Column
		want  DiffDetails
		err   error
	}{
		{
			Table{Name: "x", Columns: []Column{{Name: "x", Type: "integer"}}},
			[]Column{{Name: "x", Type: "integer"}},
			DiffDetails{},
			nil,
		},
		{
			Table{Name: "x", Columns: []Column{
				{Name: "x", Type: "integer"},
				{Name: "y", Type: "integer"},
			}},
			[]Column{
				{Name: "x", Type: "integer"},
			},
			DiffDetails{
				Remove: []Column{
					{Name: "y", Type: "integer"},
				},
			},
			nil,
		},
		{
			Table{Name: "x", Columns: []Column{
				{Name: "x", Type: "integer"},
			}},
			[]Column{
				{Name: "x", Type: "integer"},
				{Name: "y", Type: "integer"},
			},
			DiffDetails{
				Add: []Column{
					{Name: "y", Type: "integer"},
				},
			},
			nil,
		},
	}
	ctx := context.Background()
	pqxtest.CreateDB(t, "")
	pg, err := pgxpool.New(ctx, pqxtest.DSNForTest(t))
	diff.Test(t, t.Fatalf, nil, err)

	for _, tc := range cases {
		diff.Test(t, t.Fatalf, nil, tc.table.Migrate(ctx, pg))
		got, err := Diff(context.Background(), pg, tc.table.Name, tc.input)
		diff.Test(t, t.Errorf, nil, err)
		diff.Test(t, t.Errorf, tc.want, got)
		_, err = pg.Exec(ctx, "drop schema public cascade; create schema public;")
		diff.Test(t, t.Errorf, nil, err)
	}
}
