# Shovel Config TS

Use this TS package to script the creation of your Shovel JSON config.

NPM package here: https://www.npmjs.com/package/@indexsupply/shovel-config

Package docs here: https://jsr.io/@indexsupply/shovel-config

Example:

```ts
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
  url: "https://ethereum.publicnode.com",
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
