# E2PG

An Ethereum to Postgres indexer. At a high level, E2PG does the following:

- Reads blocks (header, bodies, receipts) from an Ethereum source
- Maps block data against a set of Integrations that can:
    - Decode logs (ABI) from a receipt
    - Combine decoded logs with arbitrary computation
    - Insert combined data into a Postgres database

The rest of this file describes how this process is accomplished.

## Contents

0. [Quickstart](#quickstart)
1. [Install](#install)
2. [Ethereum](#ethereum)
3. [Postgres](#postgres)
4. [Integrations](#integrations)
5. [Tasks](#tasks)
6. [Reorgs](#reorgs)

## Quickstart

```bash
# linux/amd64, darwin/arm64, darwin/amd64, windows/amd64
curl -LO https://indexsupply.net/bin/main/darwin/arm64/e2pg
chmod +x e2pg
# install postgres if needed. https://postgresapp.com/
createdb e2pg
export PG_URL=postgres:///e2pg
export RLPS_URL=https://1.rlps.indexsupply.net
./e2pg -reset -e $RLPS_URL -pg $PG_URL
# blocks are now being indexed and you can query your PG DB:
psql e2pg
```

## Install

There are two ways to install: Build or Download

Currently E2PG is in developer preview. Once we have a stable release these links will include the proper version. Right now they are running/building off of the main branch.

### Build

1. Install go version 1.20 [go.dev/doc/install](https://go.dev/doc/install)
2. `go install github.com/indexsupply/x/cmd/e2pg@main`

### Download

```bash
curl -LO https://indexsupply/net/bin/darwin/arm64/main/e2pg
chmod +x e2pg
```

### Dependencies

If you are using RLPS then Postgres is the only dependency. E2PG connects to a Postgres server via socket or url. For example:

```bash
e2pg -e https://1.rlps.indexsupply.net -pg postgres:///e2pg
```

If you are using a local Geth node you will need to specify:

1. Postgres URL
2. Geth's RPC URL
3. Geth's freezer path

```
-pg postgres:///e2pg
-e $datadir/geth.ipc
-ef $datadir/geth/chaindata/ancient/chain/
```

## Ethereum

The E in E2PG can be configured 2 different ways: Local Node OR RLPS

### Local Node

This is the fastest mode of operation. Geth keeps the last ~100k blocks in a LSM database and E2PG reads those blocks via the `debug_dbGet` RPC method. This is nearly the same performance as reading from the DB directly. The reason we use this `debug_dbGet` method at all is because both LevelDB and Pebble (DB engines used by Geth) allow only 1 process to access the DB --even if a 2nd process is read only.

```
┌───────────────────────────────────────────────────────────────────┐
│machine 1                                                          │
│                                                                   │
│  ┌───────────┐                                                    │
│  │Geth       │   Binary                                           │
│  │Freezer    │◀──Filesystem───┐                                   │
│  └───────────┘                │                                   │
│                               │                                   │
│                            ┌────┐                      ┌────────┐ │
│                            │E2PG│──────Socket─────────▶│Postgres│ │
│                            └────┘                      └────────┘ │
│  ┌───────────┐                │                                   │
│  │Geth       │   JSON         │                                   │
│  │debug_dbGet│◀──Socket───────┘                                   │
│  └───────────┘                                                    │
│                                                                   │
│                                                                   │
└───────────────────────────────────────────────────────────────────┘
```

When configuring E2PG to run in Local Node mode, use the following flags:

```
-e  $datadir/geth.ipc
-ef $datadir/geth/chaindata/ancient/chain/
```

Where $datadir matches Geth's `--datadir XXX`

It is safe to run both Geth and E2PG at the same time and so far we have not noticed any impact on Geth's operations while running E2PG.

### RLPS

If you don't want to run a full Geth node, you can use Index Supply's RLP Server. The RLP server is open source and the code is in this repo, but Index Supply offers it as a hosted service. Essentially the RLP Server reads blocks from a local node and serves them over HTTPs and via a CDN for optimized performance.

```
┌──────────┐       ┌──────────┐     ┌──────────────┐
│hosted    │       │machine 1 │     │hosted        │
│          │       │          │     │              │
│  ┌────┐  │       │  ┌────┐  │     │  ┌────────┐  │
│  │RLPS│◀─┼─HTTPS─┼──│E2PG│──┼─TCP─┼─▶│Postgres│  │
│  └────┘  │       │  └────┘  │     │  └────────┘  │
│          │       │          │     │              │
│          │       │          │     │              │
└──────────┘       └──────────┘     └──────────────┘
```

When configuring E2PG to run in RLPS mode, use the following flag:

```
-e https://1.rlps.indexsupply.net
```

## Postgres

The PG in E2PG is a Postgres server that receives the indexed data. E2PG is developed against Postgres version 16. Although older versions may work. You can give E2PG a dedicated Postgres database or you can point E2PG at your existing Postgres database. Currently E2PG operates in Postgres' public schema, but it is possible to change that so that E2PG writes in its own schema within a database. Please file an issue if you need this feature. The schema can be found here: [e2pg/schema.sql](https://github.com/indexsupply/x/blob/main/e2pg/schema.sql)

Currently there is no migration mechanism. There is an open issue for this: indexsupply/x#127

## Integrations

Currently available integrations:

- ERC721 Transfer
- ERC1155 Transfer/Batch Transfer

Integrations are pretty easy to add, we will be adding them based on request. Please open an issue and we can turn one around in a matter of days --if not hours.

Here is an example of running E2PG with both erc1155 and erc721 integrations:

```
-i erc1155,erc721
```

If no `-i ` flag is provided then all integration will be run.

Integration configuration is not persisted between runs. For example:

```
-i erc721  -begin 1  -end 10
-i erc1155 -begin 11 -end 20
```

There will be erc721 events indexed for blocks [1, 10] and erc115 events indexed for blocks [11, 20].

## Tasks

E2PG runs tasks. A task can be finite, for indexing a speific block range, or infinite, for indexing blocks as they occur on the chain. Each task is comprised of the following:

| Field         | Flag          | Description   |
| :------------ | :------------ | :------------ |
| name          | -name         | Used to identify task. Shows up in logging and UI.                                    |
| id            | -id           | Unique number of task. Used as PK/FK in Postgres.                                     |
| chain         | -chain        | Chain ID. (ie 1/mainnet 10/bedrock) used for signer config                            |
| eth           | -e            | Ethereun node source. Can be Unix Socket if using a local node or an RLPS URL.        |
| freezer       | -ef           | Location of freezer files. This is only required when using a local node.             |
| pg            | -pg           | Postgres URL                                                                          |
| concurrency   | -c            | Number of concurrent "threads" to process a batch of blocks. Usefull for backfilling  |
| batch         | -b            | Number of blocks to process during each iteration. Usefull for backfilling.           |
| integrations  | -i            | CSV List of integrations to use for the task.                                         |
| begin         | -begin        | Begin indexing at this block. 0 starts the task at the network's latest block.        |
| end           | -end          | Stop task at this block. 0 ensures the task will continue following the chain.        |

Multiple tasks can be run concurrently by definig the tasks in a JSON file or a single task can be run by using command line arguments.

Here is a sample task JSON document:
```json
[
	{
		"name": "mainnet",
		"id": 1,
		"chain": 1,
		"eth": "https://1.rlps.indexsupply.net",
		"pg": "postgres:///e2pg",
		"concurrency": 8,
		"batch": 128,
		"integrations": ["erc721", "erc1155"]
	},
	{
		"name": "bedrock",
		"id": 2,
		"chain": 10,
		"eth": "http://10.rlps.indexsupply.net",
		"pg": "postgres:///e2pg",
		"concurrency": 8,
		"batch": 1024,
		"integrations": ["erc721"]
	}
]
```

To start E2PG using with this configuration use the following command:

```bash
e2pg -config ./path/to/config.json
```

It is also possible to not use a JSON config file and instead start a single task entirely from the command line using the previously defined flags:

```bash
e2pg \
    -name mainnet \
    -id 1 \
    -chain 1 \
    -eth https://1.rlps.indexsupply.net \
    -pg postgres:///e2pg \
    -i erc721,erc1155
```

## Reorgs

If E2PG gets a block from its Ethereum source where the new block's parent doesn't match the local block's hash, then the local block, and all it's integration data are deleted. After the deletion, E2PG attempts to add the new block. This process is repeated up to 10 times or until a hash/parent match is made. If there is a reorg of more than 10 blocks the database transaction is rolled back (meaning no data was deleted) and E2PG will halt progress. This condition requires operator intervention via SQL:

```sql
delete from task where number > XXX;
delete from nft_transfers where block_number > XXX;
--etc...
```

This should rarely happen.
