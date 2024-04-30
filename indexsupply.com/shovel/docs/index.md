<title>Shovel Docs</title>

Shovel is a program that indexes data from an Ethereum node into a Postgres database. It uses standard JSON RPC APIs provided by all Ethereum nodes (ie Geth,Reth) and hosted nodes (ie Alchemy, Quicknode). It indexes blocks, transactions, and decoded event logs. Shovel uses a declarative JSON config to determine what data should be saved in Postgres.

The Shovel config contains a database URL, an Ethereum node URL, and an array of Integrations that contain a mapping of Ethereum data to Postgres tables.

<details>
<summary>
Here is a basic example of a config that saves ERC20 transfers
</summary>

```
{
  "pg_url": "postgres:///shovel",
  "eth_sources": [
    {
      "name": "mainnet",
      "chain_id": 1,
      "url": "https://ethereum-rpc.publicnode.com"
    }
  ],
  "integrations": [{
    "name": "erc20_transfers",
    "enabled": true,
    "sources": [{"name": "mainnet"}],
    "table": {
      "name": "erc20_transfers",
      "columns": [
        {"name": "block_num", "type": "numeric"},
        {"name": "tx_hash",   "type": "bytea"},
        {"name": "from",      "type": "bytea"},
        {"name": "to",        "type": "bytea"},
        {"name": "value",     "type": "bytea"},
      ]
    },
    "block": [
      {"name": "block_num", "column": "block_num"},
      {"name": "tx_hash",   "column": "tx_hash"}
    ],
    "event": {
      "name": "Transfer",
      "type": "event",
      "anonymous": false,
      "inputs": [
        {"indexed": true,  "name": "from",  "type": "address", "column": "from"},
        {"indexed": true,  "name": "to",    "type": "address", "column": "to"},
        {"indexed": false, "name": "value", "type": "uint256", "column": "value"}
      ]
    }
  }]
}
```
</details>

In the example config you will notice that we define a PG table named `erc20_transfers` with 5 columns. Shovel will create this table on startup. We specify 2 fields that we want to save from the block and transaction data and we provide the Transfer event from the ERC20 ABI JSON. The Transfer event ABI snippet has an additional key on the input objects named `column`. The `column` field indicates that we want to save the data from this input and references a column previously defined in `table`.

We can run this config with:

```
./shovel -config config.json
```

This concludes the basic introduction to Shovel. Please read the rest of this page for a comprehensive understanding of Shovel or browse these common topics:

- Index data from multiple chains: [Ethereum Sources](#ethereum-sources)
- Index additional Block/Transaction data: [Block](#block)
- Define and Filter indexed data based on the decoded logs: [Event](#event)
- And some basic [examples](#examples)

Best of luck and feel free to reach out to [support@indexsupply.com](mailto:support@indexsupply.com) with any questions.

<hr>

## Changelog

Latest stable version is: **1.4**

```
https://indexsupply.net/bin/1.4/darwin/arm64/shovel
https://indexsupply.net/bin/1.4/linux/amd64/shovel
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

### v1.4 {#changelog-v1.4}

`2f76 2024-04-29`

- index trace_block data see 'trace_*' in [block data fields](#block-data-fields)
- filter numerical data. new filter ops: eq, ne, lt, gt. see [filter operations](#filter-operations)
- fix db encoding for events that indexed an array of addresses (removed padding byte)


### v1.3 {#changelog-v1.3}

`8E73 2024-04-13`

- expose `/metrics` endpoint for Prometheus monitoring

### v1.2 {#changelog-v1.2}

`91DA 2024-04-09`

- shovel integration table config exposes field for adding a db index

### v1.1 {#changelog-v1.1}

`E1F2 2024-04-07`

- fix dashboard access via localhost
- add abi support for int types (prev. version had uint support only)

<hr>

## Install

Here are things you'll need before you get started

1. Linux or Mac
2. URL to an Ethereum node (Alchemy, Geth)
3. URL to a Postgres database

If you are running a Mac and would like a nice way to setup Postgres, checkout: https://postgresapp.com.

To install Shovel, you can build from source (see [build from source](#build-from-source)) or you can download the binaries

```
curl -LO https://indexsupply.net/bin/1.4/darwin/arm64/shovel
chmod +x shovel
```

For Linux
```
curl -LO https://indexsupply.net/bin/1.4/linux/amd64/shovel
chmod +x shovel
```

After downloading the binaries we can now run the version command

```
./shovel -version
v1.4 2f76
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
    {"name": "mainnet", "chain_id": 1, "url": "https://ethereum-rpc.publicnode.com"},
    {"name": "base", "chain_id": 8453, "url": "https://base-rpc.publicnode.com"}
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
p=8232 v=7602 chain=00001 src=mainnet dest=usdc-transfer msg=new-task
p=8232 v=7602 chain=08453 src=base dest=usdc-transfer msg=new-task
p=8232 v=7602 n=0 msg=prune-task
p=8232 v=7602 chain=00001 num=19515851 msg=start at latest
p=8232 v=7602 chain=08453 num=12316626 msg=start at latest
p=8232 v=7602 chain=00001 src=mainnet dst=usdc-transfer n=19515851 h=f5878692 nrows=0 nrpc=4 nblocks=1 elapsed=1.410290458s msg=converge
p=8232 v=7602 chain=08453 src=base dst=usdc-transfer n=12316626 h=7d6bcffa nrows=1 nrpc=4 nblocks=1 elapsed=1.555755s msg=converge
p=8232 v=7602 chain=08453 src=base dst=usdc-transfer n=12316627 h=c1e7d431 nrows=3 nrpc=2 nblocks=1 elapsed=292.192958ms msg=converge
p=8232 v=7602 chain=08453 src=base dst=usdc-transfer n=12316628 h=8f257b22 nrows=2 nrpc=2 nblocks=1 elapsed=350.890166ms msg=converge
p=8232 v=7602 chain=08453 src=base dst=usdc-transfer n=12316629 h=257a79f5 nrows=0 nrpc=2 nblocks=1 elapsed=290.85075ms msg=converge
p=8232 v=7602 chain=08453 src=base dst=usdc-transfer n=12316630 h=5878d5ad nrows=1 nrpc=2 nblocks=1 elapsed=375.884666ms msg=converge
p=8232 v=7602 chain=08453 src=base dst=usdc-transfer n=12316631 h=4388c05a nrows=4 nrpc=2 nblocks=1 elapsed=288.755125ms msg=converge
p=8232 v=7602 chain=08453 src=base dst=usdc-transfer n=12316632 h=4faeaadb nrows=6 nrpc=2 nblocks=1 elapsed=274.551083ms msg=converge
p=8232 v=7602 chain=08453 src=base dst=usdc-transfer n=12316633 h=6207463b nrows=7 nrpc=2 nblocks=1 elapsed=287.200542ms msg=converge
p=8232 v=7602 chain=00001 src=mainnet dst=usdc-transfer n=19515852 h=1c3b5de9 nrows=0 nrpc=2 nblocks=1 elapsed=364.504708ms msg=converge
p=8232 v=7602 chain=08453 src=base dst=usdc-transfer n=12316634 h=be6c2c0a nrows=6 nrpc=2 nblocks=1 elapsed=463.80825ms msg=converge
```

These logs indicate that Shovel has initialized and is beginning to index data. Congratulations. Smoke 'em if you got 'em.

<hr>

## TypeScript

Since the Shovel JSON config contains a declarative description of all the data that you are indexing, it can become large and repetitive. If you would like an easier way to manage this config, you can use the TypeScript package which includes type definitions for the config structure. This will allow you to use loops, variables, and everything else that comes with a programming language to easily create and manage your Shovel config.

NPM package: https://npmjs.com/package/@indexsupply/shovel-config

TS docs: https://jsr.io/@indexsupply/shovel-config

Source: https://github.com/indexsupply/code/tree/main/shovel-config-ts

<details>
<summary>Example</summary>

```
import { makeConfig, toJSON } from "@indexsupply/shovel-config";
import type { Source, Table, Integration } from "@indexsupply/shovel-config";

const table: Table = {
  name: "transfers",
  columns: [
    { name: "log_addr", type: "bytea" },
    { name: "from", type: "bytea" },
    { name: "to", type: "bytea" },
    { name: "amount", type: "numeric" },
  ],
};

const mainnet: Source = {
  name: "mainnet",
  chain_id: 1,
  url: "https://ethereum-rpc.publicnode.com",
};

let integrations: Integration[] = [
  {
    enabled: true,
    name: "transfers",
    sources: [{ name: mainnet.name, start: 0n }],
    table: table,
    block: [
      {
        name: "log_addr",
        column: "log_addr",
        filter_op: "contains",
        filter_arg: ["0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48"],
      },
    ],
    event: {
      type: "event",
      name: "Transfer",
      inputs: [
        { indexed: true, name: "from", type: "address", column: "from" },
        { indexed: true, name: "to", type: "address", column: "to" },
        { indexed: false, name: "amount", type: "uint256", column: "amount" },
      ],
    },
  },
];

const config = makeConfig({
  pg_url: "postgres:///shovel",
  sources: [mainnet],
  integrations: integrations,
});

console.log(toJSON(config));
```
</details>

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
4. trace_block

Shovel will choose the RPC method based on what data the integration requires. The fastest way to index data is to only use data found in the eth_getLogs response. Specifically: `block_hash`, `block_num`, `tx_hash`, `tx_idx`, `log_addr`, `log_idx`, and log topics and events. The slowest way to index data is to use `trace_block`.

See [Block Data Fields](#block-data-fields) for a table outlining the data that you can index and its required API.

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
      "poll_duration": "500ms",
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
- **poll_duration** The amount of time to wait before checking the source for a new block. A lower value (eg 100ms) will increase the total number of requests that Shove will make to your node. This may count against your rate limit. A higher value will reduce the number of requests made. The default is `1s`.
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
- poll_duration
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

An integration must contain a `table` object. It is possible for many integrations to write to the same table. If integrations with disjoint columns share a table the table is created with a union of the columns.

```
{
  "integrations": [{
    "table": {
      "name": "",
      "columns": [{"name": "", "type": ""}],
      "index": [[""]],
      "unique": [[""]],
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

- **index** Array of an array of strings representing column names that should be used in creating a B-Tree index. Each column name in this array must be represented in the table's `columns` field. A column name may be suffixed with `ASC` or `DESC` to specify a sort order for the index.

- **unique** Array of an array of strings representing column names that should be combined into a unique index. Each column name in this array must be represented in the table's `columns` field.

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

## Block Data

In addition to log/event indexing, Shovel can also index block, transaction, receipt, logs, and traces. It's possible to index log/event data and block data or just block data.

If an integration defines a `block` object and not a `event` object then Shovel will not take the time to read or decode the log data. Similarly, if and only if a `block` objects contains `trace_*` fields, then Shovel will download data from the `trace_block` endpoint.

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

- **name** Name of block field. See [Block Data Fields](#block-data-fields) for possible values.
- **column** Reference to name of column in the integration's table.
- **filter_op** See [filters](#filters) for available operations and usage.
- **filter_arg** See [filters](#filters) for available operations and usage.
- **filter_ref** See [filters](#filters) for available operations and usage.

### Block Data Fields

| Field Name  | Eth Type        | Postgres Type    | Eth API          |
|-------------|-----------------|------------------|------------------|
| chain_id    | int             | int              | `n/a`            |
| block_num   | numeric         | numeric          | `l, r, h, b, t`  |
| block_hash  | bytea           | bytea            | `l, r, h, b, t`  |
| block_time  | int             | int              | `h, b`           |
| tx_hash     | bytes32         | bytea            | `l, r, h, b, t`  |
| tx_idx      | int             | int              | `l, r, h, b, t`  |
| tx_signer   | address         | bytea            | `r, b`           |
| tx_to       | address         | bytea            | `r, b`           |
| tx_value    | uint256         | numeric          | `r, b`           |
| tx_input    | bytes           | bytea            | `r, b`           |
| tx_type     | byte            | int              | `r, b`           |
| tx_status   | byte            | int              | `r`              |
| log_idx     | int             | int              | `l, r`           |
| log_addr    | address         | bytea            | `l, r`           |
| trace_action_call_type | string  | text          | `t`              |
| trace_action_idx       | int     |int            | `t`              |
| trace_action_from      | address | bytea         | `t`              |
| trace_action_to        | address | bytea         | `t`              |
| trace_action_value     | uint256 | numeric       | `t`              |

The Eth API can be one of:

- `l`: eth_getLogs
- `r`: eth_getBlockReceipts
- `h`: eth_getBlockByNumber(no txs)
- `b`: eth_getBlockByNumber(txs)
- `t`: trace_block

Shovel will optimize its Eth API choice based on the data that your integration requires. Integrations that only require data from `eth_getLogs` will be the most performant since `eth_getLogs` can filter and batch in ways that the other Eth APIs cannot. Whereas `eth_getBlockReceipts` and `trace_block` are extremely slow. Keep in mind that integrations are run independently so that you can partition your workload accordingly.

### Block Data Examples

<details>
<summary>Index Internal Eth Transfers via Traces</summary>

```
{
    "pg_url": "postgres:///shovel",
    "eth_sources": [{"name": "mainnet", "chain_id": 1, "url": "XXX"}],
    "integrations": [
        {
            "name": "internal-eth-transfers",
            "enabled": true,
            "sources": [{"name": "mainnet", "start": 19737332, "stop": 19737333}],
            "table": {
                "name": "internal_eth_transfers",
                "columns": [
                    {"name": "block_hash", "type": "bytea"},
                    {"name": "block_num", "type": "numeric"},
                    {"name": "tx_hash", "type": "bytea"},
                    {"name": "tx_idx", "type": "int"},
                    {"name": "call_type", "type": "text"},
                    {"name": "from", "type": "bytea"},
                    {"name": "to", "type": "bytea"},
                    {"name": "value", "type": "numeric"}
                ]
            },
            "block": [
                {"name": "block_hash", "column": "block_hash"},
                {"name": "block_num", "column": "block_num"},
                {"name": "tx_hash", "column": "tx_hash"},
                {
                    "name": "trace_action_call_type",
                    "column": "call_type",
                    "filter_op": "eq",
                    "filter_arg": ["call"]
                },
                {"name": "trace_action_from", "column": "from"},
                {"name": "trace_action_to", "column": "to"},
                {
                    "name": "trace_action_value",
                    "column": "value",
                    "filter_op": "gt",
                    "filter_arg": ["0"]
                }
            ]
        }
    ]
}
```
</details>

<details>
<summary>Index transactions that call a specific function</summary>

This config uses the contains filter operation on the tx_input to index transactions that call the `mint(uint256)` function.

```
{
  "pg_url": "postgres:///shovel",
  "eth_sources": [
    {"name": "mainnet", "chain_id": 1, "url": "https://ethereum-rpc.publicnode.com"}
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

## Event Data

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

A filter reduces the amount of data in your database, and in some cases, reduces the amount of data downloaded from an [Ethereum Source](#ethereum-source).

Reducing the amount of data downloaded from the Ethereum Source is accomplished with [**eth_getLogs**](https://ethereum.org/en/developers/docs/apis/json-rpc/#eth_getlogs) and its filtering system. This API allows users to filter logs based on: block range, contract address, and topics. For example, if you are indexing USDC transfers, Shovel's filters would supply eth_getLogs with a filter such that Shovel would only download logs related to USDC transfers.

For all other data: blocks, transactions, receipts, and traces; Shovel will download the complete objects and then, with the use of filters, remove the objects that don't match the criteria.

### Filter Fields

The following are a list of filter fields that can be embedded in a `block` or `event` item.

- **filter_op** Must be: `contains` or `!contains` when using `filter_ref` or can be: `contains`, `!contains`, `eq`, `ne`, `gt`, or `lt` when using `filter_arg`.
- **filter_arg** Not required when using `filter_ref`. Use `filter_arg` when you want to filter on static data. Must be an array of strings.
- **filter_ref** Not required when using `filter_arg`. Use `filter_ref` if you want to filter based on dynamic data that has been created by other integrations. For example, this is useful for indexing events from factory created contracts.
    - **integration** Must be the name of an integration. This reference is used to determine the table name used for the filter data.
    - **column** Must be a column name defined in the integration's table.

### Filter Operations

Here are the available filter operations. The `filter_op` and `filter_arg` are to be used inside the `event.inputs[]` or `block[]` object. The `input` is the data from the Ethereum Source that is to be tested.

    filter_op: contains, !contains
        input: binary data
        filter_arg: json array of hex encoded, 0x-prefixed bytes

        input: string
        filter_arg: json array of utf8 encoded strings

    filter_op: eq, ne
        input: binary data
        filter_arg: json array of hex encoded, 0x-prefixed bytes

        input: string
        filter_arg: json array of utf8 encoded strings

        input: int/uint
        filter_arg: json array of a single 256bit number encoded as decimal

    filter_op: lt, gt
        input: int/uint
        filter_arg: json array of a single 256bit number encoded as decimal


### Filter References and Integration Dependencies

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

## Monitoring

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

This endpoint is rate limited to 1 request per second.

Latency is measured in milliseconds.

### Prometheus

Shovel also exposes a `/metrics` endponit that prints the following Prometheus metrics:

```
# HELP shovel_latest_block_local last block processed
# TYPE shovel_latest_block_local gauge
shovel_latest_block_local{src="mainnet"} 19648035

# HELP shovel_pg_ping number of ms to make basic status query
# TYPE shovel_pg_ping gauge
shovel_pg_ping 0

# HELP shovel_pg_ping_error number of errors in making basic status query
# TYPE shovel_pg_ping_error gauge
shovel_pg_ping_error 0

# HELP shovel_latest_block_remote latest block height from rpc api
# TYPE shovel_latest_block_remote gauge
shovel_latest_block_remote{src="mainnet"} 19648035

# HELP shovel_rpc_ping number of ms to make a basic http request to rpc api
# TYPE shovel_rpc_ping gauge
shovel_rpc_ping{src="mainnet"} 127

# HELP shovel_rpc_ping_error number of errors in making basic rpc api request
# TYPE shovel_rpc_ping_error gauge
shovel_rpc_ping_error{src="mainnet"} 0
```

This endpoint will iterate through all the [eth sources](#ethereum-sources) and query for the latest block on both the eth source and the `shovel.task_updates` table. Each source will use a separate Prometheus label.

This endpoint is rate limited to 1 request per second.

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
      "url": "https://ethereum-rpc.publicnode.com"
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
      "url": "https://ethereum-rpc.publicnode.com",
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
    {"name": "mainnet", "chain_id": 1, "url": "https://ethereum-rpc.publicnode.com"},
    {"name": "goerli",  "chain_id": 11155111, "url": "https://ethereum-sepolia-rpc.publicnode.com"}
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
      "url": "https://ethereum-rpc.publicnode.com"
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

<hr>

You've reached the end. Thank you for reading.
