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

## Versions

Latest stable version is: **1.0beta**

```
https://indexsupply.net/bin/1.0beta/darwin/arm64/shovel
https://indexsupply.net/bin/1.0beta/linux/amd64/shovel
```

Latest version on main:

```
https://indexsupply.net/bin/main/darwin/arm64/shovel
https://indexsupply.net/bin/main/linux/amd64/shovel
```

The following resources are automatically deployed on a main commit:

- Binaries https://indexsupply.net/bin/main
  `darwin/arm64`, `darwin/amd64`, `linux/amd64`, `windows/amd64`
- Docker https://hub.docker.com/r/indexsupply/shovel
  `linux/amd64`, `linux/arm64`
- This web site https://indexsupply.com

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

<hr>

## Ethereum Sources

A single Shovel process can connect to many Ethereum sources. Each Ethereum source is identified by name and is supplemented with a chain id and a URL. Shovel uses the following HTTP JSON RPC API methods:

1. eth_getBlockByNumber
2. eth_getLogs
3. eth_getBlockReceipts

Shovel will choose the RPC method based on what data the integration requires. The fastest way to index data is to only use data found in the eth_getLogs response. Specifically: `block_hash`, `block_num`, `tx_hash`, `tx_idx`, `log_addr`, `log_idx`, and log topics and events.

Upon startup, Shovel will use `eth_getBlockByNumber` to find the latest block. It will then compare the response with its latest block in Postgres to figure out where to begin. While indexing data, Shovel will make batched calls to `eth_getBlockByNumber`, batched calls to `eth_getBlockReceipts`, and single calls to `eth_getLogs` depending on the configured `batch_size` and the RPC methods required to index the data.

Ethereum sources are defined at the top level configuration object and then referenced in each integration.

```
{
  ...
  "eth_sources": [
    {
      "name": "",
      "chain_id": 0,
      "url": "",
      "ws_url": "",
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
- **url** A URL that points to a HTTP JSON RPC API. This can be a local {G,R}eth node or a Quicknode.
- **ws_url** An optional URL that points to a Websocket JSON RPC API. If this URL is set Shovel will use the websocket to get the _latest_ block instead of calling `eth_getBlockByNumber`. If the websocket fails for any reason Shovel will fallback on the HTTP based API.
- **batch_size** The maximum number of batched requests to make to the JSON RPC API. This can speed up backfill operations but will potentially use a lot of API credits if you are running on a hosted node.
- **concurrency** The maximum number of concurrent threads to run within a task. This value relates to `batch_size` in that for each task, the `batch_size` is partitioned amongst `concurrency` threads.

**integrations[].sources**

- **name** References the Ethereum source in the top level `eth_sources` field.
- **start** Optional. Only required if you want to backfill the integration. When you set start > 0 Shovel will create a task to track the latest blocks on the chain while concurrently creating a backfill task starting at `start`.
- **stop** Optional. If for some reason you don't want to backfill the entire chain but only a portion you can specify a `stop` value and the backfill will not index data that is < start or > stop.

It is possible to use an environment variable in the config object so that you don't have to embed your node url secret into the file.

```
{
  ...
  "eth_sources": [
    {
      ...
      "url": "$RPC_URL"
    }
  ],
```

Any value that is prefixed with a `$` will instruct Shovel to read from the environment. So something like `$L1_URL` works too.

Environment interpolation will work on the following fields in eth_sources:

- name
- chain_id
- url
- concurrency
- batch_size

And will also work on the following fields in integrations[].sources:

- name
- start
- stop

### Ethereum Source Performance

There are 2 dependent controls for Source performance:

1. The data you are indexing in your `integration`
2. `batch_size` and `concurrency` in `eth_sources` config

The more blocks we can process per RPC request (`batch_size`) the better. If you can limit your integration to using data provided by `eth_getLogs` then Shovel is able to reliably request up to 2,000 blocks worth of data in a single request. The data provided by eth_getLogs is as follows:

* block_hash
* block_num
* tx_hash
* tx_idx
* log_addr
* log_idx
* decoded log topics and data

You can further optimize the requests to `eth_getLogs` by providing a filter on `log_addr`. This will set the address filter in the `eth_getLogs` request. For example:

```
{
  "pg_url": "...",
  "eth_sources": [...],
  "integrations": [{
    ...
    "block": [
      {
        "name": "log_addr",
        "column": "log_addr",
        "filter_op": "contains",
        "filter_arg": [
          "0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48",
          "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913"
        ]
      }
    ],
    "event": {
      ...
    }
  }]
```

If, for example, you also need data from the header, such as `block_time` then Shovel will also need to download the headers for each block, along with the logs. Shovel will use JSON RPC batching, but this will be limited in comparison to eth_getLogs.

### Source Performance Config Values

The current version of Shovel has `batch_size` and `concurrency` values per eth source. If you have multiple integrations with varying data requirements then the eth source will be limited by the slowest integration's requirements. You can have multiple eth sources with identical urls and chain_ids but different names to workaround this.


If you are only indexing data provided by the logs, you can safely request 2,000 blocks worth of logs in a single request.

```
  ...
  "eth_sources": [
    {
      ...,
      "batch_size": 2000,
      "concurrency": 1
    }
  ],
  ...
}
```

You could further increase your throughput by increasing concurrency:

```
  ...
  "eth_sources": [
    {
      ...,
      "batch_size": 4000,
      "concurrency": 2
    }
  ],
  ...
}
```

However, if you are requesting data provided by the block header or by the block bodies (ie `tx_value`) then you will need to reduce your `batch_size` to a maximum of 100

```
  ...
  "eth_sources": [
    {
      ...,
      "batch_size": 100,
      "concurrency": 1
    }
  ],
  ...
}
```

You may see '429 too many request' errors in your logs as you are backfilling data. This is not necessarily bad since Shovel will always retry. In fact, the essence of Shovel's internal design is one big retry function. But the errors may eventually be too many to meaningfully make progress. In this case you should try reducing the `batch_size` until you no longer see high frequency errors.

<hr>

## Table

An integration must contains a `table` object. It is possible for many integrations to write to the same table. Each `integration` object must provide a complete `table` object. If integrations with disjoint columns share a table the table is created with a union of the columns.

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

- **name** Postgres table name.

- **columns** Array of columns that will be written to by the integration. Each column requires a name and a type.

  -- **name** Postgres column name
  -- **type** Can be one of: `bool`, `byte`, `bytea`, `int`, `numeric`, `text`.

    Shovel will not set default values for new columns. This means table modification are cheap and the table may contain null values.

    The following columns are required and will be added if they are not defined in the config:

    ```
    [
      {"name": "ig_name",   "type": "text"},
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

## Notifications

Shovel can use the [Postgres NOTIFY function](https://www.postgresql.org/docs/current/sql-notify.html) to send notifications when new rows are added to a table. This is useful if you want to provide low latency updates to clients. For example, you may have a browser client that opens an HTTP SSE connection to your web server. Your web server can use Postgres' `LISTEN` command to wait for new rows added to an integration's table. When the web server receives a notification, it can use data in the notification's payload to quickly send the update to the client via the HTTP SSE connection.

To configure notifications on an integration, speicfy a `notification` object in the integration's config.

There is a slight performance cost to using notifications. The cost should be almost 0 when Shovel is processing latest blocks but it may be non-zero when backfilling data. A couple of performance related things to keep in mind:

1. If there are no listeners then the notifications are instantly dropped
2. You can ommit the notification config if you are doing a backfill and then add it once things are in the steady state

```
{
    ...
    "integrations": [{
        "name": "foo",
        "sources": [{"name": "mainnet"}],
        "table": {
            ...
            "columns": [
                {"name": "a", "type": "bytea"},
                {"name": "b", "type": "bytea"}
            ]
        },
        "notification": {
            "columns": ["block_num", "a", "b"],
        }
        ...
    }]
}
```

- **columns** A list of strings that reference column names. Column names must be previously defined the the integration's [table](#table) config. The columns values are serialized to text (hex when binary) and encoded into a comma seperated list and placed in the notification's payload. The order of the payload is the order used in the `columns` list.

With this config, and when Shovel is inserting new data into the foo table, it will send a notification with the following data:

```
NOTIFY mainnet-foo '$block_num,$a,$b'
```

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
- **filter_ref** See [filters](#filters) for available operations and usage.

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
  - **filter_ref** See [filters](#filters) for available operations and usage.

The event object is a superset of [Solidity's ABI JSON spec](https://docs.soliditylang.org/en/v0.8.23/abi-spec.html#json). Shovel will read a few additional fields to determine how it will index an event. If an input defines a column, and that column name references a valid column in the integration's table configuration, then the input's value will be saved in the table. Omitting a column value indicates that the input's value should not be saved.

The event name, and input type names can be used to construct the hashed event signature (eg topics[0]). In fact, this is one of the check's that Shovel will preform while it is deciding if it should process a log.

<hr>

## Filters

A filter provides a way to reduce the amount of data in your database. You can filter on the block level (eg `tx_input`, `log_addr`) or at the event level (custom event fields).

The basic filter operation is to check if the incoming ethereum data contains (or doesn't contain) one or many byte arrays. You can specify the byte arrays either using a static, hex encoded json array (`filter_arg`) or you can reference annother integration's table as the source of data (`filter_ref`).

The filter is built from the following fields:

- **filter_op** Must be either `contains` or `!contains`
- **filter_arg** Must be an array of hex encoded (0x prefixed) bytes. Not required if using `filter_ref`. Use `filter_arg` when you want to filter on a set of static data.
- **filter_ref** Not required if using `filter_arg`. Use `filter_ref` if you want to filter based on dynamic data that has been created by other integrations. For example, this is useful for indexing events from factory created contracts.
    - **integration** Must be the name of an integration. This reference is used to determine the table name used for the filter data.
    - **column** Must be the name of a column defined in the integration's table.

**Filter References and Integration Dependencies**

Using `filter_ref` creates a dependency between the integration being filtered and the referenced integration. Shovel ensures that the referenced integration runs before the dependent integration.

A common BTREE index is automatically created for the reference table's column. This ensures that filter checks can happen as quickly as possible. The index is named `shovel_%s` where `%s` is the name of the referenced column.

### Multiple Filters

An integration can have multiple filters. Evaluation order is unspecified. An event, or transaction, is saved if one of the filters evaluates to `true`. In other words, the filters are evaluated and the results are combined using an `OR` operation.

### Filter Examples

<details>
<summary>Block filter</summary>

```
...
"integrations": [
    {
        ...
        "block": [
            {
                "name": "log_addr",
                "column": "log_addr",
                "filter_op": "contains",
                "filter_arg": ["0xabc"]
            }
        ]
    }
]
```
</details>

<details>
<summary>Event filter</summary>

```
...
"integrations": [
    {
        ...
        "event": {
            "name": "Transfer",
            "type": "event",
            "anonymous": false,
            "inputs": [
                {
                    "indexed": true,
                    "name": "from",
                    "type": "address",
                    "column": "f",
                    "filter_op": "contains",
                    "filter_arg": ["0x0000000000000000000000000000000000000000"]
                },
                {
                    "indexed": true,
                    "name": "to",
                    "type": "address",
                    "column": "t"
                },
                {
                    "indexed": false,
                    "name": "value",
                    "type": "uint256",
                    "column": "v"
                }
            ]
        }
    }
]
```
</details>

<details>
<summary>Block filter using filter_ref</summary>

```
...
"integrations": [
    {
        "name": "factory_contracts",
        ...
        "table": {
            "name": "factory_contracts",
            "columns": [
                {"name": "log_addr", "type": "bytea"},
                {"name": "addr", "type": "bytea"}
            ]
        },
        ...
    },
    {
        ...
        "event": {
            "name": "Transfer",
            "type": "event",
            "anonymous": false,
            "inputs": [
                {
                    "indexed": true,
                    "name": "from",
                    "type": "address",
                    "column": "f",
                    "filter_op": "contains",
                    "filter_ref": {
                        "integration": "factory_contracts",
                        "column": "addr"
                    }
                },
                {
                    "indexed": true,
                    "name": "to",
                    "type": "address",
                    "column": "t"
                },
                {
                    "indexed": false,
                    "name": "value",
                    "type": "uint256",
                    "column": "v"
                }
            ]
        }
    }
]
```
</details>

<hr>


## Database Schema

There are 2 parts to Shovel's database schema. The first are tables created in the `public` schema as a result of the table definitions in each integration's config. The second are tables located in the `shovel` schema that are related to Shovel's internal operations.

As Shovel indexes batches of blocks, each set of database operations are done within a Postgres transaction. This means that the internal tables and public integration tables are atomically updated. Therefore the database remains consistent in the event of a crash. For example, if there is a `shovel.task_update` record for block X then you can be sure that the integration tables contain data for block X --and vice versa.

The primary, internal table that Shovel uses to synchronize its work is the `shovel.task_updates` table:

### Task Updates Table

```
shovel=# \d shovel.task_updates
                      Table "shovel.task_updates"
  Column   |           Type           | Collation | Nullable | Default
-----------+--------------------------+-----------+----------+---------
 num       | numeric                  |           |          |
 hash      | bytea                    |           |          |
 ig_name   | text                     |           |          |
 src_name  | text                     |           |          |
 src_hash  | bytea                    |           |          |
 src_num   | numeric                  |           |          |
 nblocks   | numeric                  |           |          |
 nrows     | numeric                  |           |          |
 latency   | interval                 |           |          |
 insert_at | timestamp with time zone |           |          | now()
Indexes:
    "task_src_name_num_idx" UNIQUE, btree (ig_name, src_name, num DESC)
```

Each time Shovel indexes a batch of blocks, it will update the `shovel.task_updates` table with the last indexed `num` and `hash`. The `src_num` and `src_hash` are the latest num and hash as reported by the task's Ethereum source.


### Public Schema Table Requirements

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

`src_name`, `ig_name`, and `block_num` are used in the case of a reorg. When a reorg is detected, Shovel will delete rows from `shovel.task_updates` and for each pruned block Shovel will also delete rows from the integration tables using the aforementioned columns.

### Printing the Schema

Shovel has a command line flag that will print the schema based on the config's integrations.

```
./shovel -config minimal.json --print-schema
create table if not exists x(
	ig_name text,
	src_name text,
	block_num numeric,
	tx_idx int
);
create unique index if not exists u_x on x (
	ig_name,
	src_name,
	block_num,
	tx_idx
);
```

<hr>

## Tasks

Shovel's main thing is a task. Tasks are derived from Shovel's configuration. Shovel will parse the config (both in the file and in the database) to build a set of tasks to run.

- A task has a single Ethereum source.
- A task has a single Postgres destination.
- An integration can produce multiple tasks. One per source.
- A task starts at the configured start block in the integration's source field or, if the start field is omitted,  at the latest block height when the task is first run.

<hr>

# Monitoring

Shovel provides an unauthenticated diagnostics JSON endpoint at: `/diag` which returns:

```
[
  {
    "source": "mainnet",
    "latest": 100,
    "latency": 135,
    "error": "",
    "pg_latest": 100,
    "pg_latency": 1,
    "pg_error": ""
  },
  {
    "source": "sepolia",
    "latest": 100,
    "latency": 42,
    "error": "rpc error: unable to connect",
    "pg_latest": 90,
    "pg_latency": 3,
    "pg_error": ""
  }
]
```

This endpoint will iterate through all the [eth sources](#ethereum-sources) and query for the latest block on both the eth source and the `shovel.task_updates` table.

Latency is measured in milliseconds.

<hr>

## Logging

**msg=prune-task** This indicates indicates that Shovel has pruned the task updates table (`shovel.task_updates`). Each time that a task (backfill or main) indexes a batch of blocks, the latest block number is saved in the table. This is used for unwinding blocks during a reorg. On the last couple hundred of blocks are required and so Shovel will delete all but the last couple hundred records.

**p=16009** This is the local process id on the system. This can be useful when debugging process management (ie systemd) or deploys and restarts.

**v=d80f** This is the git commit that was used to build the binary. You can see this commit in the https://github.com/indexsupply/code repo by the following command

**nblocks=1** The number of blocks that were processed in a processing loop. If you are backfilling then this value will be min(batch_size/concurrency, num_blocks_behind). Otherwise, during incremental processing, it should be 1.

**nrpc=2** The number of JSON RPC requests that were made during a processing loop. In most cases the value will be 2. 1 to find out about the latest block and another to get the logs.

**nrows=0** The number of rows inserted into Postgres during the processing loop. Some blocks won't match your integrations and the value will be 0. Otherwise it will often correspond to the number of transactions of events that were matched during a processing loop.

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

## Dashboard

Shovel comes with a dashboard that can be used to:

1. Monitor the status of Shovel
2. Add new Ethereum Sources
3. Add new Integrations

Since the dashboard can affect the operations of Shovel it requires authentication. Here is how the authentication works:

By default non localhost requests will require authentication. This is to prevent someone accidentally exposing their unsecured Shovel dashboard to the internet.

By default localhost requests will require no authentication.

When authentication is enabled (by default or otherwise) the password will be either:

1. Set via the config
2. Randomly generated when the config file doesn't specify a password.

To set a password using the config file

```
{
  ...
  "dashboard": {
    "root_password": ""
  }
}
```

If the config is ommitted then Shovel will print the randomly generated password to the logs when a login web request is made. It will look like this

```
... password=171658e9feca092b msg=random-temp-password ...
```

Localhost authentication may be desirable for Shovel developers wanting to test the authentication bits. This is achieved with the following config
```
{
  ...
  "dashboard": {
    "enable_loopback_authn": true
  }
}
```

Authentication can be disabled entirely via:
```
{
  ...
  "dashboard": {
    "disable_authn": true
  }
}
```

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
            {"name": "chain_id",    "type": "numeric"},
            {"name": "log_addr",    "type": "bytea"},
            {"name": "block_time",  "type": "numeric"},
            {"name": "f",           "type": "bytea"},
            {"name": "t",           "type": "bytea"},
            {"name": "v",           "type": "numeric"}
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
