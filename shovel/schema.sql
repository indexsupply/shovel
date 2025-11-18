create schema if not exists public;
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
	chain_id integer,
	ig_name text,
	num numeric,
	hash bytea,
	insert_at timestamptz default now(),
	src_hash bytea,
	src_num numeric,
	nblocks numeric,
	nrows numeric,
	latency interval,
	src_name text,
	stop numeric
);

create table if not exists shovel.block_verification (
	src_name text not null,
	ig_name text not null,
	block_num numeric not null,
	consensus_hash bytea,
	receipt_hash bytea,
	audit_status text not null default 'pending',
	provider_set jsonb,
	retry_count int not null default 0,
	last_verified_at timestamptz,
	created_at timestamptz not null default now(),
	unique (src_name, ig_name, block_num)
);
create index if not exists block_verification_audit_status_idx
on shovel.block_verification (audit_status, block_num);

create table if not exists shovel.repair_jobs (
	repair_id text primary key,
	src_name text not null,
	ig_name text not null,
	start_block numeric not null,
	end_block numeric not null,
	status text not null default 'in_progress',
	blocks_deleted int not null default 0,
	blocks_reprocessed int not null default 0,
	errors jsonb,
	created_at timestamptz not null default now(),
	completed_at timestamptz
);
create index if not exists repair_jobs_src_ig_created_idx
on shovel.repair_jobs (src_name, ig_name, created_at);

create table if not exists shovel.repair_audit (
	id bigserial primary key,
	repair_id text references shovel.repair_jobs(repair_id),
	requester text,
	src_name text not null,
	ig_name text not null,
	start_block numeric not null,
	end_block numeric not null,
	dry_run boolean not null default false,
	created_at timestamptz not null default now()
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

-- Changes:

alter table shovel.task_updates
add column if not exists chain_id int;

alter table shovel.task_updates
add column if not exists ig_name text;

drop index if exists shovel.task_src_name_num_idx;
drop index if exists shovel.task_src_name_num_idx1;

do $$
begin
if exists (
	select 1
	from information_schema.columns
	where table_schema = 'shovel'
	and table_name = 'task_updates'
	and column_name = 'backfill'
) then
with tasks as (
    select distinct on (src_name)
    src_name, num, hash, chain_id
    from shovel.task_updates
    where backfill = false
    order by src_name, num desc
), igs as (
	select
		distinct on (i.src_name, i.name)
		t.chain_id,
		i.src_name,
		i.name,
		i.num,
		t.hash
	from shovel.ig_updates i
	left outer join tasks t
	on t.src_name = i.src_name
	and t.num = i.num
	where i.backfill = false
	order by i.src_name, i.name, i.num desc
) insert into shovel.task_updates (
	chain_id,
	src_name,
	ig_name,
	num,
	hash
) select chain_id, src_name, name, num, hash from igs;
delete from shovel.task_updates where ig_name is null;
end if;
end
$$;

alter table shovel.task_updates
drop column if exists backfill;

create unique index
if not exists task_src_name_num_idx
on shovel.task_updates
using btree (ig_name, src_name, num DESC);

drop view if exists shovel.source_updates;
create or replace view shovel.source_updates as
select distinct on (src_name)
src_name, num, hash, src_num, src_hash, nblocks, nrows, latency
from shovel.task_updates
order by src_name, num desc;

drop view if exists shovel.latest;
create or replace view shovel.latest as
with abs_latest as (
	select src_name, max(num) num
	from shovel.task_updates
	group by src_name
), src_latest as (
	select
		shovel.task_updates.src_name,
		max(shovel.task_updates.num) num
	from shovel.task_updates, abs_latest
	where shovel.task_updates.src_name = abs_latest.src_name
	and abs_latest.num - shovel.task_updates.num <= 10
	group by shovel.task_updates.src_name, shovel.task_updates.ig_name
)
select src_name, min(num) num from src_latest group by 1;
