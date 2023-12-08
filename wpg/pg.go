package wpg

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"blake.io/pqx/pqxtest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

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

type Table struct {
	Name    string   `json:"name"`
	Columns []Column `json:"columns"`

	DisableUnique bool       `json:"disable_unique"`
	Unique        [][]string `json:"unique"`
}

func CreateTable(ctx context.Context, pg Conn, t Table) error {
	if len(t.Columns) == 0 {
		return fmt.Errorf("no columns to add")
	}
	var s strings.Builder
	s.WriteString(fmt.Sprintf("create table if not exists %s(", t.Name))
	for i, col := range t.Columns {
		s.WriteString(fmt.Sprintf("%s %s", col.Name, col.Type))
		if i+1 == len(t.Columns) {
			s.WriteString(")")
			break
		}
		s.WriteString(",")
	}
	if _, err := pg.Exec(ctx, s.String()); err != nil {
		return fmt.Errorf("creating table for %s: %w", t.Name, err)
	}
	diff, err := Diff(ctx, pg, t.Name, t.Columns)
	if err != nil {
		return fmt.Errorf("getting diff for %s: %w", t.Name, err)
	}
	for _, c := range diff.Add {
		s.Reset()
		const q = "alter table %s add column %s %s"
		s.WriteString(fmt.Sprintf(q, t.Name, c.Name, c.Type))
		slog.InfoContext(ctx, "create-table-diff", "add-column", s.String())
		if _, err := pg.Exec(ctx, s.String()); err != nil {
			return fmt.Errorf("adding column %s/%s: %w", t.Name, c.Name, err)
		}
	}
	return nil
}

func Rename(ctx context.Context, pg Conn, t Table) error {
	const q = `
		DO $$
		BEGIN
			ALTER TABLE %s RENAME COLUMN intg_name TO ig_name;
		EXCEPTION
			WHEN undefined_column THEN RAISE NOTICE 'column intg_name does not exist';
		END; $$;
	`
	if _, err := pg.Exec(ctx, fmt.Sprintf(q, t.Name)); err != nil {
		return fmt.Errorf("updating intg_name col on %s: %w", t.Name, err)
	}
	return nil
}

func CreateUIDX(ctx context.Context, pg Conn, t Table) error {
	if t.DisableUnique {
		slog.InfoContext(ctx, "disable unique index", "table", t.Name)
		return nil
	}
	var s strings.Builder
	for _, cols := range t.Unique {
		s.Reset()
		const q = "create unique index if not exists u_%s on %s ("
		s.WriteString(fmt.Sprintf(q, t.Name, t.Name))
		for i, cname := range cols {
			s.WriteString(cname)
			if i+1 == len(cols) {
				s.WriteString(")")
				break
			}
			s.WriteString(",")
		}
		if _, err := pg.Exec(ctx, s.String()); err != nil {
			return fmt.Errorf("creating index %s: %w", t.Name, err)
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
