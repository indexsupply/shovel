<title>Shovel Docs</title>

Shovel is a program that indexes data from an Ethereum node into a Postgres database. It uses standard JSON RPC APIs provided by Ethereum nodes (ie Geth,Reth) and node providers (ie Alchemy, Quicknode). It indexes blocks, transactions, decoded event logs, and traces. Shovel uses a declarative JSON config to determine what data should be saved in Postgres.

The Shovel Config contains a database URL, an Ethereum node URL, and an array of Integrations that contain a mapping of Ethereum data to Postgres tables.

<details>
<summary>
Here is a basic Config that saves ERC20 transfers
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
        {"name": "tx_hash", "type": "bytea"},
        {"name": "from", "type": "bytea"},
        {"name": "to", "type": "bytea"},
        {"name": "value", "type": "bytea"},
      ]
    },
    "block": [
      {"name": "block_num", "column": "block_num"},
      {"name": "tx_hash", "column": "tx_hash"}
    ],
    "event": {
      "name": "Transfer",
      "type": "event",
      "anonymous": false,
      "inputs": [
        {"indexed": true, "name": "from", "type": "address", "column": "from"},
        {"indexed": true, "name": "to", "type": "address", "column": "to"},
        {"name": "value", "type": "uint256", "column": "value"}
      ]
    }
  }]
}
```
</details>

The example Config defines a PG table named `erc20_transfers` with 5 columns. The table is created on startup. We specify 2 block fields to be indexed: `block_num` & `tx_hash`. We also provide the `Transfer` event from the ERC20 ABI JSON. The Transfer event ABI snippet has an additional key on the input objects named `column`. The `column` field indicates that we want to save the data from this event input by referencing a column previously defined in `table`.

To run this Config, run the following commands on your Mac:
```
brew install postgresql@16
brew services start postgresql@16
createdb shovel

curl -LO --silent https://raw.githubusercontent.com/indexsupply/code/main/cmd/shovel/demo.json
curl -LO --silent https://indexsupply.net/bin/1.6/darwin/arm64/shovel
chmod +x shovel

./shovel -config demo.json
l=info  v=1.6 msg=new-task ig=usdc-transfer src=mainnet
l=info  v=1.6 msg=new-task ig=usdc-transfer src=base
l=info  v=1.6 msg=prune-task n=0
l=info  v=1.6 msg=start at latest ig=usdc-transfer src=mainnet num=19793295
l=info  v=1.6 msg=start at latest ig=usdc-transfer src=base num=13997369
l=info  v=1.6 msg=converge ig=usdc-transfer src=mainnet req=19793295/1 n=19793295 h=f537a5c3 nrows=1 nrpc=4 nblocks=1 elapsed=1.117870458s
l=info  v=1.6 msg=converge ig=usdc-transfer src=base req=13997369/1 n=13997369 h=9b4a1912 nrows=0 nrpc=4 nblocks=1 elapsed=1.160724958s
```

<hr>

## Change Log {#changelog}

Latest stable version is: **1.6**

```
https://indexsupply.net/bin/1.6/darwin/arm64/shovel
https://indexsupply.net/bin/1.6/linux/amd64/shovel
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

### main {#changelog-main}

On main but not yet associated with a new version tag.

- fix db encoding for negative int{..256} values
- fix `error="getting receipts: no rpc error but empty result"`
- add tx_gas_used, tx_effective_gas_price as [data](#data) options. requires eth_getBlockReceipts
- fix NOTIFICATION payload encoding for numeric/uint256
- add trace_ fields to shovel-config-ts

### v1.6 {#changelog-v1.6}

`582D 2024-05-06`

- rewrite the docs to include more detailed desc. of config
- handle eth api provider's [unsynchronized nodes](https://indexsupply.com/shovel/docs/#unsynchronized-ethereum-nodes) for eth_getLogs

### v1.5 {#changelog-v1.5}

`4EE1 2024-05-01`

- bugfix: reorg may halt progress until restart. [more details](https://github.com/indexsupply/code/commit/4ee10c4789af31a054f96f968ed038db3aa3501f)

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

For Mac
```
curl -LO https://indexsupply.net/bin/1.6/darwin/arm64/shovel
chmod +x shovel
```

For Linux
```
curl -LO https://indexsupply.net/bin/1.6/linux/amd64/shovel
chmod +x shovel
```

Test
```
./shovel -version
v1.6 582d
```

The first part of this command prints a version string (which is also a git tag) and the first two bytes of the latest commit that was used to build the binaries.

### Build From Source

[Install the Go toolchain](https://go.dev/doc/install)

Clone the repo

```
git clone https://github.com/indexsupply/code.git shovel
```

Build

```
cd shovel
go run ./cmd/shovel
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


## Config

Shovel’s primary UI is the JSON Config file. In this file you specify a Postgres URL, Ethereum Sources, and Integrations (descriptions of the data to be indexed). The [TypeScript](#typescript) package may help manage complex configurations, but for simple tasks a basic JSON file will work.

Certain fields within Config may contain a `$` prefixed string which instructs Shovel to read the values from the OS’s environment. Read [Environment Variables](#config-environment-variables) for more detail.

This section will summarize the top level Config objects:

- Postgres
- Ethereum
- Integrations

The complete Config reference can be found [here.](#config-object)

### Postgres

Shovel connects to a single Postgres server. Shovel uses a Postgres schema named `shovel` to keep its internal bookkeeping tables. User defined tables are stored in the public schema. You can ask Shovel to print the user defined schema to `stdout` using the `--print-schema` flag. More details in [Printing the Schema](#printing-the-schema).

Shovel uses a `pg_advisory_xact_lock` to ensure only a single shovel can index a particular block(s) at a time.

Postgres is configured with a database URL. It will respect the SSL Mode defined in the URL. See this [link](https://www.postgresql.org/docs/current/libpq-ssl.html#LIBPQ-SSL-SSLMODE-STATEMENTS) for more details.

### Ethereum

A single Shovel process can connect to many Ethereum Sources. Each Source is identified by name and includes a Chain ID and a URL. Shovel uses the following HTTP JSON RPC API methods:

1. `eth_getBlockByNumber`
2. `eth_getLogs`
3. `eth_getBlockReceipts`
4. `trace_block`

Shovel will choose the RPC method depending on the integration’s data requirements. See [Data](#data) for a table outlining the data that you can index, its required API, and the associated performance cost.

Upon startup, Shovel will use `eth_getBlockByNumber` to find the latest block. It will then compare the response with its latest block in Postgres to figure out where to begin. While indexing data, Shovel will make batched calls to `eth_getBlockByNumber`, batched calls to `eth_getBlockReceipts`, and single calls to `eth_getLogs` depending on the configured `batch_size` and the RPC methods required to index the data.

### Integrations

Integrations combine: a Postgres table, one or more Ethereum Sources, and the desired data to be indexed from an Ethereum: block, transaction, receipt, log, or trace. Shovel is able to concurrently run hundreds of integrations. An integration references one or more Sources. This is useful when you want to index a particular set of data across multiple chains. Each Integration/ Source reference contains the name of the Ethereum Source and an optional start / stop value. If the start value is empty Shovel starts at block 1. If stop value is empty Shovel indexes data indefinitely. Shovel keeps track of the Integration/Source progress in its shovel.ig_updates table.

<hr>

## TypeScript

Since the Shovel JSON config contains a declarative description of all the data that you are indexing, it can become large and repetitive. If you would like an easier way to manage this config, you can use the TypeScript package which includes type definitions for the config structure. This will allow you to use loops, variables, and everything else that comes with a programming language to easily create and manage your Shovel config.

NPM package: https://npmjs.com/package/@indexsupply/shovel-config

TS docs: https://jsr.io/@indexsupply/shovel-config

Source: https://github.com/indexsupply/code/tree/main/shovel-config-ts

<details>
<summary>Example</summary>


Install the `shovel-config` TS package

```
bun install @indexsupply/shovel-config
```

Create a file named `shovel-config.ts` and add the following to the file:

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

Run the TS file and save its output to a file for Shovel to read.

```
bun run shovel-config.ts > config.json
shovel -config config.json
```
</details>

<hr>

## Performance

There are 2 dependent controls for Shovel performance:

1. The data you are indexing in your Integration
2. `batch_size` and `concurrency` defined in `eth_sources` Config

### Data Selection

The more blocks we can process per RPC request (`batch_size`) the better. If you can limit your Integration to using data provided by `eth_getLogs` then Shovel is able to reliably request up to 2,000 blocks worth of data in a single request. `eth_getLogs` provides:

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

### Concurrency

Shovel uses `batch_size` and `concurrency` defined in the Ethereum Source object to setup its concurrency.


If you are only indexing data provided by the logs, you can safely request 2,000 blocks worth of logs in a single request.

```
"eth_sources": [{..., "batch_size": 2000, "concurrency": 1}],
```

You could further increase your throughput by increasing concurrency:

```
"eth_sources": [{..., "batch_size": 4000, "concurrency": 2}],
```

However, if you are requesting data provided by the block header or by the block bodies (ie `tx_value`) then you will need to reduce your `batch_size` to a maximum of 100

```
"eth_sources": [{..., "batch_size": 100, "concurrency": 1}],
```

You may see '429 too many request' errors in your logs as you are backfilling data. This is not necessarily bad since Shovel will always retry. In fact, the essence of Shovel's internal design is one big retry function. But the errors may eventually be too many to meaningfully make progress. In this case you should try reducing the `batch_size` until you no longer see high frequency errors.

<hr>

## Reliability

Shovel's goal is for the operator to forget that it's running.
Once you establish you Config, you should be able to set and forget.

### Simplicity

There is an extraordinary focus on keeping simple code and abstractions. Most of the core functionality is a few hundred LOC in several different files.

### Crash Only Design

Shovel attempts to be [Crash Only Software](https://www.usenix.org/legacy/events/hotos03/tech/full_papers/candea/candea.pdf).

Shovel carefully uses PG transactions such that any crash (operator initiated or otherwise) will leave the database in a good state. When Shovel restarts, it will pick right back up where it left off.

Shovel's main function is called `Converge`. Converge will find the latest block from the Ethereum Source and the latest block from the Postgres database, and then index the delta. If anything goes wrong during `Converge` we simply roll back the transaction and try again. This means that if the Ethereum Source is giving errors, Shovel will continue retrying indefinitely.

For details on how Shovel uses Postgres, see [Task Updates Table](#task-updates-table).

Shovel is effectively one big retry loop.

### Multiple Shovel Instances

Shovel makes use of a `pg_advisory_xact_lock` each time it attempts to index data. This means that you can have multiple Shovel instances running the same Config, connected to the same Postgres database, and you should observe that work is randomly distributed across the Shovel instances.

### Unsynchronized Ethereum Nodes

Shovel tracks the head of the chain by asking the Ethereum Source for its latest block. If the Source responds with a block that is ahead of Shovel's latest block then Shovel will begin to index the new block. For Integrations that need logs, Shovel will immediately ask for the logs of the new block. 3rd party Ethereum API providers may load balance RPC requests across a set of unsynchronized nodes. In rare cases, the `eth_getLogs` request will be routed to a node that doesn't have the latest block.

Shovel mitigates this problem by requesting logs using a batch request that includes: `eth_getLogs` and `eth_getBlockByNumber`. Shovel tests the `eth_getBlockByNumber` response to ensure the node serving the `eth_getLogs` request has processed the requested block. This solution assumes the node provider does not separate the batch.

Shovel logs an error when the node provider is unsynchronized

```
error=getting logs: eth backend missing logs for block
```


<hr>

## Monitoring

Shovel has 3 monitoring interfaces: Prometheus, logs, and a Dashboard.

### Prometheus

Shovel provides a `/metrics` api that prints the following Prometheus metrics:

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

# HELP shovel_delta number of blocks between the source and the shovel database
# TYPE shovel_delta gauge
shovel_delta{src="mainnet"} 0
```

This endpoint will iterate through all the [Ethereum Sources](#ethereum-sources) and query for the latest block on both the Source and the `shovel.task_updates` table. Each Source will use a separate Prometheus label.

This endpoint is rate limited to 1 request per second.

### Logging

**msg=prune-task** This indicates indicates that Shovel has pruned the task updates table (`shovel.task_updates`). Each time that a task (backfill or main) indexes a batch of blocks, the latest block number is saved in the table. This is used for unwinding blocks during a reorg. On the last couple hundred of blocks are required and so Shovel will delete all but the last couple hundred records.

**v=d80f** This is the git commit that was used to build the binary. You can see this commit in the https://github.com/indexsupply/code repo by the following command

**nblocks=1** The number of blocks that were processed in a processing loop. If you are backfilling then this value will be min(batch_size/concurrency, num_blocks_behind). Otherwise, during incremental processing, it should be 1.

**nrpc=2** The number of JSON RPC requests that were made during a processing loop. In most cases the value will be 2. 1 to find out about the latest block and another to get the logs.

**nrows=0** The number of rows inserted into Postgres during the processing loop. Some blocks won't match your integrations and the value will be 0. Otherwise it will often correspond to the number of transactions of events that were matched during a processing loop.

```
git show d80f --stat
commit d80f21e11cfc68df06b74cb44c9f1f6b2b172165 (tag: v1.0beta)
Author: Ryan Smith <r@32k.io>
Date:   Mon Nov 13 20:54:17 2023 -0800

    shovel: ui tweak. update demo config

 cmd/shovel/demo.json  | 76 ++++++++++++++++++++++++++-------------------------
 shovel/web/index.html |  3 ++
 2 files changed, 42 insertions(+), 37 deletions(-)
```

### Dashboard

Shovel comes with a dashboard that can be used to:

1. Monitor the status of Shovel
2. Add new Ethereum Sources
3. Add new Integrations

Since the dashboard can affect the operations of Shovel it requires authentication. Here is how the authentication works:

By default non localhost requests will require authentication. This is to prevent someone accidentally exposing their unsecured Shovel dashboard to the internet.

By default localhost requests will require no authentication.

When authentication is enabled (by default or otherwise) the password will be either:

1. Set via the Config
2. Randomly generated when undefined in Config

To set a password in the Config

```
{..., "dashboard": {"root_password": ""}}
```

If the Config does not contain a password then Shovel will print the randomly generated password to the logs when a login web request is made. It will look like this

```
... password=171658e9feca092b msg=random-temp-password ...
```

Localhost authentication may be desirable for Shovel developers wanting to test the authentication bits. This is achieved with the following config
```
{..., "dashboard": { "enable_loopback_authn": true }}
```

Authentication can be disabled entirely via:
```
{..., "dashboard": {"disable_authn": true}}
```

<hr>

# Reference

The rest of this page contains detailed reference data.

<hr>

## Data {.reference}

Shovel indexes data from: blocks, transactions, receipts, logs, ABI encoded event logs, and traces. To index ABI enocded event logs, you will use the [event config](#config-integrations-event). Use the [block config](#config-integrations-block) To index data from the: block, transaction, or receipt.

An integration that indexes `trace_` data cannot also index ABI encoded event data.

Shovel will optimize its Ethereum JSON RPC API method choice based on the data that your integration requires. Integrations that only require data from `eth_getLogs` will be the most performant since `eth_getLogs` can filter and batch in ways that the other Eth APIs cannot. Whereas `eth_getBlockReceipts` and `trace_block` are extremely slow. Keep in mind that integrations are run independently so that you can partition your workload accordingly.


| Name                   | Eth Type        | Postgres Type    | Eth API          |
|------------------------|-----------------|------------------|------------------|
| chain_id               | int             | int              | `n/a`            |
| block_num              | numeric         | numeric          | `l, h, b, r, t`  |
| block_hash             | bytea           | bytea            | `l, h, b, r, t`  |
| block_time             | int             | int              | `h, b`           |
| tx_hash                | bytes32         | bytea            | `l, h, b, r, t`  |
| tx_idx                 | int             | int              | `l, h, b, r, t`  |
| tx_signer              | address         | bytea            | `b, r`           |
| tx_to                  | address         | bytea            | `b, r`           |
| tx_value               | uint256         | numeric          | `b, r`           |
| tx_input               | bytes           | bytea            | `b, r`           |
| tx_type                | byte            | int              | `b, r`           |
| tx_status              | byte            | int              | `r`              |
| tx_gas_used            | uint64          | bigint           | `r`              |
| tx_effective_gas_price | uint256         | numeric          | `r`              |
| log_idx                | int             | int              | `l, r`           |
| log_addr               | address         | bytea            | `l, r`           |
| event data             | n/a             | n/a              | `l`              |
| trace_action_call_type | string          | text             | `t`              |
| trace_action_idx       | int             | int              | `t`              |
| trace_action_from      | address         | bytea            | `t`              |
| trace_action_to        | address         | bytea            | `t`              |
| trace_action_value     | uint256         | numeric          | `t`              |

The Eth API can be one of (in asc order of perf cost):

- `l`: eth_getLogs
- `h`: eth_getBlockByNumber(no txs)
- `b`: eth_getBlockByNumber(txs)
- `r`: eth_getBlockReceipts
- `t`: trace_block


<hr>

## Filters {.reference}

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

## Notifications {.reference}

Shovel may use the Postgres [NOTIFY](https://www.postgresql.org/docs/current/sql-notify.html) command to send notifications when new rows are added to a table. This is useful if you want to provide low latency updates to clients. For example, you may have a browser client that opens an HTTP SSE connection to your web server. Your web server can use Postgres' [LISTEN](https://www.postgresql.org/docs/current/sql-listen.html) command to wait for new rows added to an Integration’s table. When the web server receives a notification, it can use data in the notification’s payload to quickly send the update to the client via the HTTP SSE connection.


There is a slight performance cost to using notifications. The cost should be almost 0 when Shovel is processing latest blocks but it may be non-zero when backfilling data. A couple of performance related things to keep in mind:

1. If there are no listeners then the notifications are instantly dropped
2. You can omit the notification config if you are doing a backfill and then add it once things are in the steady state

To configure notifications on an Integration, specify a notification object in the [Integration’s Config](#config-integrations-notification).

<hr>

## Schema {.reference}

Shovel uses the public schema to create tables defined in the Config. Shovel also keeps a set of internal tables in a schema named `shovel`. These tables help Shovel keep track of indexer progress and aid in crash recovery.

### Sharing Tables

It is possible for Integration's to share tables. In this case, Shovel will create the table using a union of the columns defined in each integration. This means that some rows in the table will have NULL values.

### Adding & Removing Columns

Columns can be added or removed to an Integration even if the Integration has already indexed data.

If a column is removed from an [Integration's Table](#config-integrations-table-columns) then Shovel will no longer write to that column. However, the column will not be deleted from the table.

When a column is added, if the table was non-empty, the existing rows will have NULL values for the new column.

### Query Shovel's Latest Block

A SQL VIEW for finding the most recent block indexed, grouped by the Integration's Source. Since many Integrations can share a Source, and since each Integration can be working on different parts of the chain, this VIEW finds the min block number (after removing in-active Integrations) amongst the Integrations

```
select * from shovel.latest ;
 src_name |   num
----------+----------
 base     | 14090162
```

The VIEW is defined [here](https://github.com/indexsupply/code/blob/main/shovel/schema.sql#L118-L132).

### Print Schema

This `--print-schema` command flag will instruct Shovel to parse the Config and print the computed schema (derived from the Config's Integrations) to `stdout` command flag will instruct Shovel to parse the Config and print the computed schema (derived from the Config's Integrations) to `stdout`.

This can be run without a Postgres or Ethereum connection. It can be useful for testing your application or just exploring what data Shovel will provide.

```
shovel -config demo.json --print-schema
create table if not exists usdc(
	log_addr bytea,
	block_time numeric,
	"from" bytea,
	"to" bytea,
	value numeric,
	ig_name text,
	src_name text,
	block_num numeric,
	tx_idx int,
	log_idx int,
	abi_idx int2
);
create unique index if not exists u_usdc on usdc (
	ig_name,
	src_name,
	block_num,
	tx_idx,
	log_idx,
	abi_idx
);
```

### Task Updates Table

As Shovel indexes batches of blocks, each set of database operations are done within a Postgres transaction. This means that the internal tables and public integration tables are atomically updated. Therefore the database remains consistent in the event of a crash. For example, if there is a `shovel.task_update` record for block X then you can be sure that the integration tables contain data for block X --and vice versa.

The primary, internal table that Shovel uses to synchronize its work is the `shovel.task_updates` table:

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

<hr>

## Config Object {.reference}

This JSON Config is the primary UI for Shovel. The Config object holds the database URL, the Ethereum URL, and a specification for all the data that you would like to index.

<details>
<summary>Here is the expanded overview of the Config object</summary>
<div id="config-object-list">

- [`pg_url`](#config-pg-url)
- [`eth_sources[]`](#config-eth-sources)
    - [`name`](#config-eth-sources-name)
    - [`chain_id`](#config-eth-sources-chain-id)
    - [`url`](#config-eth-sources-url)
    - [`ws_url`](#config-eth-sources-ws-url)
    - [`poll_duration`](#config-eth-sources-poll-duration)
    - [`concurrency`](#config-eth-sources-concurrency)
    - [`batch_size`](#config-eth-sources-batch-size)
- [`integrations[]`](#config-integrations)
    - [`name`](#config-integrations-name)
    - [`enabled`](#config-integrations-enabled)
    - [`sources`](#config-integrations-sources)
        - [`name`](#config-integrations-sources-name)
        - [`start`](#config-integrations-sources-start)
        - [`stop`](#config-integrations-sources-stop)
    - [`table`](#config-integrations-table)
        - [`name`](#config-integrations-table-name)
        - [`columns[]`](#config-integrations-table-columns)
            - [`name`](#config-integrations-table-columns-name)
            - [`type`](#config-integrations-table-columns-type)
        - [`index[][]`](#config-integrations-table-index)
        - [`unique[][]`](#config-integrations-table-unique)
        - [`disable_unique`](#config-integrations-table-disable-unique)
    - [`notification`](#config-integrations-notification)
    - [`block[]`](#config-integrations-block)
        - [`name`](#config-integrations-block-name)
        - [`column`](#config-integrations-block-column)
        - [`filter_op`](#config-integrations-block-filter_op)
        - [`filter_arg`](#config-integrations-block-filter_arg)
        - [`filter_ref`](#config-integrations-block-filter_ref)
    - [`event`](#config-integrations-event)
        - [`name`](#config-integrations-event-name)
        - [`anonymous`](#config-integrations-event-anonymous)
        - [`inputs[]`](#config-integrations-event-inputs)
            - [`name`](#config-integrations-event-inputs-name)
            - [`indexed`](#config-integrations-event-inputs-indexed)
            - [`type`](#config-integrations-event-inputs-type)
            - [`column`](#config-integrations-event-inputs-column)
            - [`filter_op`](#config-integrations-event-inputs-filter-op)
            - [`filter_arg`](#config-integrations-event-inputs-filter-arg)
            - [`filter_ref`](#config-integrations-event-inputs-filter-ref)
- [`dashboard`](#config-dashboard)
    - [`root_password`](#config-dashboard-root-password)
    - [`enable_loopback_authn`](#config-dashboard-enable-loopback-authn)
    - [`disable_authn`](#config-dashboard-disable-authn)

</div>
</details>

### Environment Variables {#config-environment-variables .reference}

Shovel reads certain config values from the os env when the value is a:
 `$`-prefixed string.

```
{"pg_url": "$PG_URL", ...}
```

The following fields are able to read from the os env:

- `pg_url`
- `eth_sources.name`
- `eth_sources.chain_id`
- `eth_sources.url`
- `eth_sources.ws_url`
- `eth_sources.poll_duration`
- `eth_sources.concurrency`
- `eth_sources.batch_size`
- `integrations[].sources[].name`
- `integrations[].sources[].start`
- `integrations[].sources[].stop`

This fields contain a mix of strings and numbers. In the case of numbers, Shovel will attempt to parse the string read from the env as a decimal.

If you use a `$`-prefixed string, and there is no corresponding environment variable, Shovel will exit-1 with an error messages.

If you use a `$`-prefixed string as a config val for a config that isn't on the list then Shovel will silently use the `$`-prefixed string as the value. This is a bug.

### `pg_url` {#config-pg-url .reference}

A single shovel connects to a single postgres. A connection is made using the URL defined in the config object.

```
{"pg_url": "postgres:///shovel", ...}
```

It is possible to use an environment variable in the config object so that you don't have to embed your database password into the file.

```
{"pg_url": "$DATABASE_URL",...}
```

Any value that is prefixed with a `$` will instruct Shovel to read from the environment. So something like `$PG_URL` works too.

### `eth_sources[]` {#config-eth-sources .reference}

An array of Ethereum Sources. Each integration will reference a Source in this array. It is fine to have many sources with the same chain_id and url so long as the name is unique.

### `eth_sources[].name` {#config-eth-sources-name .reference}

A unique name identifying the Ethereum source. This will be used throughout the system to identify the source of the data. It is also used as a foreign key in `integrations[].sources.name`

### `eth_sources[].chain_id` {#config-eth-sources-chain-id .reference}

There isn't a whole lot that depends on this value at the moment (ie no crypto functions) but it will end up in the integrations tables.

### `eth_sources[].url` {#config-eth-sources-url .reference}

A URL that points to a HTTP JSON RPC API. This can be a local {G,R}eth node or a Quicknode.

### `eth_sources[].ws_url` {#config-eth-sources-ws-url .reference}

An optional URL that points to a Websocket JSON RPC API. If this URL is set Shovel will use the websocket to get the _latest_ block instead of calling `eth_getBlockByNumber`. If the websocket fails for any reason Shovel will fallback on the HTTP based API.

### `eth_sources[].poll_duration` {#config-eth-sources-poll-duration .reference}

A string that can be parsed into an interval. For example:

- `12s`
- `500ms`

Default: `1s`.

The amount of time to wait before checking the source for a new block. A lower value (eg 100ms) will increase the total number of requests that Shove will make to your node. This may count against your rate limit. A higher value will reduce the number of requests made.

If an error is encountered, Shovel will sleep for 1s before retrying. This is not yet configurable.

### `eth_sources[].batch_size` {#config-eth-sources-batch-size .reference}

On each loop iteration, Shovel will compute the delta between the local PG and the source node. Shovel then will index `max(batch_size, delta)` blocks over `concurrency` go-routines (threads).

If the integration uses: logs, headers, or blocks each thread will group `batch_size/concurrency` block requests into a single batched RPC request.

Otherwise, the integration will make `batch_size/concurrency` requests to get the data for the blocks.

### `eth_sources[].concurrency` {#config-eth-sources-concurrency .reference}

The maximum number of concurrent threads to run within a task. This value relates to `batch_size` in that for each task, the `batch_size` is partitioned amongst `concurrency` threads.

### `integrations[]` {#config-integrations .reference}

Shovel will create a Task for each Integration/Source pair in this array.

### `integrations[].name` {#config-integrations-name .reference}

A good, concise description of the integration. This value should not be changed. Records inserted into Postgres tables will set `ig_name` to this value.

### `integrations[].enabled` {#config-integrations-enabled .reference}

Must be true or false. Shovel will only load integrations that are enabled. This is meant to be a quick way to disable an integration while keeping the config in the file.

### `integrations[].sources` {#config-integrations-sources .reference}

An integration can reference many Sources by the Source's name. This is also the place to define the start/stop block numbers for the integration.

### `integrations[].sources[].name` {#config-integrations-sources-name .reference}

References an Ethereum Source using the [Ethereum Source name](#config-eth-sources-name).

### `integrations[].sources[].start` {#config-integrations-sources-start .reference}

Optional.

When left undefined, Shovel will start at the highest block in the Ethereum Source at the time of deployment.

When defined, shovel will start processing using the following logic:

```
let db_latest = query_task_updates_table()
let next_to_process = max(db_latest, integrations[].sources[].start)
```

### `integrations[].sources[].stop` {#config-integrations-sources-stop .reference}

Optional.

This can be helpful if you want to index a specific block range.

### `integrations[].table` {#config-integrations-table .reference}

Each Integration must define a Table. It is possible for many integrations to write to the same table. If Integrations with disjoint columns share a table the table is created with a union of the columns.

### `integrations[].table.name` {#config-integrations-table-name .reference}

Postgres table name. Must be lower case. Typically with underscores instead of hyphens.

### `integrations[].table.columns[]` {#config-integrations-table-columns .reference}

Array of columns that will be written to by the integration. Each column requires a name and a type.

Shovel will not set default values for new columns. This means table modification are cheap and the table may contain null values.

The following columns are required and will be added if not defined

```
[
  {"name": "ig_name", "type": "text"},
  {"name": "src_name", "type": "text"},
  {"name": "block_num", "type": "numeric"},
  {"name": "tx_idx", "type": "numeric"}
]
```

If the integration uses a log, defaults include:

```
{"name": "log_idx", "type": "int"}
```

If the integration uses a decoded log, defaults include:

```
{"name": "abi_idx", "type": "int"}
```

If the integration uses `trace_*`, defaults include:

```
{"name": "trace_action_idx", "type": "int"}
```

### `integrations[].table.columns[].name` {#config-integrations-table-columns-name .reference}

Postgres column name. Must be lower case. Typically with underscores instead of hyphens.

### `integrations[].table.columns[].type` {#config-integrations-table-columns-type .reference}

Must be one of: `bool`, `byte`, `bytea`, `int`, `numeric`, `text`.

### `integrations[].table.index[]` {#config-integrations-table-index .reference}

Array of an array of strings representing column names that should be used in creating a B-Tree index. Each column name in this array must be represented in the table's `columns` field. A column name may be suffixed with `ASC` or `DESC` to specify a sort order for the index.

```
{..., "index": [["from", "block_num ASC"]], ...}
```

### `integrations[].table.unique[]` {#config-integrations-table-unique .reference}

Array of an array of strings representing column names that should be combined into a unique index. Each column name in this array must be represented in the table's `columns` field.

The name of the index will be `u_` followed by the column names joined with an `_`. For example: `u_chain_id_block_num`

If no `unique` field is defined in the config, and unless `disable_unique = true`, a default unique index will be created with the table's required columns.

### `integrations[].table.disable_unique` {#config-integrations-table-disable-unique .reference}

There are some good reasons for not wanting to create an index by default. One such reason may be faster backfilling. In this case, you don't want any indexes (although you have to create the indexes at some point.) But in these cases you can disable default index creation. Shovel checks this value on startup so if `disable_unique` is removed then the default index will be created.

### `integrations[].notification` {#config-integrations-notification .reference}

Optional.

See [Notifications](#notifications) for overview.

### `integrations[].notification.columns[]` {#config-integrations-notification-columns .reference}

A list of strings that reference column names. Column names must be previously defined the integration's [table](#table) config. The columns values are serialized to text (hex when binary) and encoded into a comma separated list and placed in the notification's payload. The order of the payload is the order used in the `columns` list.

With this config, and when Shovel is inserting new data into the foo table, it will send a notification with the following data:

```
NOTIFY "mainnet-foo" '$block_num,$a,$b'
```

### `integrations[].block[]` {#config-integrations-block .reference}

An array of data to index that comes from: blocks, transactions, receipts, logs, or traces. See [Event](#config-integrations-event) for indexing ABI decoded event logs.

If an Integration defines a `block` object and not an `event` object then Shovel will not take the time to download event logs. Similarly, Shovel downloads traces iff the `block` objects references `trace_*` fields.

### `integrations[].block[].name` {#config-integrations-block-name .reference}

The name of the block field to index. Must be defined in: [Data](#data)

### `integrations[].block[].column` {#config-integrations-block-column .reference}

Reference to the name of a [column](#config-integrations-table-columns-name) in the Integration's table.

### `integrations[].block[].filter_op` {#config-integrations-block-filter_op .reference}

See [filters](#filters) for more detail.

### `integrations[].block[].filter_arg ` {#config-integrations-block-filter_arg .reference}

See [filters](#filters) for more detail.

### `integrations[].block[].filter_ref` {#config-integrations-block-filter_ref .reference}

See [filters](#filters) for more detail.

### `integrations[].event` {#config-integrations-event .reference}

The event object is used for decoding logs/events. By adding an annotated ABI snippet to the event object, Shovel will match relevant logs, decode the logs using our optimized ABI decoder, and save the decoded result to the integration's table.

The event object is a superset of [Solidity's ABI JSON spec](https://docs.soliditylang.org/en/v0.8.23/abi-spec.html#json). Shovel will read a few additional fields to determine how it will index an event. If an input defines a column, and that column name references a valid column in the integration's table, then the input's value will be saved in the table. Omitting a column value indicates that the input's value should not be saved.

The event name, and input type names can be used to construct the hashed event signature (eg topics[0]). In fact, this is one of the check's that Shovel will preform while it is deciding if it should process a log.

### `integrations[].event.name` {#config-integrations-event-name .reference}

The ABI defined event's name.

### `integrations[].event.type` {#config-integrations-event-type .reference}

Must be `"event"`

### `integrations[].event.anonymous` {#config-integrations-event-anonymous .reference}

The ABI defined event's `anonymous` value.

### `integrations[].event.inputs[]` {#config-integrations-event-inputs .reference}

The ABI defined list of event inputs. Fully compatible with the ABI spec. And therefore can contain arbitrarily nested data.

### `integrations[].event.inputs[].indexed` {#config-integrations-event-inputs-indexed .reference}

Copied from the ABI. Indicates if the data is stored in: `topics[4][32]`

### `integrations[].event.inputs[].name` {#config-integrations-event-inputs-name .reference}

Optional.

Not encoded in the event log nor is it consumed by Shovel.

### `integrations[].event.inputs[].type` {#config-integrations-event-inputs-type .reference}

The ABI defined input type. (eg `uint256`, `bytes32`)

### `integrations[].event.inputs[].column` {#config-integrations-event-inputs-column .reference}

Setting this value indicates that you would like this decoded value to be saved in the Integration's table. Therefore, the column value must reference a [column](#config-integrations-table-columns-name) defined in the integration's table.

The user should also take care to match the Table's column type with this type. For example: `uint256` -> `numeric`, `address` -> `bytea`

### `integrations[].event.inputs[].filter_op` {#config-integrations-event-inputs-filter-op .reference}

See [filters](#filters) for more detail.

### `integrations[].event.inputs[].filter_arg` {#config-integrations-event-inputs-filter-arg .reference}

See [filters](#filters) for more detail.

### `integrations[].event.inputs[].filter_ref` {#config-integrations-event-inputs-filter-ref .reference}

See [filters](#filters) for more detail.

### `dashboard` {#config-dashboard .reference}

Shovel includes a dashboard that listens on port `8546` or whatever address is specified using the command line flag `-l`.

```
./shovel -l :8080
```

The dashboard authenticates request except from localhost.

### `dashboard.root_password` {#config-dashboard-root-password .reference}

If the root password is unset, Shovel will generate a new, random password on startup.

This new password will be printed to the logs each time you make a request so that the operator can gain access.

```
{..., "dashboard": {..., "root_password": ""}}
```

### `dashboard.enable_loopback_authn` {#config-dashboard-enable-loopback-authn .reference}

This may be useful if you are developing features on Shovel and want to test the authentication code using localhost.

```
{..., "dashboard": {..., "enable_loopback_authn": true}}
```

### `dashboard.disable_authn` {#config-dashboard-enable-disable-authn .reference}

If, for whatever reason, you want to disable authentication entirely.

```
{..., "dashboard": {..., "disable_authn": true}}
```

<hr>

You've reached the end. Thank you for reading.
