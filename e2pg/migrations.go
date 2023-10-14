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
	9: pgmig.Migration{
		SQL: `
			create table e2pg.sources(name text, chain_id int, url text);
			create unique index on e2pg.sources(name, chain_id);

			create table e2pg.integrations(name text, conf jsonb);
			create unique index on e2pg.sources(name);
		`,
	},
	10: pgmig.Migration{
		SQL: `
			alter table e2pg.task add column src_hash bytea;
			alter table e2pg.task add column src_num numeric;
			alter table e2pg.task add column nblocks numeric;
			alter table e2pg.task add column nrows numeric;
			alter table e2pg.task add column latency interval;
			alter table e2pg.task add column dstat jsonb;
		`,
	},
}
