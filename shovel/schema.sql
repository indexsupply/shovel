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

drop view if exists public.latest_blocks  ;
create or replace view public.latest_blocks   
with (security_invoker=on)
as
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

BEGIN;

-- Create latest_blocks table in shovel schema
CREATE TABLE IF NOT EXISTS shovel.latest_blocks (
    chain_id INT NOT NULL,
    src_name TEXT NOT NULL,
    block_number NUMERIC NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (chain_id, src_name)
);

-- Create an index for faster lookups
CREATE INDEX IF NOT EXISTS idx_latest_blocks_src_name ON shovel.latest_blocks (src_name);

-- Create function to update latest_blocks
CREATE OR REPLACE FUNCTION shovel.update_latest_blocks()
RETURNS TRIGGER AS $$
BEGIN
    INSERT INTO shovel.latest_blocks (chain_id, src_name, block_number)
    VALUES (NEW.chain_id, NEW.src_name, NEW.num)
    ON CONFLICT (chain_id, src_name)
    DO UPDATE SET
        block_number = GREATEST(shovel.latest_blocks.block_number, EXCLUDED.block_number),
        updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Create trigger to update latest_blocks
DROP TRIGGER IF EXISTS trigger_update_latest_blocks ON shovel.task_updates;
CREATE TRIGGER trigger_update_latest_blocks
AFTER INSERT OR UPDATE ON shovel.task_updates
FOR EACH ROW
EXECUTE FUNCTION shovel.update_latest_blocks();

-- Commit transaction
COMMIT;

GRANT USAGE ON SCHEMA shovel TO service_role;

GRANT SELECT ON ALL TABLES IN SCHEMA shovel TO service_role;
