

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
    id text NOT NULL,
    number bigint,
    hash bytea,
    insert_at timestamp with time zone DEFAULT now()
);



ALTER TABLE ONLY e2pg.migrations
    ADD CONSTRAINT migrations_pkey PRIMARY KEY (idx, hash);



CREATE UNIQUE INDEX sources_name_chain_id_idx ON e2pg.sources USING btree (name, chain_id);



CREATE UNIQUE INDEX sources_name_idx ON e2pg.sources USING btree (name);



CREATE INDEX task_id_number_idx ON e2pg.task USING btree (id, number DESC);




