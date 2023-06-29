CREATE TABLE main_driver (
	number bigint,
	hash bytea,
	insert_at timestamptz default now()
);

CREATE UNLOGGED TABLE nft_transfers (
	contract bytea,
	token_id numeric,
	quantity numeric,
	f bytea,
	t bytea,
	tx_sender bytea,
	eth numeric,
	block_hash bytea,
	block_number numeric,
	transaction_hash bytea,
	transaction_index numeric,
	log_index numeric
);
