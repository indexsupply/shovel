import { expect, test } from "bun:test";
import { makeConfig, toJSON } from "../src/index";
import type { Source, Table, Integration } from "../src/index";

test("makeConfig", () => {
  const transfersTable: Table = {
    name: "transfers",
    columns: [
      { name: "from", type: "bytea" },
      { name: "to", type: "bytea" },
      { name: "value", type: "numeric" },
    ],
  };
  const mainnet: Source = {
    name: "mainnet",
    url: "https://ethereum.publicnode.com",
    chain_id: 1,
  };
  const integrations: Integration[] = [
    {
      name: "transfers",
      enabled: true,
      sources: [{ name: mainnet.name, start: 0n }],
      table: transfersTable,
      block: [],
      event: {
        type: "event",
        name: "Transfer",
        anonymous: false,
        inputs: [{ indexed: true, name: "from", type: "address" }],
      },
    },
  ];
  const c = makeConfig({
    pg_url: "",
    sources: [mainnet],
    integrations: integrations,
  });

  expect(c).toEqual({
    dashboard: {},
    pg_url: "",
    sources: [
      {
        name: "mainnet",
        url: "https://ethereum.publicnode.com",
        chain_id: 1,
      },
    ],
    integrations: [
      {
        name: "transfers",
        enabled: true,
        sources: [
          {
            name: "mainnet",
            start: 0n,
          },
        ],
        table: {
          name: "transfers",
          columns: [
            {
              name: "from",
              type: "bytea",
            },
            {
              name: "to",
              type: "bytea",
            },
            {
              name: "value",
              type: "numeric",
            },
          ],
        },
        block: [],
        event: {
          type: "event",
          name: "Transfer",
          anonymous: false,
          inputs: [
            {
              indexed: true,
              name: "from",
              type: "address",
            },
          ],
        },
      },
    ],
  });
});