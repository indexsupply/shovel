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
			alter table e2pg.task rename column number to num;
			alter table e2pg.task add column src_hash bytea;
			alter table e2pg.task add column src_num numeric;
			alter table e2pg.task add column nblocks numeric;
			alter table e2pg.task add column nrows numeric;
			alter table e2pg.task add column latency interval;
			alter table e2pg.task add column dstat jsonb;
		`,
	},
	11: pgmig.Migration{
		SQL: `
			delete from e2pg.task where insert_at < now() - '2 hours'::interval;
			alter table e2pg.task add column backfill bool default false;
			alter table e2pg.task add column src_name text;
			update e2pg.task set src_name = split_part(id, '-', 1);
			alter table e2pg.task drop column id;
			create unique index on e2pg.task(src_name, num desc) where backfill = true;
			create unique index on e2pg.task(src_name, num desc) where backfill = false;
		`,
	},
	12: pgmig.Migration{
		SQL: `
			create table e2pg.intg (
				name text not null,
				src_name text not null,
				backfill bool default false,
				num numeric not null,
				latency interval,
				nrows numeric
			);
			create unique index on e2pg.intg(name, src_name, num desc) where backfill;
			create unique index on e2pg.intg(name, src_name, num desc) where not backfill;
		`,
	},
	13: pgmig.Migration{
		SQL: `
			drop index e2pg.intg_name_src_name_num_idx;
			drop index e2pg.intg_name_src_name_num_idx1;
			create unique index on e2pg.intg(name, src_name, backfill, num desc);
		`,
	},
	14: pgmig.Migration{
		SQL: `
			alter table e2pg.intg add column stop numeric;
			alter table e2pg.task add column stop numeric;
		`,
	},
}
