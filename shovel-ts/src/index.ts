type Hex = `0x${string}`;

export type PGColumnType =
  | "bool"
  | "bytea"
  | "int"
  | "numeric"
  | "text"
  | "timestamp";

export type Column = {
  name: string;
  type: PGColumnType;
};

export type Table = {
  name: string;
  columns: Column[];
};

export type FilterOp = "contains" | "!contains";

export type FilterReference = {
  integration: string;
  column: string;
};

export type Filter = {
  op: FilterOp;
  arg: Hex[];
};

export type BlockData = {
  name: "block_hash" | "block_num" | "block_time";
  column: string;
};

export type EthSourceReference = {
  name: string;
  startBlock: BigInt;
};

export type EventIntput = {
  indexed: boolean;
  name: string;
  type: string;
};

export type EventData = {
  name: string;
  anonamous: boolean;
};

export type Integration = {
  name: string;
  enabled: boolean;
  source: EthSourceReference;
  table: Table;
  block: BlockData[];
  event: EventData;
};

export type EthSource = {
  name: string;
  url: string;
  chainId: number;
  concurrency?: number;
  batchSize?: number;
};

export type Config = {
  pgURL: string;
  ethSources: EthSource[];
  integrations: Integration[];
};

export function makeConfig(args: {
  pgURL: string;
  sources: EthSource[];
  integrations: Integration[];
}): Config {
  //TODO validation
  return {
    pgURL: args.pgURL,
    ethSources: args.sources,
    integrations: args.integrations,
  };
}
