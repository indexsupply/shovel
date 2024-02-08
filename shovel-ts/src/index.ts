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

export type BlockDataOptions =
  | "src_name"
  | "ig_name"
  | "chain_id"
  | "block_hash"
  | "block_num"
  | "block_time"
  | "tx_hash"
  | "tx_idx"
  | "tx_signer"
  | "tx_to"
  | "tx_value"
  | "tx_input"
  | "tx_type"
  | "tx_status"
  | "log_idx"
  | "log_addr";

export type BlockData = {
  name: BlockDataOptions;

  column: string;
  filter_op?: FilterOp;
  filter_arg?: Hex[];
  filter_ref?: FilterReference;
};

export type EthSourceReference = {
  name: string;
  startBlock: BigInt;
};

export type EventIntput = {
  indexed: boolean;
  name: string;
  type: string;
  components?: EventIntput[];

  column?: string;
  filter_op?: FilterOp;
  filter_arg?: Hex[];
  filter_ref?: FilterReference;
};

export type Event = {
  name: string;
  anonamous: boolean;
  inputs: EventIntput[];
};

export type Integration = {
  name: string;
  enabled: boolean;
  source: EthSourceReference;
  table: Table;
  block: BlockData[];
  event: Event;
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
