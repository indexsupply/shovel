# E2PG

An Ethereum to Postgres indexer. At a high level, E2PG does the following:

- Reads blocks (header, bodies, receipts) from an Ethereum source
- Maps block data against a set of Integrations that can:
    - Decode logs (ABI) from a receipt
    - Combine decoded logs with arbitrary computation
    - Insert combined data into a Postgres database

The rest of this file describes how this process is accomplished.

## Contents

1. [Install](#install)
2. [Sync Mode](#sync-mode)
2. [Integrations](#integrations)
3. [Ethereum](#ethereum)
4. [Postgres](#postgres)

## Quickstart

```
# linux/amd64, darwin/arm64, darwin/amd64, windows/amd64
curl -LO https://indexsupply.net/bin/main/darwin/arm64/e2pg
chmod +x e2pg
# install postgres if needed. https://postgresapp.com/
createdb e2pg
export PG_URL=postgres:///e2pg
export RLPS_URL=https://c.rlps.indexsupply.net
./e2pg -reset -begin -1 -pg $PG_URL -rlps $RLPS_URL
psql e2pg
```

## Install

There are two ways to install: Build or Download

Currently E2PG is in developer preview. Once we have a stable release these links will include the proper version. Right now they are running/building off of the main branch.

### Build

1. Install go version 1.20 [go.dev/doc/install](https://go.dev/doc/install)
2. `go install github.com/indexsupply/x/cmd/e2pg@main`

### Download

```
curl -LO https://indexsupply/net/bin/darwin/arm64/main/e2pg
chmod +x e2pg
```

### Dependencies

If you are using RLPS then Postgres is the only dependency. E2PG connects to a Postgres server via socket or url. For example:

```
e2pg -rlps https://c.rlps.indexsupply.net -pg postgres:///e2pg
```

If you are using a local Geth node you will need to specify:

1. Postgres URL
2. Geth's RPC URL
3. Geth's freezer path

```
-pg postgres:///e2pg
-r $datadir/geth.ipc
-f $datadir/geth/chaindata/ancient/chain/
```

See [Machine Configuration](#machine-configuration) for more details on the difference between using a local node and RLPS.

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
-i erc721 - begin 1  -end 10
-i erc1155 -begin 11 -end 20
```

There will be erc721 events indexed for blocks [1, 10] and erc115 events indexed for blocks [11, 20].

## Sync Mode

E2PG can be used to backfill an index. Meaning you can process a set of integrations over historical blocks. E2PG can also continuously process blocks as they are created on the chain. Both modes use the same code path and produce the same result.

It is also possible to have a continuously running E2PG running alongside a backfill E2PG that is backfilling a newly added integration.

### Index a Block Range

The `-begin` and `-end` flags can be used to process a specific range of blocks and then exit once the `-end` block has been processed. If no `-end` flag is provided then E2PG will transition into continuous sync mode.

```
-begin 0 -end 17000000
```

### Index from Latest

To start with the network's current latest block, use the following flag:

```
-begin -1
```

### Reorgs

If E2PG gets a block from its Ethereum source where the new block's parent doesn't match the local block's hash, then the local block, and all it's integration data are deleted. After the deletion, E2PG attempts to add the new block. This process is repeated up to 10 times or until a hash/parent match is made. If there is a reorg of more than 10 blocks the database transaction is rolled back (meaning no data was deleted) and E2PG will halt progress. This condition requires operator intervention via SQL:

```sql
delete from driver where number > XXX;
delete from nft_transfers where block_number > XXX;
--etc...
```

This should never happen on Ethereum mainnet.

### Concurrency

There are two concurrency controls: number of workers and batch size. A batch is a set of blocks that are processed concurrently, and depending on your number of CPUs, in parallel. Note that it is possible that blocks are processed out of order. E2PG's integrations are designed accordingly.

The following flags control workers and batch size:

```
-w 32
-b 2048
```

It's best to use powers of 2.

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
-f $datadir/geth/chaindata/ancient/chain/
-r $datadir/geth.ipc
```

Where $datadir matches Geth's `--datadir XXX`

It is safe to run both Geth and E2PG at the same time and so far we have not noticed any impact on Geth's operations while running E2PG.

### RLPS

If you don't want to run a full Geth node, you can use our RLP Server. The RLP server is open source and the code is in this repo, but Index Supply offers it as a hosted service. Essentially the RLP Server reads blocks from Geth and serves them over HTTPs and via a CDN for optimized performance.

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
-rlps https://c.rlps.indexsupply.net
```

## Postgres

The PG in E2PG is a Postgres server that receives the indexed data. E2PG is developed against Postgres version 16. Although older versions may work. You can give E2PG a dedicated Postgres database or you can point E2PG at your existing Postgres database. Currently E2PG operates in Postgres' public schema, but it is possible to change that so that E2PG writes in its own schema within a database. Please file an issue if you need this feature. The schema can be found here: [e2pg/schema.sql](https://github.com/indexsupply/x/blob/main/e2pg/schema.sql)

Currently there is no migration mechanism. There is an open issue for this: indexsupply/x#127
