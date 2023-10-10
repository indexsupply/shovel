# E2PG

An Ethereum to Postgres indexer. At a high level, E2PG does the following:

- Reads blocks (header, bodies, receipts) from an Ethereum source
- Maps block data against a set of Integrations that can:
    - Decode logs (ABI) from a receipt
    - Insert data into a Postgres database

The rest of this file describes how this process is accomplished.

## Contents

0. [Quickstart](#quickstart)
1. [Install](#install)
2. [Configure](#configure)
3. [Reorgs](#reorgs)

## Quickstart

```bash
# linux/amd64, darwin/arm64, darwin/amd64, windows/amd64
curl -LO https://indexsupply.net/bin/main/darwin/arm64/e2pg
chmod +x e2pg

# install postgres if needed. https://postgresapp.com/
createdb e2pg

curl -LO https://raw.githubusercontent.com/indexsupply/x/main/cmd/e2pg/config.json
./e2pg -config config.json

# blocks are now being indexed and you can query your PG DB:
psql e2pg
```

## Install

There are two ways to install: Build or Download

Currently E2PG is in developer preview. Once we have a stable release these links will include the proper version. Right now they are running/building off of the main branch.

### Build

1. Install go version 1.21 [go.dev/doc/install](https://go.dev/doc/install)
2. `go install github.com/indexsupply/x/cmd/e2pg@main`

### Download

```bash
curl -LO https://indexsupply.net/bin/main/darwin/arm64/e2pg
chmod +x e2pg
```

### Dependencies

E2PG will need Postgres and an Ethereum node. Both of these are specified in the config file and can be URLs that point to local or hosted services.

## Configure

E2PG is configured via a JSON config file. Here is the basic structure:

```json
{
	"pg_url": "postgres:///e2pg",
	"eth_sources": [
		{"name": "goerli", "chain_id": 5, "url": "https://5.rlps.indexsupply.net"}
	],
	"integrations": []
}
```

### pg_url

E2PG will setup its database on startup. It keeps bookkeeping tables in a schema named "e2pg". Tables created from integrations are created in the public schema.

You can specify the database url in the config file. Or, you can instruct the config file to read from an environment variable via:

```json
"pg_url": "$DATABASE_URL"
```

The dollar sign indicates that E2PG should read from env.

### eth_sources

A list of sources that E2PG will use to download data. Integrations specify a list of sources by name.

Each source name must be unique but is only used for bookkeeping.

The chain_id is used to derive tx_signer information.

The url can point to a standard JSON RPC API (local or hosted) or it can point to Index Supply's hosted node service RLPS.

You can specify the url directly or you can instruct the config file to read from an environment variable via:

```json
"url": "$ETH_URL"
```

The dollar sign indicates that E2PG should read from env.

### JSON RPC API

E2PG uses `eth_getBlockByNumber` and `eth_getLogs`.

#### RLPS

Index Supply offers a data API that is optimized for indexing. It is faster than the JSON RPC API and can be significantly more cost effective for backfilling indexes.

### integrations

Integrations map a transaction or a log onto a row (set of columns) and that row is inserted into a Postgres table.

The log can be decoded using an ABI event specification.

Integrations are specified using the config file. Here is an example of an integration that reads transaction data and decodes a log event based on an ABI fragment:

```json
"integrations": [{
    "name": "ERC20 Transfers",
    "enabled": true,
    "sources": [{"name": "goerli"}],
    "table": {
        "name": "erc20_transfers",
        "columns": [
            {"name": "task_id", "type": "text"},
            {"name": "chain_id", "type": "numeric"},
            {"name": "block_num", "type": "numeric"},
            {"name": "tx_hash", "type": "bytea"},
            {"name": "contract", "type": "bytea"},
            {"name": "f", "type": "bytea"},
            {"name": "t", "type": "bytea"},
            {"name": "amt", "type": "numeric"}
        ]
    },
    "block": [
        {"name": "task_id", "column": "task_id"},
        {"name": "chain_id", "column": "chain_id"},
        {"name": "block_num", "column": "block_num"},
        {"name": "tx_hash", "column": "tx_hash"},
        {"name": "log_addr", "column": "contract"}
    ],
    "event": {
        "name": "Transfer",
        "type": "event",
        "anonymous": false,
        "inputs": [
            {
                "indexed": true,
                "name": "from",
                "type": "address",
                "column": "f"
            },
            {
                "indexed": true,
                "name": "to",
                "type": "address",
                "column": "t"
            },
            {
                "indexed": false,
                "name": "amount",
                "type": "uint256",
                "column": "amt"
            }
        ]
    }
}]
```
#### sources

Each integration can run on 1 or more chains. Chains are defined in the config's sources field. Within an integration, they are referenced by name.

#### table

The table is created dynamically. If the table already exists, nothing is changed. Later versions of E2PG will deal with table differences.

#### block

Instructs the integration to retrieve block level data. The available data include:

- task_id, text (e2pg internal bookkeeping)
- chain_id, numeric
- block_hash, bytea
- block_num, numeric
- tx_hash, bytea
- tx_idx, numeric
- tx_signer, bytea (aka from)
- tx_to, bytea
- tx_value, numeric (aka eth)
- tx_input, bytea (aka data)
- log_idx, numeric
- log_addr, bytea (aka contract)

The name must be in this list and the column must correspond to a column name in the table definition.

Additionally, you may also specify a "filter_op" and "filter_arg".

#### event

The event is a ABI fragment containing an ABI JSON event definition. However, an event's input may contain 3 additional fields:

1. "column" - a reference to a column name in the table definition
2. "filter_op" - a way to filter an input's value. Possible values are: "contains" and "!contains"
3. "filter_arg" - a JSON list of hex encoded, non-0x prefixed values.

## Reorgs

If E2PG gets a block from its Ethereum source where the new block's parent doesn't match the local block's hash, then the local block, and all it's integration data are deleted. After the deletion, E2PG attempts to add the new block. This process is repeated up to 10 times or until a hash/parent match is made. If there is a reorg of more than 10 blocks the database transaction is rolled back (meaning no data was deleted) and E2PG will halt progress. This condition requires operator intervention via SQL:

```sql
delete from e2pg.task where number > XXX;
--etc...
```

This should rarely happen.
