

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


CREATE SCHEMA e2pg;


SET default_tablespace = '';

SET default_table_access_method = heap;


CREATE TABLE e2pg.integrations (
    name text,
    conf jsonb
);



CREATE TABLE e2pg.intg (
    name text NOT NULL,
    src_name text NOT NULL,
    backfill boolean DEFAULT false,
    num numeric NOT NULL,
    latency interval,
    nrows numeric
);



CREATE TABLE e2pg.migrations (
    idx integer NOT NULL,
    hash bytea NOT NULL,
    inserted_at timestamp with time zone DEFAULT now() NOT NULL
);



CREATE TABLE e2pg.sources (
    name text,
    chain_id integer,
    url text
);



CREATE TABLE e2pg.task (
    num bigint,
    hash bytea,
    insert_at timestamp with time zone DEFAULT now(),
    src_hash bytea,
    src_num numeric,
    nblocks numeric,
    nrows numeric,
    latency interval,
    dstat jsonb,
    backfill boolean DEFAULT false,
    src_name text
);



ALTER TABLE ONLY e2pg.migrations
    ADD CONSTRAINT migrations_pkey PRIMARY KEY (idx, hash);



CREATE UNIQUE INDEX intg_name_src_name_backfill_num_idx ON e2pg.intg USING btree (name, src_name, backfill, num DESC);



CREATE UNIQUE INDEX sources_name_chain_id_idx ON e2pg.sources USING btree (name, chain_id);



CREATE UNIQUE INDEX sources_name_idx ON e2pg.sources USING btree (name);



CREATE UNIQUE INDEX task_src_name_num_idx ON e2pg.task USING btree (src_name, num DESC) WHERE (backfill = true);



CREATE UNIQUE INDEX task_src_name_num_idx1 ON e2pg.task USING btree (src_name, num DESC) WHERE (backfill = false);




