package e2pg

import (
	"context"
	"strings"
	"testing"

	"blake.io/pqx/pqxtest"
	"github.com/indexsupply/x/wpg"
	"github.com/jackc/pgx/v5/pgxpool"
	"kr.dev/diff"
)

func TestPruneIntg(t *testing.T) {
	ctx := context.Background()

	pqxtest.CreateDB(t, Schema)
	pg, err := pgxpool.New(ctx, pqxtest.DSNForTest(t))
	diff.Test(t, t.Fatalf, err, nil)

	iub := newIUB(1)
	iub.updates[0].Name = "foo"
	iub.updates[0].SrcName = "bar"
	iub.updates[0].Num = 1

	err = iub.write(ctx, pg)
	diff.Test(t, t.Fatalf, err, nil)
	checkQuery(t, pg, `select count(*) = 1 from e2pg.intg`)

	err = iub.write(ctx, pg)
	if err == nil || !strings.Contains(err.Error(), "intg_name_src_name_num_idx1") {
		t.Errorf("expected 2nd write to return unique error")
	}

	for i := 0; i < 10; i++ {
		iub.updates[0].Num = uint64(i + 2)
		err := iub.write(ctx, pg)
		diff.Test(t, t.Fatalf, err, nil)
	}
	checkQuery(t, pg, `select count(*) = 11 from e2pg.intg`)
	err = PruneIntg(ctx, pg, 10)
	diff.Test(t, t.Fatalf, err, nil)
	checkQuery(t, pg, `select count(*) = 10 from e2pg.intg`)

	iub.updates[0].Name = "foo"
	iub.updates[0].SrcName = "baz"
	iub.updates[0].Num = 1
	err = iub.write(ctx, pg)
	diff.Test(t, t.Fatalf, err, nil)
	checkQuery(t, pg, `select count(*) = 1 from e2pg.intg where src_name = 'baz'`)
	checkQuery(t, pg, `select count(*) = 11 from e2pg.intg`)

	err = PruneIntg(ctx, pg, 10)
	diff.Test(t, t.Fatalf, err, nil)
	checkQuery(t, pg, `select count(*) = 11 from e2pg.intg`)
}

func checkQuery(tb testing.TB, pg wpg.Conn, query string) {
	var found bool
	err := pg.QueryRow(context.Background(), query).Scan(&found)
	diff.Test(tb, tb.Fatalf, err, nil)
	if !found {
		tb.Errorf("query\n%s\nreturned false", query)
	}
}
