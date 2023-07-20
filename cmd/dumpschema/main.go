package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/indexsupply/x/e2pg"
	"github.com/indexsupply/x/pgmig"

	"github.com/jackc/pgx/v5/pgxpool"
)

func check(err error) {
	if err != nil {
		panic(err)
	}
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

const tmpdb = `dumpschema`

func main() {
	ctx := context.Background()

	check(run("dropdb", "--if-exists", tmpdb))
	check(run("createdb", tmpdb))

	pgp, err := pgxpool.New(ctx, fmt.Sprintf("postgres:///%s", tmpdb))
	check(err)
	_, err = pgp.Exec(ctx, fmt.Sprintf(`CREATE SCHEMA IF NOT EXISTS %s`, "public"))
	check(err)
	_, err = pgp.Exec(ctx, fmt.Sprintf(`ALTER DATABASE %s SET TIMEZONE TO 'UTC'`, tmpdb))
	check(err)

	check(pgmig.Migrate(pgp, e2pg.Migrations))

	var buf bytes.Buffer
	pgdump := exec.Command("pg_dump", "-sOx", tmpdb)
	pgdump.Stdout = &buf
	pgdump.Stderr = os.Stderr
	check(pgdump.Run())

	f, err := os.Create(filepath.Join("e2pg", "schema.sql"))
	check(err)
	defer f.Close()

	for _, line := range strings.Split(buf.String(), "\n") {
		if strings.HasPrefix(line, "--") || strings.Contains(line, "COMMENT") {
			continue
		}
		_, err = f.WriteString(line + "\n")
		check(err)
	}
	pgp.Close()
	check(run("dropdb", tmpdb))
}
