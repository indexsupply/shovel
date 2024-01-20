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
