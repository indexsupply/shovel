<title>Shovel Docs</title>

Shovel's main thing is indexing Transactions and Events.

To do this, you will create a JSON config file defining the events and block data that you want to save along with the Postgres table that will store the saved data. With this file, you can start indexing by running Shovel:

```
./shovel -config config.json
```

<details>
<summary>Here is the smallest possible config file</summary>

```
{
  "pg_url": "postgres:///shovel",
  "eth_sources": [{"name": "m", "chain_id": 1, "url": "https://1.rlps.indexsupply.net"}],
  "integrations": [{
    "name": "small",
    "enabled": true,
    "sources": [{"name": "m"}],
    "table": {"name": "small", "columns": []},
    "block": [],
    "event": {}
  }]
}
```
_This config file works because there are required columns that are added to the table and to the block object array by default._
</details>

It's likely that you will want to index actual data. To do that, you'll need to fill in the `block` and `event` fields in the config object. See the following sections for instructions on how to do that:

1. [Event](#event)
2. [Block](#block)

You can also browse [Examples](#examples) to find one that does what you need.

<hr>

## Install

Here are things you'll need before you get started

1. Linux or Mac
2. URL to an Ethereum node (Alchemy, Geth)
3. URL to a Postgres database

If you are running a Mac and would like a nice way to setup Postgres, checkout: https://postgresapp.com.

To install Shovel, you can build from source (see [build from source](#build-from-source)) or you can download the binaries

```
curl -LO https://indexsupply.net/bin/1.0beta/darwin/arm64/shovel
chmod +x shovel
```

For Linux
```
curl -LO https://indexsupply.net/bin/1.0beta/linux/amd64/shovel
chmod +x shovel
```

After downloading the binaries we can now run the version command

```
./shovel -version
v1.0beta d80f
```

The first part of this command prints a version string (which is also a git tag) and the first two bytes of the latest commit that was used to build the binaries.

Once you have the dependencies setup you are ready to begin. Let's start by creating a Postgres database

```
createdb shovel
```

Now let's create a Shovel config file. You can copy the following contents into a local file named: config.json

```
{
  "pg_url": "postgres:///shovel",
  "eth_sources": [
    {"name": "mainnet", "chain_id": 1, "url": "https://1.rlps.indexsupply.net"},
    {"name": "base", "chain_id": 8453, "url": "https://8453.rlps.indexsupply.net"}
  ],
  "integrations": [{
    "name": "usdc-transfer",
    "enabled": true,
    "sources": [{"name": "mainnet"}, {"name": "base"}],
    "table": {
      "name": "usdc",
      "columns": [
        {"name": "log_addr",  "type": "bytea"},
        {"name": "block_time","type": "numeric"},
        {"name": "f",         "type": "bytea"},
        {"name": "t",         "type": "bytea"},
        {"name": "v",         "type": "numeric"}
      ]
    },
    "block": [
      {"name": "block_time", "column": "block_time"},
      {
        "name": "log_addr",
        "column": "log_addr",
        "filter_op": "contains",
        "filter_arg": [
          "a0b86991c6218b36c1d19d4a2e9eb0ce3606eb48",
          "833589fCD6eDb6E08f4c7C32D4f71b54bdA02913"
        ]
      }
    ],
    "event": {
      "name": "Transfer",
      "type": "event",
      "anonymous": false,
      "inputs": [
        {"indexed": true,  "name": "from",  "type": "address", "column": "f"},
        {"indexed": true,  "name": "to",    "type": "address", "column": "t"},
        {"indexed": false, "name": "value", "type": "uint256", "column": "v"}
      ]
    }
  }]
}
```

Let's run this config and see what happens

```
./shovel -config config.json
p=16009 v=d80f bf=0 n=0 msg=prune-ig
p=16009 v=d80f bf=0 n=0 msg=prune-task
p=16009 v=d80f chain=00001 bf=0 integrations=1 msg=new-task
p=16009 v=d80f chain=00001 bf=1 integrations=1 msg=new-task
p=16009 v=d80f chain=08453 bf=0 integrations=1 msg=new-task
p=16009 v=d80f chain=08453 bf=1 integrations=1 msg=new-task
p=16009 v=d80f chain=08453 bf=1 msg=done
p=16009 v=d80f chain=08453 bf=0 n=6904763 msg=converge
p=16009 v=d80f chain=08453 bf=0 n=6904764 msg=converge
p=16009 v=d80f chain=00001 bf=0 n=18622553 msg=converge
```

These logs indicate that Shovel has initialized and is beginning to index data.

<hr>

## Postgres

A single shovel connects to a single postgres. A connection is made using the URL defined in the config object.

```
{
  "pg_url": "postgres:///shovel",
  ...
}
```

It is possible to use an environment variable in the config object so that you don't have to embed your database password into the file.

```
{
  "pg_url": "$DATABASE_URL",
  ...
}
```

Any value that is prefixed with a `$` will instruct Shovel to read from the environment. So something like `$PG_URL` works too.

## Ethereum Sources

A single Shovel process can connect to many Ethereum sources. Each Ethereum source is identified by name and is supplemented with a chain id and a URL. Shovel will use the JSON RPC API on the other end of the URL. Shovel uses the following RPC methods:

1. eth_getBlockByNumber
2. eth_getLogs

Upon startup, Shovel will use `eth_getBlockByNumber` to find the latest block. It will then compare that with its latest block in Postgres to figure out where to begin. While indexing data, Shovel will make batched calls to `eth_getBlockByNumber` and a single call to `eth_getLogs` (depending on the configured batch size).

Ethereum sources are defined at the top level configuration object and then referenced in each integration.

```
{
  ...
  "eth_sources": [
    {
      "name": "",
      "chain_id": 0,
      "url": "",
      "batch_size": 1,
      "concurrency": 1
    }
  ],
  "integrations": [
    {
      ...
      "sources": [
        {
          "name": "",
          "start": 0,
          "stop": 0
      ]
    }
  ]
}
```

**eth_sources**

- **name** A unique name identifying the Ethereum source. This will be used throughout the system to identify the source of the data. It is also used as a foreign key in `integrations[].sources.name`
- **chain_id** There isn't a whole lot that depends on this value at the moment (ie no crypto functions) but it will end up in the integrations tables.
- **url** A URL that points to a JSON RPC API. This can be a local {G,R}eth node or a Quicknode.
- **batch_size** The maximum number of batched requests to make to the JSON RPC API. This can speed up backfill operations but will potentially use a lot of API credits if you are running on a hosted node.
- **concurrency** The maximum number of concurrent threads to run within a task. This value relates to `batch_size` in that for each task, the `batch_size` is partitioned amongst `concurrency` threads.

**integrations[].sources**

- **name** References the Ethereum source in the top level `eth_sources` field.
- **start** Optional. Only required if you want to backfill the integration. When you set start > 0 Shovel will create a task to track the latest blocks on the chain while concurrently creating a backfill task starting at `start`.
- **stop** Optional. If for some reason you don't want to backfill the entire chain but only a portion you can specify a `stop` value and the backfill will not index data that is < start or > stop.

<hr>

## Table

Each integration contains a `table` object. It is possible for many integrations to write to the same table even if they have unique sets of columns. On startup, Shovel will create the table if it doesn't exist (using the name) and then add missing columns.

```
{
  "integrations": [{
    "table": {
      "name": "",
      "columns": [{"name": "", "type": ""}],
      "unique": [""],
      "disable_unique": false
    }
  }]
}
```

- **name** Name of postgres table that the integration will write to. Multiple integrations can write to a single integration. See `columns` for more detail on table sharing.

- **columns** Array of columns that will be written to by the integration. Each column requires a name and a key.

  -- **name** Can be anything that is a valid postgres column name
  -- **type** Type can be one of: `bytea`, `numeric`, `text`.

  When Shovel sets up this integration, it will check to see if the table already exists (using the table name) and if it doesn't it will be created with the defined columns. If it does exist, Shovel will determine if there are tables defined in the configuration that are not defined in the database and each missing table will be added to the database.

  Shovel will not set default values for new columns. This means that the table modification will not be expensive but does mean that the table may contain null values for newly added columns.

  The following columns are required and will be added if they are not defined in the config:

    ```
    [
      {"name": "intg_name", "type": "text"},
      {"name": "src_name",  "type": "text"},
      {"name": "block_num", "type": "numeric"},
      {"name": "tx_idx",    "type": "numeric"}
    ]
    ```

  If the integration accessed data from a log it will also include

    ```
    [
      {"name": "log_idx", "type": "numeric"}
    ]
    ```

  If the log contains ABI encoded data it will also include

    ```
    [
      {"name": "abi_idx", "type": "numeric"}
    ]
    ```

- **unique** List of strings representing column names that should be combined into a unique index. Each column name in this array must be represented in the integration's `columns` field.

  The name of the index will be `u_` followed by the column names joined with an `_`. For example: `u_chain_id_block_num`

  If no `unique` field is defined in the config, and unless `disable_unique = true`, a default unique index will be created with the table's required columns.

- **disable_unique** There are some good reasons for not wanting to create an index by default. One such reason may be faster backfilling. In this case, you don't want any indexes (although you have to create the indexes at some point.) But in these cases you can disable default index creation. Shovel checks this value on startup so if `disable_unique` is removed then the default index will be created.

<hr>

## Block

In addition to log/event indexing, Shovel can also index standard block, transaction, receipt, and log data. It's possible to index log/event data and block data or just block data.

If an integration defines a `block` object and not a `event` object then Shovel will not take the time to read or decode the log data.

Here is a config snippet outlining how to index block data

```
{
  "integrations": [
    {
      "block": [{
        "name": "",
        "column": "",
        "filter_op": "",
        "filter_arg": [""]
      }],
    }
  ]
}
```

- **name** The name of the block field to index. Possible values include:
  - `chain_id int`
  - `block_num  numeric` - `block_hash bytea`
  - `block_time int (unix time)`
  - `tx_hash   bytea`
  - `tx_idx    int`
  - `tx_signer bytea`
  - `tx_to     bytea`
  - `tx_value  bytea`
  - `tx_input  bytea`
  - `tx_type   int`
  - `log_idx   int`
  - `log_addr  bytea`
- **column** The name of the corresponding column in the table definition.
- **filter_op** See [filters](#filters) for available operations and usage.
- **filter_arg** See [filters](#filters) for available operations and usage.

<details>
<summary>Example: Index transactions that call a specific function</summary>

This config uses the contains filter operation on the tx_input to index transactions that call the `mint(uint256)` function.

```
{
  "pg_url": "postgres:///shovel",
  "eth_sources": [
    {"name": "mainnet", "chain_id": 1, "url": "https://1.rlps.indexsupply.net"}
  ],
  "integrations": [{
    "name": "specific-tx",
    "enabled": true,
    "sources": [{"name": "mainnet"}],
    "table": {
      "name": "tx",
      "columns": [
        {"name": "tx_input", "type": "bytea"}
      ]
    },
    "block": [{
      "name": "tx_input",
      "column": "tx_input",
      "filter_op": "contains",
      "filter_arg": ["a0712d68"]
    }],
  }]
}
```
</details>

<hr>

## Event

The event config object is used for decoding logs/events. By adding an annotated ABI snippet to the event config, Shovel will match relevant logs, decode the logs using our optimized ABI decoder, and save the decoded result into your integration's table.

Here is a config snippet outlining how to index event data

```
{
  ...
  "integrations": [
    {
      ...
      "event": {
        "name": "",
        "type": "",
        "anonymous": false,
        "inputs": [{
          "indexed": false,
          "name": "",
          "type": "",
          "column": "",
          "filter_op": "",
          "filter_arg": [""]
        }]
      }
    }
  ]
}
```
- **name** The name of the event as defined in the ABI.
- **type** Must be `"event"`
- **anonymous** Must match value defined in the ABI.
- **inputs**
  - **indexed** Must match value defined in the ABI
  - **name** Less important since the type is used for building topics[0]
  - **type** Solidity type. Must match value defined in the ABI.
  - **column** Setting this value indicates that you would like this value to be saved in the table. Therefore, the column value must reference a column defined in the integration's table. Its types must also be compatible.
  - **filter_op** See [filters](#filters) for available operations and usage.
  - **filter_arg** See [filters](#filters) for available operations and usage.

The event object is a superset of [Solidity's ABI JSON spec](https://docs.soliditylang.org/en/v0.8.23/abi-spec.html#json). Shovel will read a few additional fields to determine how it will index an event. If an input defines a column, and that column name references a valid column in the integration's table configuration, then the input's value will be saved in the table. Omitting a column value indicates that the input's value should not be saved.

The event name, and input type names can be used to construct the hashed event signature (eg topics[0]). In fact, this is one of the check's that Shovel will preform while it is deciding if it should process a log.

<hr>

## Filters

Both block and event config objects expose a function for filtering block and event data. This gives the user the ability to reduce the amount of data that is being saved in the Postgres database.

In the case of a block object, you can add `filter_op` and `filter_arg` directly to the block object. In the case of an event object, the `filter_op` and `filter_arg` are located alongside an input object. See [block](#block) and [event](#event) sections for more details on their usage.

In both cases, an item can be filtered using two fields: `filter_op` and `filter_arg`.

**filter_op**

Current filter operations include: `contains` and `!contains`

**contains** takes an item (implicitly) and an array of hex encoded bytes. It compares the item to see if the item is contained in at least one of the hex encoded byte arrays in the argument.

**!contains** is the inverse of **contains**

<hr>

## Log Messages

**msg=prune-ig** This indicates that Shovel has pruned the integration updates table (`shovel.ig_updates`). Each time a batch of blocks is indexed, and for each running integration, Shovel will update this table with the latest block number. Only the earliest and the latest block number is required for internal book keeping so Shovel will periodically delete all but two records.

**msg=prune-task** This indicates indicates that Shovel has pruned the task updates table (`shovel.task_updates`). Each time that a task (backfill or main) indexes a batch of blocks, the latest block number is saved in the table. This is used for unwinding blocks during a reorg. On the last couple hundred of blocks are required and so Shovel will delete all but the last couple hundred records.

**p=16009** This is the local process id on the system. This can be useful when debugging process management (ie systemd) or deploys and restarts.

**v=d80f** This is the git commit that was used to build the binary. You can see this commit in the https://github.com/indexsupply/code repo by the following command

```
git show d80f --stat
commit d80f21e11cfc68df06b74cb44c9f1f6b2b172165 (tag: v1.0beta)
Author: Ryan Smith <r@32k.io>
Date:   Mon Nov 13 20:54:17 2023 -0800

    shove: ui tweak. update demo config

 cmd/shovel/demo.json  | 76 ++++++++++++++++++++++++++-------------------------
 shovel/web/index.html |  3 ++
 2 files changed, 42 insertions(+), 37 deletions(-)
```
<hr>

## Database Schema

There are 2 parts to Shovel's database schema. The first are the tables that are created in the `public` schema as a result of the table definitions in each integration's config. The second are tables located in the `shovel` schema that are related to Shovel's internal operations.

As Shovel indexes batches of blocks, each set of database operations are done within a Postgres transaction. This means that the internal tables and public integration tables are atomically updated. Therefore the database remains consistent in the event of a crash. For example, if there is a task_update record for block X then you can be sure that the integration tables contain data for block X --and vice versa.

There are 2 primary internal tables in the `shovel` schema.

**1. shovel.task_updates**

```
shovel=# \d shovel.task_updates
                      Table "shovel.task_updates"
  Column   |           Type           | Collation | Nullable | Default
-----------+--------------------------+-----------+----------+---------
 src_name  | text                     |           |          |
 backfill  | boolean                  |           |          | false
 num       | numeric                  |           |          |
 hash      | bytea                    |           |          |
 src_hash  | bytea                    |           |          |
 src_num   | numeric                  |           |          |
 nblocks   | numeric                  |           |          |
 nrows     | numeric                  |           |          |
 latency   | interval                 |           |          |
 insert_at | timestamp with time zone |           |          | now()
 stop      | numeric                  |           |          |
Indexes:
    "task_src_name_num_idx" UNIQUE, btree (src_name, num DESC) WHERE backfill = true
    "task_src_name_num_idx1" UNIQUE, btree (src_name, num DESC) WHERE backfill = false
```
For each Ethereum source, Shovel will create a main task, and if an integration's source config specifies a `start` block, Shovel will also create a backfill task.

Each time Shovel indexes a batch of blocks, it will update the `shovel.task_updates` table with the last indexed `num` and `hash`. The `src_num` and `src_hash` are the latest num and hash as reported by the task's Ethereum source.

`stop` indicates where the task will stop. For a main task this value is null and for a backfill task it is the latest block stop request amongst the task's integrations.

**2. shovel.ig_updates**

```
shovel=# \d shovel.ig_updates
              Table "shovel.ig_updates"
  Column  |   Type   | Collation | Nullable | Default
----------+----------+-----------+----------+---------
 name     | text     |           | not null |
 src_name | text     |           | not null |
 backfill | boolean  |           |          | false
 num      | numeric  |           | not null |
 latency  | interval |           |          |
 nrows    | numeric  |           |          |
 stop     | numeric  |           |          |
Indexes:
    "ig_name_src_name_backfill_num_idx" UNIQUE, btree (name, src_name, backfill, num DESC)
```

Each task (backfill or main) contains a set of integrations. The progress of each integration is tracked in the `shovel.ig_updates` table. For each batch of blocks that are indexed, the latest `num` is written to this table.

A backfill task's progress can be measured by the difference of: `max(num) where backfill = true` and `min(num) where backfill=false`.

**Public Schema Table Requirements**

Each table created by Shovel has a minimum set of required columns

```
                Table "public.minimal"
  Column   |  Type   | Collation | Nullable | Default
-----------+---------+-----------+----------+---------
 ig_name   | text    |           |          |
 src_name  | text    |           |          |
 block_num | numeric |           |          |
 tx_idx    | integer |           |          |
Indexes:
    "u_minimal" UNIQUE, btree (ig_name, src_name, block_num, tx_idx)
```

`src_name`, `ig_name`, and `block_num` are used in the case of a reorg. When a reorg is detected, Shovel will delete rows from its `shovel.task_updates` and `shovel.ig_updates` tables and for each pruned block Shovel will also delete rows from the integration tables using the aforementioned columns.

<hr>

## Tasks

Shovel's main thing is a task. tasks are derived from Shovel's configuration. Shovel will parse the config (both in the file and in the database) and build a set of tasks to run.

- A task has a single Ethereum source.
- A task has one or many integrations.
- A task can either be a `main` task or a `backfill` task
- A main task runs forever
- A backfill task runs until it catches up with the main task

Therefore, on startup, Shovel parses the config, and creates a set of tasks, one for each source / backfill combination and, depending on the integration's source config, adds one or more integrations to each task.

The tasks then are initialized, where they setup their book keeping records in the database, and then each task begins Converging.

Convergence is the process where the task figures out the Ethereum Source's latest block, and it figures out the task's latest block in the database, and then proceeds to download and index the delta.

For the backfill task, when the delta is 0 the task exits. For a main task, when the delta is 0, it sleeps for a second before attempting to converge again.

<hr>

## Examples

<details>
<summary>Transaction Inputs</summary>

```
{
  "pg_url": "postgres:///shovel",
  "eth_sources": [
    {
      "name": "mainnet",
      "chain_id": 1,
      "url": "https://1.rlps.indexsupply.net"
    }
  ],
  "integrations": [
    {
      "name": "specific-tx",
      "enabled": true,
      "sources": [
        {
          "name": "mainnet"
        }
      ],
      "table": {
        "name": "tx",
        "columns": [
          {
            "name": "tx_input",
            "type": "bytea"
          }
        ]
      },
      "block": [
        {
          "name": "tx_input",
          "column": "tx_input",
          "filter_op": "contains",
          "filter_arg": [
            "a1671295"
          ]
        }
      ]
    }
  ]
}
```
```
shovel=# \d tx
                  Table "public.tx"
  Column   |  Type   | Collation | Nullable | Default
-----------+---------+-----------+----------+---------
 tx_input  | bytea   |           |          |
 ig_name   | text    |           |          |
 src_name  | text    |           |          |
 block_num | numeric |           |          |
 tx_idx    | integer |           |          |
Indexes:
    "u_tx" UNIQUE, btree (ig_name, src_name, block_num, tx_idx)
```
</details>

<details>
<summary>Backfilling with Concurrency</summary>

```
{
  "pg_url": "postgres:///shovel",
  "eth_sources": [
    {
      "name": "fast",
      "chain_id": 1,
      "url": "https://1.rlps.indexsupply.net",
      "concurrency": 10,
      "batch_size": 100
    }
  ],
  "integrations": [{
    "name": "fast",
    "enabled": true,
    "sources": [{"name": "fast", "start": 1}],
    "table": {"name": "fast", "columns": []},
    "block": [],
    "event": {}
  }]
}
```
</details>

<details>
<summary>USDC Transfers on Multiple Chains</summary>

```
{
  "pg_url": "postgres:///shovel",
  "eth_sources": [
    {"name": "mainnet", "chain_id": 1, "url": "https://1.rlps.indexsupply.net"},
    {"name": "goerli",  "chain_id": 5, "url": "https://5.rlps.indexsupply.net"}
  ],
  "integrations": [
    {
      "name": "tokens",
      "enabled": true,
      "sources": [{"name": "mainnet"}, {"name": "goerli"}],
      "table": {
        "name": "transfers",
          "columns": [
            {"name": "chain_id",   "type": "numeric"},
            {"name": "log_addr",   "type": "bytea"},
            {"name": "block_time", "type": "numeric"},
            {"name": "f",          "type": "bytea"},
            {"name": "t"           "type": "bytea"},
            {"name": "v",          "type": "numeric"}
          ]
      },
      "block": [
        {"name": "chain_id", "column": "chain_id"},
        {"name": "block_time", "column": "block_time"},
        {
          "name": "log_addr",
          "column": "log_addr",
          "filter_op": "contains",
          "filter_arg": ["a0b86991c6218b36c1d19d4a2e9eb0ce3606eb48"]
        }
      ],
      "event": {
        "name": "Transfer",
        "type": "event",
        "anonymous": false,
        "inputs": [
          {"indexed": true,  "name": "from",  "type": "address", "column": "f"},
          {"indexed": true,  "name": "to",    "type": "address", "column": "t"},
          {"indexed": false, "name": "value", "type": "uint256", "column": "v"}
        ]
      }
    }
  ]
}
```
```
shovel=# \d transfers
                Table "public.transfers"
   Column   |   Type   | Collation | Nullable | Default
------------+----------+-----------+----------+---------
 chain_id   | numeric  |           |          |
 log_addr   | bytea    |           |          |
 block_time | numeric  |           |          |
 f          | bytea    |           |          |
 t          | bytea    |           |          |
 v          | numeric  |           |          |
 ig_name    | text     |           |          |
 src_name   | text     |           |          |
 block_num  | numeric  |           |          |
 tx_idx     | integer  |           |          |
 log_idx    | smallint |           |          |
 abi_idx    | smallint |           |          |
Indexes:
    "u_transfers" UNIQUE, btree (ig_name, src_name, block_num, tx_idx, log_idx, abi_idx)
```
</details>

<details>
<summary>Seaport Sales (Complex ABI Decoding)</summary>

```
{
  "pg_url": "postgres:///shovel",
  "eth_sources": [
    {
      "name": "mainnet",
      "chain_id": 1,
      "url": "https://1.rlps.indexsupply.net"
    }
  ],
  "integrations": [
    {
      "name": "seaport-orders",
      "enabled": true,
      "sources": [
        {
          "name": "mainnet"
        }
      ],
      "table": {
        "name": "seaport_orders",
        "columns": [
          {
            "name": "order_hash",
            "type": "bytea"
          }
        ]
      },
      "block": [],
      "event": {
        "name": "OrderFulfilled",
        "type": "event",
        "anonymous": false,
        "inputs": [
          {
            "indexed": false,
            "internalType": "bytes32",
            "name": "orderHash",
            "column": "order_hash",
            "type": "bytes32"
          },
          {
            "indexed": true,
            "internalType": "address",
            "name": "offerer",
            "type": "address"
          },
          {
            "indexed": true,
            "internalType": "address",
            "name": "zone",
            "type": "address"
          },
          {
            "indexed": false,
            "internalType": "address",
            "name": "recipient",
            "type": "address"
          },
          {
            "indexed": false,
            "internalType": "struct SpentItem[]",
            "name": "offer",
            "type": "tuple[]",
            "components": [
              {
                "internalType": "enum ItemType",
                "name": "itemType",
                "type": "uint8"
              },
              {
                "internalType": "address",
                "name": "token",
                "type": "address"
              },
              {
                "internalType": "uint256",
                "name": "identifier",
                "type": "uint256"
              },
              {
                "internalType": "uint256",
                "name": "amount",
                "type": "uint256"
              }
            ]
          },
          {
            "indexed": false,
            "internalType": "struct ReceivedItem[]",
            "name": "consideration",
            "type": "tuple[]",
            "components": [
              {
                "internalType": "enum ItemType",
                "name": "itemType",
                "type": "uint8"
              },
              {
                "internalType": "address",
                "name": "token",
                "type": "address"
              },
              {
                "internalType": "uint256",
                "name": "identifier",
                "type": "uint256"
              },
              {
                "internalType": "uint256",
                "name": "amount",
                "type": "uint256"
              },
              {
                "internalType": "address payable",
                "name": "recipient",
                "type": "address"
              }
            ]
          }
        ]
      }
    }
  ]
}
```
```
shovel=# \d seaport_orders
             Table "public.seaport_orders"
   Column   |   Type   | Collation | Nullable | Default
------------+----------+-----------+----------+---------
 order_hash | bytea    |           |          |
 ig_name    | text     |           |          |
 src_name   | text     |           |          |
 block_num  | numeric  |           |          |
 tx_idx     | integer  |           |          |
 log_idx    | smallint |           |          |
 abi_idx    | smallint |           |          |
Indexes:
    "u_seaport_orders" UNIQUE, btree (ig_name, src_name, block_num, tx_idx, log_idx, abi_idx)
```
</details>

You've reached the end. Thank you for reading.
<hr>
