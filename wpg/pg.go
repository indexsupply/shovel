package wpg

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"blake.io/pqx/pqxtest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:generate go run gen/main.go

func TestPG(tb testing.TB, schema string) *pgxpool.Pool {
	tb.Helper()
	db := pqxtest.CreateDB(tb, schema)

	var name string
	const q = "select current_database()"
	err := db.QueryRow(q).Scan(&name)
	if err != nil {
		tb.Fatal(err)
	}

	cfg, err := pgconn.ParseConfig(pqxtest.DSN())
	if err != nil {
		tb.Fatal(err)
	}

	pgurl := fmt.Sprintf("postgres://localhost:%d/%s", cfg.Port, name)
	pg, err := pgxpool.New(context.Background(), pgurl)
	if err != nil {
		tb.Fatal(err)
	}
	return pg
}

type Column struct {
	Name string `db:"column_name"json:"name"`
	Type string `db:"data_type"json:"type"`
}

func escaped(s string) string {
	_, kw := keywords[strings.ToLower(s)]
	if kw {
		return strconv.Quote(s)
	}

	return s
}

type Table struct {
	Name    string   `json:"name"`
	Columns []Column `json:"columns"`

	DisableUnique bool       `json:"disable_unique"`
	Unique        [][]string `json:"unique"`
	Index         [][]string `json:"index"`
}

func (t Table) DDL() []string {
	if len(t.Columns) == 0 {
		return nil
	}
	var res []string

	createTable := fmt.Sprintf("create table if not exists %s(", t.Name)
	for i, col := range t.Columns {
		createTable += fmt.Sprintf("%s %s", escaped(col.Name), col.Type)
		if i+1 == len(t.Columns) {
			createTable += ")"
			break
		}
		createTable += ", "
	}
	res = append(res, createTable)

	for _, cols := range t.Unique {
		createIndex := fmt.Sprintf(
			"create unique index if not exists u_%s on %s (",
			t.Name,
			t.Name,
		)
		for i, cname := range cols {
			createIndex += escaped(cname)
			if i+1 == len(cols) {
				createIndex += ")"
				break
			}
			createIndex += ", "
		}
		res = append(res, createIndex)
	}

	for _, cols := range t.Index {
		createIndex := fmt.Sprintf(
			"create index if not exists shovel_%s on %s (",
			strings.Join(cols, "_"),
			t.Name,
		)
		for i, cname := range cols {
			createIndex += escaped(cname)
			if i+1 == len(cols) {
				createIndex += ")"
				break
			}
			createIndex += ", "
		}
		res = append(res, createIndex)
	}

	return res
}

func (t Table) Migrate(ctx context.Context, pg Conn) error {
	for _, stmt := range t.DDL() {
		if _, err := pg.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("table %q stmt %q: %w", t.Name, stmt, err)
		}
	}
	diff, err := Diff(ctx, pg, t.Name, t.Columns)
	if err != nil {
		return fmt.Errorf("getting diff for %s: %w", t.Name, err)
	}
	for _, c := range diff.Add {
		var q = fmt.Sprintf(
			"alter table %s add column if not exists %s %s",
			t.Name,
			escaped(c.Name),
			c.Type,
		)
		if _, err := pg.Exec(ctx, q); err != nil {
			return fmt.Errorf("adding column %s/%s: %w", t.Name, c.Name, err)
		}
	}
	return nil
}

type DiffDetails struct {
	Remove []Column
	Add    []Column
}

func Diff(
	ctx context.Context,
	pg Conn,
	tableName string,
	cols []Column,
) (DiffDetails, error) {
	const q = `
		select column_name, data_type
		from information_schema.columns
		where table_schema = 'public'
		and table_name = $1
	`
	rows, _ := pg.Query(ctx, q, tableName)
	indb, err := pgx.CollectRows(rows, pgx.RowToStructByName[Column])
	if err != nil {
		return DiffDetails{}, fmt.Errorf("querying for table info: %w", err)
	}
	var dd DiffDetails
	for i := range cols {
		var found bool
		for j := range indb {
			if cols[i].Name == indb[j].Name {
				found = true
				break
			}
		}
		if !found {
			dd.Add = append(dd.Add, cols[i])
		}
	}
	for i := range indb {
		var found bool
		for j := range cols {
			if indb[i].Name == cols[j].Name {
				found = true
				break
			}
		}
		if !found {
			dd.Remove = append(dd.Remove, indb[i])
		}
	}
	return dd, nil
}

func Indexes(ctx context.Context, pg Conn, table string) []map[string]any {
	const q = `
		select indexname, indexdef
		from pg_indexes
		where tablename = $1
	`
	rows, _ := pg.Query(ctx, q, table)
	res, err := pgx.CollectRows(rows, pgx.RowToMap)
	if err != nil {
		return []map[string]any{map[string]any{"error": err.Error()}}
	}
	return res
}

func RowEstimate(ctx context.Context, pg Conn, table string) string {
	const q = `
		select trim(to_char(reltuples, '999,999,999,999'))
		from pg_class
		where relname = $1
	`
	var res string
	if err := pg.QueryRow(ctx, q, table).Scan(&res); err != nil {
		return err.Error()
	}
	switch {
	case res == "0":
		return "pending"
	case strings.HasPrefix(res, "-"):
		return "pending"
	default:
		return res
	}
}

func TableSize(ctx context.Context, pg Conn, table string) string {
	const q = `SELECT pg_size_pretty(pg_total_relation_size($1))`
	var res string
	if err := pg.QueryRow(ctx, q, table).Scan(&res); err != nil {
		return err.Error()
	}
	return res
}
