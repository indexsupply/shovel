

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


CREATE TABLE e2pg.migrations (
    idx integer NOT NULL,
    hash bytea NOT NULL,
    inserted_at timestamp with time zone DEFAULT now() NOT NULL
);



CREATE TABLE e2pg.task (
    id smallint NOT NULL,
    number bigint,
    hash bytea,
    insert_at timestamp with time zone DEFAULT now()
);



CREATE TABLE public.erc20_transfers (
    contract bytea,
    f bytea,
    t bytea,
    value numeric,
    tx_signer bytea,
    eth numeric,
    task_id numeric,
    chain_id numeric,
    block_hash bytea,
    block_number numeric,
    transaction_hash bytea,
    transaction_index numeric,
    log_index numeric
);



CREATE TABLE public.erc4337_userops (
    contract bytea,
    op_hash bytea,
    op_sender bytea,
    op_paymaster bytea,
    op_nonce numeric,
    op_success boolean,
    op_actual_gas_cost numeric,
    op_actual_gas_used numeric,
    tx_signer bytea,
    eth numeric,
    task_id numeric,
    chain_id numeric,
    block_hash bytea,
    block_number numeric,
    transaction_hash bytea,
    transaction_index numeric,
    log_index numeric
);



CREATE TABLE public.nft_transfers (
    contract bytea,
    token_id numeric,
    quantity numeric,
    f bytea,
    t bytea,
    tx_signer bytea,
    eth numeric,
    task_id numeric,
    chain_id numeric,
    block_hash bytea,
    block_number numeric,
    transaction_hash bytea,
    transaction_index numeric,
    log_index numeric
);



ALTER TABLE ONLY e2pg.migrations
    ADD CONSTRAINT migrations_pkey PRIMARY KEY (idx, hash);



ALTER TABLE ONLY public.erc20_transfers
    ADD CONSTRAINT erc20_transfers_unique UNIQUE (chain_id, transaction_hash, log_index);



ALTER TABLE ONLY public.erc4337_userops
    ADD CONSTRAINT erc4337_userops_unique UNIQUE (chain_id, transaction_hash, log_index);



CREATE INDEX task_id_number_idx ON e2pg.task USING btree (id, number DESC);




