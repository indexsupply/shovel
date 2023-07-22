

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

SET default_tablespace = '';

SET default_table_access_method = heap;


CREATE TABLE public.e2pg_migrations (
    idx integer NOT NULL,
    hash bytea NOT NULL,
    inserted_at timestamp with time zone DEFAULT now() NOT NULL
);



CREATE TABLE public.erc20_transfers (
    contract bytea,
    f bytea,
    t bytea,
    value numeric,
    tx_sender bytea,
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
    tx_sender bytea,
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
    tx_sender bytea,
    eth numeric,
    task_id numeric,
    chain_id numeric,
    block_hash bytea,
    block_number numeric,
    transaction_hash bytea,
    transaction_index numeric,
    log_index numeric
);



CREATE TABLE public.task (
    id smallint NOT NULL,
    number bigint,
    hash bytea,
    insert_at timestamp with time zone DEFAULT now()
);



ALTER TABLE ONLY public.e2pg_migrations
    ADD CONSTRAINT e2pg_migrations_pkey PRIMARY KEY (idx, hash);




