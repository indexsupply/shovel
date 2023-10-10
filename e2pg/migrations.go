package e2pg

import "github.com/indexsupply/x/pgmig"

var Migrations = map[int]pgmig.Migration{
	8: pgmig.Migration{
		SQL: `
			create table e2pg.task (
				id text not null,
				number bigint,
				hash bytea,
				insert_at timestamptz default now()
			);
			create index on e2pg.task(id, number desc);
		`,
	},
}
