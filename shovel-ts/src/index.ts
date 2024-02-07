type hex = `0x${string}`;

type DbType = "bool" | "bytea" | "int" | "numeric" | "text" | "timestamp";

type Column = {
    name: string;
    type: DbType;
}

export type Table = {
    name: string;
    columns: Column[];
}

type FilterOp = "contains" | "!contains";

type FilterReference = {
    integration: string;
    column: string;
}

type Filter = {
    op: FilterOp;
    arg: hex[];
}

type BlockData = {
    name: "block_hash" | "block_num" | "block_time";
    column: string;
}

type EthSourceReference = {
    name: string;
    startBlock: BigInt;
}

type EventIntput = {
    indexed: boolean;
    name: string;
    type: string;
}

type EventData = {
    name: string;
    anonamous: boolean;
}

export type Integration = {
    name: string;
    enabled: boolean;
    source: EthSourceReference;
    table: Table;
    block: BlockData[];
    event: EventData;
}

export type EthSource = {
    name: string;
    url: string;
    chainId: number;
    concurrency?: number;
    batchSize?: number;
}

export type Config = {
    pgURL: string;
    ethSources: EthSource[];
    integrations : Integration[];
};

export function makeConfig(pgURL: string, sources: EthSource[], integrations: Integration[]): Config {
    //TODO validation
    return {
        pgURL: pgURL,
        ethSources: sources,
        integrations: integrations
    };
}