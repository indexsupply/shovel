

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;


CREATE SCHEMA shovel;


SET default_tablespace = '';

SET default_table_access_method = heap;


CREATE TABLE shovel.ig_updates (
    name text NOT NULL,
    src_name text NOT NULL,
    backfill boolean DEFAULT false,
    num numeric NOT NULL,
    latency interval,
    nrows numeric,
    stop numeric
);



CREATE TABLE shovel.integrations (
    name text,
    conf jsonb
);



CREATE TABLE shovel.migrations (
    idx integer NOT NULL,
    hash bytea NOT NULL,
    inserted_at timestamp with time zone DEFAULT now() NOT NULL
);



CREATE TABLE shovel.sources (
    name text,
    chain_id integer,
    url text
);



CREATE TABLE shovel.task_updates (
    num bigint,
    hash bytea,
    insert_at timestamp with time zone DEFAULT now(),
    src_hash bytea,
    src_num numeric,
    nblocks numeric,
    nrows numeric,
    latency interval,
    backfill boolean DEFAULT false,
    src_name text,
    stop numeric
);



ALTER TABLE ONLY shovel.migrations
    ADD CONSTRAINT migrations_pkey PRIMARY KEY (idx, hash);



CREATE UNIQUE INDEX intg_name_src_name_backfill_num_idx ON shovel.ig_updates USING btree (name, src_name, backfill, num DESC);



CREATE UNIQUE INDEX sources_name_chain_id_idx ON shovel.sources USING btree (name, chain_id);



CREATE UNIQUE INDEX sources_name_idx ON shovel.sources USING btree (name);



CREATE UNIQUE INDEX task_src_name_num_idx ON shovel.task_updates USING btree (src_name, num DESC) WHERE (backfill = true);



CREATE UNIQUE INDEX task_src_name_num_idx1 ON shovel.task_updates USING btree (src_name, num DESC) WHERE (backfill = false);




