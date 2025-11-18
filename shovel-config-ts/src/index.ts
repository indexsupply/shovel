/**
 * This module contains functions to script the creation of a Shovel Config
 * @module
 */

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

/**
 * An IndexStatement is an array of strings. Each
 * string reprsents a column name and may be followed
 * by ASC or DESC to specify the index sort order.
 */
export type IndexStatment = string[];

export type Table = {
  name: string;
  columns: Column[];
  index?: IndexStatment[];
};

export type FilterRefOp = "contains" | "!contains";

export type FilterArgOp = FilterRefOp | "eq" | "ne" | "gt" | "lt";

export type FilterReference = {
  integration: string;
  column: string;
};

export type Filter = {
  op: FilterRefOp;
  arg: string[];
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
  | "tx_gas_used"
  | "tx_gas_price"
  | "tx_effective_gas_price"
  | "tx_contract_address"
  | "tx_max_priority_fee_per_gas"
  | "tx_max_fee_per_gas"
  | "tx_nonce"
  | "log_addr"
  | "trace_action_call_type"
  | "trace_action_idx"
  | "trace_action_from"
  | "trace_action_to"
  | "trace_action_value";

/**
 * BlockData represents non-event data to index. Shovel can index
 * block, receipt, transaction, and log data in addition to abi
 * decoded event log data.
 */
export type BlockData = {
  name: BlockDataOptions;

  column: string;
} & ({
  filter_op?: FilterArgOp;
  filter_arg?: string[];
} | {
  filter_op?: FilterRefOp;
  filter_ref?: FilterReference;
});

/**
 * EventInput is a superset of the ABI JSON defintion for event
 * inputs. The additions to the standard are column, filter_op,
 * filter_arg, and filter_ref.
 *
 * These additions are instruction for Shovel so that it can map the
 * event data to your PG table.
 *
 * If column is omitted, then the event input field will not be saved.
 */
export type EventInput = {
  readonly indexed?: boolean;
  readonly name: string;
  readonly type: string;
  /**
   * internalType is not used by shovel
   * but is specified for easy copy/paste.
   */
  readonly internalType?: string;
  readonly components?: EventInput[];

  column?: string;
} & ({
  filter_op?: FilterArgOp;
  filter_arg?: string[];
} | {
  filter_op?: FilterRefOp;
  filter_ref?: FilterReference;
});

export type Event = {
  readonly name: string;
  readonly type: "event";
  readonly anonymous?: boolean;
  readonly inputs: readonly EventInput[];
};

/**
 * Consensus configuration for multi-provider validation.
 */
export type Consensus = {
  /**
   * Number of RPC providers to use for consensus validation.
   * Defaults to 1 if not specified.
   */
  providers?: EnvRef | number;
  /**
   * Number of providers that must agree for data to be accepted.
   * Defaults to 1 if not specified.
   * Must be <= providers.
   */
  threshold?: EnvRef | number;
  /**
   * Initial backoff duration when retrying after consensus failure.
   * Defaults to "2s" if not specified.
   */
  retry_backoff?: EnvRef | string;
  /**
   * Maximum backoff duration for retries.
   * Defaults to "30s" if not specified.
   */
  max_backoff?: EnvRef | string;
};

/**
 * Source represents an Ethereum HTTP JSON RPC API Provider.
 */
export type Source = {
  name: string;
  url: string;
  /**
   * Shovel will round-robin requests to these urls.
   * This may be helpful for reducing downtime.
   *
   * url is added to urls
   */
  urls: string[];
  chain_id: EnvRef | number;
  poll_duration?: EnvRef | string;
  concurrency?: EnvRef | number;
  batch_size?: EnvRef | number;
  /**
   * Consensus configuration for multi-provider validation.
   * When specified, shovel will query multiple providers and require
   * agreement before accepting data.
   */
  consensus?: Consensus;
};

export type SourceReference = {
  name: string;
  start: EnvRef | bigint;
};

export type Notification = {
	columns: string[];
};

export type Integration = {
  name: string;
  enabled: boolean;
  sources: SourceReference[];
  table: Table;
  notification?: Notification;
  block?: BlockData[];
  event?: Event;
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

/** @returns a stringified JSON representation of the Config.
 * Handles bigint serialization. Passes through the `space` parameter to `JSON.stringify`.
 * @param c - the Config to serialize
 * @param space - the number of spaces to use for indentation
 */
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
