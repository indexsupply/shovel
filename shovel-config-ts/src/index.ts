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

export type EventIntput = {
  readonly indexed?: boolean;
  readonly name: string;
  readonly type: string;
  readonly components?: EventIntput[];

  column?: string;
  filter_op?: FilterOp;
  filter_arg?: Hex[];
  filter_ref?: FilterReference;
};

export type Event = {
  readonly name: string;
  readonly type: "event";
  readonly anonymous?: boolean;
  readonly inputs: readonly EventIntput[];
};

export type Source = {
  name: string;
  url: string;
  chainId: number;
  concurrency?: number;
  batchSize?: number;
};

export type SourceReference = {
  name: string;
  start: bigint;
};

export type Integration = {
  name: string;
  enabled: boolean;
  sources: SourceReference[];
  table: Table;
  block: BlockData[];
  event: Event;
};

export type Config = {
  pgURL: string;
  sources: Source[];
  integrations: Integration[];
};

export function makeConfig(args: {
  pgURL: string;
  sources: Source[];
  integrations: Integration[];
}): Config {
  //TODO validation
  return {
    pgURL: args.pgURL,
    sources: args.sources,
    integrations: args.integrations,
  };
}

export function toJSON(c: Config): string {
  const bigintjson = (_key: any, value: any) =>
    typeof value === "bigint" ? value.toString() : value;
  return JSON.stringify(
    {
      pg_url: c.pgURL,
      eth_sources: c.sources,
      integrations: c.integrations,
    },
    bigintjson
  );
}