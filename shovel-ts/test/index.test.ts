import { expect, test } from "bun:test";
import { newConfig } from "../src/index";
import type { EthSource, Table } from "../src/index";

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
    let c = newConfig();
    c.integrations.push({
        name: "transfers",
        enabled: true,
        source: {
            name: mainnet.name,
            startBlock: 0,
        },
        table: transfersTable,
        block: [],
        event: {
            name: "Transfer",
            anonamous: false,
        },
    });
    expect(JSON.parse(JSON.stringify(c))).toEqual(JSON.parse(`{
        "pgURL":"",
        "ethSources":[],
        "integrations":[
            {
                "name": "transfers",
                "enabled": true,
                "source": {
                    "name": "mainnet",
                    "startBlock": 0
                },
                "table": {
                    "name": "transfers",
                    "columns": [
                        {
                            "name": "from",
                            "type": "address"
                        },
                        {
                            "name": "to",
                            "type": "address"
                        },
                        {
                            "name": "value",
                            "type": "uint256"
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
    }`))
});
