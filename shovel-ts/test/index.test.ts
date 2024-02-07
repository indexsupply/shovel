import { expect, test } from "bun:test";
import { makeConfig } from "../src/index";
import type { EthSource, Table, Integration } from "../src/index";

test("XXX", () => {
    const transfersTable: Table = {
        name: "transfers",
        columns: [
            { name: "from", type: "bytea" },
            { name: "to", type: "bytea" },
            { name: "value", type: "numeric" },
        ]
    };
    const mainnet: EthSource = {
        name: "mainnet",
        url: "https://ethereum.publicnode.com",
        chainId: 1,
    };
    const integrations: Integration[] = [];

    integrations.push({
        name: "transfers",
        enabled: true,
        source: {
            name: mainnet.name,
            startBlock: 0n,
        },
        table: transfersTable,
        block: [],
        event: {
            name: "Transfer",
            anonamous: false,
        },
    });

    const c = makeConfig("", [], integrations);
    expect(c).toEqual({
        "pgURL":"",
        "ethSources":[],
        "integrations":[
            {
                "name": "transfers",
                "enabled": true,
                "source": {
                    "name": "mainnet",
                    "startBlock": 0n
                },
                "table": {
                    "name": "transfers",
                    "columns": [
                        {
                            "name": "from",
                            "type": "bytea"
                        },
                        {
                            "name": "to",
                            "type": "bytea"
                        },
                        {
                            "name": "value",
                            "type": "numeric"
                        }
                    ]
                },
                "block": [],
                "event": {
                    "name": "Transfer",
                    "anonamous": false
                }
            }
        ]
    });
});