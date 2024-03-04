// string values with `$` prefix instruct shovel to read
// the values from the evnironment at runtime
type EnvRef = `$${string}`;

type Hex = `0x${string}`;

export type PGColumnType =
  | "bigint"
  | "bool"
  | "byte"
  | "bytea"
  | "int"
  | "numeric"
  | "smallint"
  | "text";

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

export type EventInput = {
  readonly indexed?: boolean;
  readonly name: string;
  readonly type: string;
  readonly components?: EventInput[];

  column?: string;
  filter_op?: FilterOp;
  filter_arg?: Hex[];
  filter_ref?: FilterReference;
};

export type Event = {
  readonly name: string;
  readonly type: "event";
  readonly anonymous?: boolean;
  readonly inputs: readonly EventInput[];
};

export type Source = {
  name: string;
  url: string;
  chain_id: EnvRef | number;
  concurrency?: EnvRef | number;
  batch_size?: EnvRef | number;
};

export type SourceReference = {
  name: string;
  start: EnvRef | bigint;
};

export type Integration = {
  name: string;
  enabled: boolean;
  sources: SourceReference[];
  table: Table;
  block: BlockData[];
  event: Event;
};

export type Dashboard = {
  root_password?: string;
  enable_loopback_authn?: EnvRef | boolean;
  disable_authn?: EnvRef | boolean;
};

export type Config = {
  dashboard: Dashboard;
  pg_url: string;
  sources: Source[];
  integrations: Integration[];
};

export function makeConfig(args: {
  dashboard?: Dashboard;
  pg_url: string;
  sources: Source[];
  integrations: Integration[];
}): Config {
  //TODO validation
  return {
    dashboard: args.dashboard || {},
    pg_url: args.pg_url,
    sources: args.sources,
    integrations: args.integrations,
  };
}

export function toJSON(c: Config, space: number = 0): string {
  const bigintjson = (_key: any, value: any) =>
    typeof value === "bigint" ? value.toString() : value;
  return JSON.stringify(
    {
      dashboard: c.dashboard,
      pg_url: c.pg_url,
      eth_sources: c.sources,
      integrations: c.integrations,
    },
    bigintjson,
    space
  );
}
