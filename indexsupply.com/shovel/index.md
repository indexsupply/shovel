<title>Index Supply / Shovel</title>

# [Index Supply](/) / Shovel

Shovel is an [open source][1] tool for synchronizing Ethereum data to your Postgres database. \
Own your blockchain data without vendor lock-in.

## Things you can do with Shovel

- Use **Ethereum**, **Base**, **Optimism**, **Arbitrum**, or any other EVM chain
- Connect to hosted nodes (Alchemy, Quicknode) or local nodes (Geth, Reth)
- Decode and save events to Postgres <ins>without custom functions</ins>
- Index Transactions and Block metadata too
- Stay in sync with the latest blocks across many chains
- Concurrently backfill data

Learn more in the **[docs](/shovel/docs).**

## Checkout this Demo

<div style="padding:64.64% 0 0 0;position:relative;"><iframe src="https://player.vimeo.com/video/884276989?badge=0&amp;autopause=0&amp;quality_selector=1&amp;player_id=0&amp;app_id=58479" frameborder="0" allow="autoplay; fullscreen; picture-in-picture" style="position:absolute;top:0;left:0;width:100%;height:100%;" title="Shovel Demo #1"></iframe></div><script src="https://player.vimeo.com/api/player.js"></script>

Shovel processes multiple chains and multiple integrations concurrently. It starts indexing the latest block right away and optionally indexes historical data in the background.

Shovel is configured using declarative JSON that maps the data you care about onto your Postgres tables. No need to write custom functions or subgraphs for the basics. Simple things are easy and complex things are possible.

<details>
	<summary><h2>ERC20 Transfer Example</h2></summary>
	<div class="quickstart">
	<code><pre>
{
  "pg_url": "postgres:///shovel",
  "eth_sources": [
    {
        "name": "mainnet",
        "chain_id": 1,
        "urls": ["https://ethereum-rpc.publicnode.com"]
    },
    {
        "name": "sepolia",
        "chain_id": 11155111,
        "urls": ["https://ethereum-sepolia-rpc.publicnode.com"]
    }
  ],
  "integrations": [
    {
      "name": "tokens",
      "enabled": true,
      "sources": [{"name": "mainnet"}, {"name": "sepolia"}],
      "table": {
        "name": "transfers",
          "columns": [
            {"name": "log_addr", "type": "bytea"},
            {"name": "block_time", "type": "numeric"},
            {"name": "from", "type": "bytea"},
            {"name": "to", "type": "bytea"},
            {"name": "value", "type": "numeric"}
          ]
      },
      "block": [
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
          {"indexed": true, "name": "from", "type": "address", "column": "from"},
          {"indexed": true, "name": "to", "type": "address", "column": "to"},
          {"indexed": false, "name": "value", "type": "uint256", "column": "value"}
        ]
      }
    }
  ]
}
	</pre></code>
	</div>
	<div class="quickstart">
	<code><pre>
shovel=# select * from transfers limit 1;
-[ RECORD 1 ]------------------------------------------
block_time | 1699912391
f          | \x66a89e05525bc2d4de6973ed2f30015f8ac4f165
t          | \x7e40bfd3d80e060cdd7b8970ec967eb8adf121f9
v          | 227300000
ig_name    | tokens
src_name   | mainnet
block_num  | 18565786
tx_idx     | 17
log_idx    | 0
abi_idx    | 0
log_addr   | \xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48
	</pre></code>
	</div>
</details>

<details>
    <summary><h2>Quickstart</h2></summary>

```
# assumes you have pg running
createdb shovel

# download demo config file
curl -LO https://raw.githubusercontent.com/indexsupply/code/main/cmd/shovel/demo.json

# download shovel
curl -LO https://indexsupply.net/bin/1.6/darwin/arm64/shovel
chmod +x shovel
./shovel -config demo.json
```
</details>

<details>
	<summary><h2>Arch. Diagram</h2></summary>
	<img src="https://indexsupply.com/shovel-diag.png" />
</details>

<hr />

## FAQ

Q: Is this similar to The Graph?
A: Yes. But there is no token model for Index Supply and Shovel is much simpler

Q: Is this similar to Dune?
A: You could build something like Dune with Shovel. But Shovel is like an OLTP system whereas Dune is an OLAP system. You wouldn't want to build a UI using Dune as your backend.

Q: How many and what kind of chains are compatible with Shovel?
A: Any chain that uses the Ethereum JSON RPC API and ABI encoding. You can easily index 10 different chains with a single Shovel instance. Maybe more.

[1]: https://github.com/indexsupply/code
