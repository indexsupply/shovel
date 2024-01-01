create schema if not exists shovel;

create table if not exists shovel.integrations (
	name text,
	conf jsonb
);
create table if not exists shovel.ig_updates (
	name text not null,
	src_name text not null,
	backfill boolean default false,
	num numeric not null,
	latency interval,
	nrows numeric,
	stop numeric
);
create table if not exists shovel.sources (
	name text,
	chain_id integer,
	url text
);
create table if not exists shovel.task_updates (
	num numeric,
	hash bytea,
	insert_at timestamptz default now(),
	src_hash bytea,
	src_num numeric,
	nblocks numeric,
	nrows numeric,
	latency interval,
	backfill boolean default false,
	src_name text,
	stop numeric
);
create unique index
if not exists intg_name_src_name_backfill_num_idx
on shovel.ig_updates
using btree (name, src_name, backfill, num desc);

create unique index
if not exists sources_name_chain_id_idx
on shovel.sources
using btree (name, chain_id);

create unique index
if not exists sources_name_idx
on shovel.sources
using btree (name);

create unique index
if not exists task_src_name_num_idx
on shovel.task_updates
using btree (src_name, num DESC)
where (backfill = true);

create unique index
if not exists task_src_name_num_idx1
on shovel.task_updates
using btree (src_name, num DESC)
where (backfill = false);

-- Changes:

alter table shovel.task_updates
add column if not exists chain_id int;
