# Shovel

This file serves as the primary onboarding document for developers working on Shovel. It provides essential context about the architecture, development workflows, and design principles.

## Project Overview

Shovel is an Ethereum blockchain indexer that extracts data from Ethereum nodes via JSON-RPC and stores it in PostgreSQL databases.The system uses declarative configuration to define what blockchain data to extract and how to transform it into database tables.

**Core value proposition:** Users can index any Ethereum event ortransaction data into PostgreSQL without writing indexing code - just provide a JSON configuration describing the data to extract.

## Quick Start

New to the codebase? Start here:

1. **Prerequisites:** Go 1.21+, PostgreSQL, Ethereum RPC endpoint(or use a public node)

2. **Build and run tests:**

   ```bash
   go build -o shovel ./cmd/shovel
   go test -p 1 ./...
   ```

3. **Try the demo config:**

   ```bash
   # Create a local PostgreSQL database
   createdb shovel

   # Run with demo config (indexes USDC transfers)
   ./shovel -config cmd/shovel/demo.json -v

   # View dashboard
   open http://localhost:8546
   ```

4. **Understand the architecture:**

   - Read the [Architecture](#architecture) section below
   - Examine `cmd/shovel/demo.json` to see a complete config
   - Review `shovel/task.go` to understand the convergence algorithm
   - Check `jrpc2/client.go` for RPC interaction patterns

5. **Make your first change:**
   - Follow a [Common Development Workflow](#common-development-workflows)
   - Run tests to verify: `go test -p 1 ./...`
   - Test with a real config against live data

## Development Commands

### Go (Main Application)

```bash
# Run all tests (must use -p 1 for sequential execution)
go test -p 1 ./...

# Run tests for a specific package
go test ./shovel
go test ./jrpc2
go test ./eth

# Run a single test by name
go test -p 1 ./shovel -run TestTaskConverge

# Verify dependencies are tidy (required before committing)
go mod tidy
git diff --name-only --exit-code  # Should show no changes

# Build the shovel binary
go build -o shovel ./cmd/shovel

# Run shovel with a config file
./shovel -config config.json

# Print database schema from config without running
./shovel -config config.json -print-schema

# Run with verbose logging (shows debug-level logs)
./shovel -config config.json -v

# Skip database migrations on startup (useful during development)
./shovel -config config.json -skip-migrate

# Specify custom dashboard listen address
./shovel -config config.json -l localhost:9000
```

### TypeScript Config Package (shovel-config-ts/)

```bash
# Install dependencies
cd shovel-config-ts
bun install

# Run tests
bun test

# Build the package
bun run build

# Publish to npm (requires appropriate permissions)
npm publish
```

## Common Development Workflows

### Adding a New Integration

1. Create or modify a JSON config file with the new integration definition
2. Use `-print-schema` to verify the generated SQL schema
3. Run shovel with the config to create tables and begin indexing
4. Monitor progress via dashboard at `localhost:8546`
5. Query the generated table to verify data

### Debugging Indexing Issues

1. Enable verbose logging: `-v` flag
2. Check dashboard for task status and error messages
3. Examine `shovel.task_updates` table for latest indexed blocks
4. Use `pprof` endpoints (`/debug/pprof/`) for performance profiling
5. Check RPC URL connectivity and rate limits

### Working on Core Indexing Logic

1. Make changes to relevant package (e.g., `shovel/task.go`)
2. Run package-specific tests: `go test -p 1 ./shovel`
3. Run integration tests to verify end-to-end behavior
4. Test with a real config against a live Ethereum node
5. Verify reorg handling works correctly

### Modifying ABI Decoding

1. Update `dig/dig.go` or related ABI parsing code
2. Add test cases in `testdata/` with sample events
3. Run tests: `go test -p 1 ./dig`
4. Verify against real events from mainnet using a test config
5. Check that complex types (tuples, arrays) decode correctly

## Architecture

### Core Components

**Task** (`shovel/task.go`)
The fundamental execution unit connecting an Ethereum source to adatabase table. Each Task represents one integration indexing data
from one source. Tasks implement the convergence algorithm, reorg
detection, state tracking, and batch processing.

**Manager** (`shovel/task.go`)
Orchestrates multiple Tasks based on configuration. Loads source and integration configs, creates Task instances, runs them concurrently, and handles dynamic reconfiguration.

**Source (jrpc2.Client)** (`jrpc2/client.go`)
Handles all Ethereum node communication via JSON-RPC 2.0. Supports
multi-URL load balancing, WebSocket real-time updates, HTTP polling fallback, intelligent caching, and batch requests. Can fetch headers, full blocks, receipts, logs, or traces.

**Destination (dig.Integration)** (`dig/dig.go`)
Implements declarative integration logic to transform blockchain data into database rows. Supports ABI-based event decoding and compiled custom Go code. Handles filtering, data extraction, and bulkinserts via PostgreSQL COPY protocol.

**Configuration System** (`shovel/config/config.go`)
Supports dual configuration sources: JSON files and database storage. Validates configs, generates schema migrations, manages dependencies between integrations, and injects required fields.

### Indexing Data Flow

1. **Manager.Run()** loads config and spawns Task for each source/integration pair
2. **Task.Converge()** runs in a loop:
   - Queries latest indexed block from PostgreSQL
   - Queries latest available block from Ethereum node
   - Calculates delta (what needs indexing)
   - Checks integration dependencies
   - **Task.load()** OR **ConsensusEngine.FetchWithQuorum()** fetches blocks from Ethereum:
     - Single-provider mode: concurrent batch requests, filter application, reorg detection
     - Multi-provider consensus mode: fetches from multiple providers, compares hashes, selects majority response
     - **Provider expansion:** If consensus fails, progressively adds more providers from pool until consensus reached
   - **Task.insert()** transforms and inserts data (ABI decoding,filter evaluation, bulk COPY)
   - Updates task_updates table with progress

### Reorg Handling

**Why reorg handling is critical:** Blockchain reorgs (reorganizations) occur when the canonical chain changes, invalidating previously indexed blocks. Without reorg detection, the database would contain incorrect historical data that never actually occurred on the canonical chain.

Shovel detects reorgs by comparing parent hashes:

1. Load blocks starting from last indexed block + 1
2. Check if first block's parent hash matches the stored hash from previous indexing
3. If mismatch detected: delete rows from current block backwardsand retry indexing
4. Supports up to 1000 reorg iterations per convergence cycle

The parent hash check ensures chain continuity - each block references its parent, creating an immutable chain. A hash mismatch indicates the chain has forked and Shovel must roll back to the fork point.

### Consensus Engine (Phase 1 - SEND-68)

**ConsensusEngine** (`shovel/consensus.go`)
Implements multi-provider consensus to prevent missing logs from faulty RPC providers. When enabled, fetches data from multiple providers in parallel, computes deterministic hashes of results, and requires N-of-M providers to agree before accepting data.

**Key Features:**
- **Provider expansion:** Starts with K providers, progressively expands toward M total providers on consensus failures
- **Quorum voting:** Configurable N-of-M threshold (e.g., 2-of-3, 3-of-5)
- **Deterministic hashing:** keccak256(blockNum || txHash || logIdx || payloadHash) for log comparison
- **Retry with backoff:** Exponential backoff (2s → 30s max) on consensus failures
- **Metrics:** Tracks consensus attempts, failures, expansions, provider errors

**Configuration Example:**
```json
{
  "eth_sources": [{
    "name": "mainnet",
    "chain_id": 1,
    "urls": [
      "https://provider1.example.com",
      "https://provider2.example.com",
      "https://provider3.example.com",
      "https://provider4.example.com",
      "https://provider5.example.com"
    ],
    "consensus": {
      "providers": 3,          // Start with 3 providers (K)
      "threshold": 2,          // Require 2 to agree (N)
      "retry_backoff": "2s",   // Initial retry delay
      "max_backoff": "30s"     // Maximum retry delay
    }
  }]
}
```

**How Provider Expansion Works:**
1. **Attempt 1:** Query K=3 providers (providers 1-3)
2. If no consensus: wait 2s, **expand to K+1=4 providers** (providers 1-4)
3. If still no consensus: wait 4s, **expand to K+2=5 providers** (all providers)
4. Continue with exponential backoff up to 1000 attempts or until consensus reached

**Budget Tuning:**
- **Small deployments:** `providers: 2, threshold: 2` (minimal cost, both must agree)
- **Production:** `providers: 3, threshold: 2` (2-of-3 consensus, tolerates 1 faulty provider)
- **High-assurance:** `providers: 5, threshold: 3` (3-of-5 consensus, tolerates 2 faulty providers)

**Observability:**
- `shovel_consensus_attempts_total` - Number of consensus evaluations
- `shovel_consensus_failures_total` - Failed consensus attempts per retry
- `shovel_consensus_expansions_total` - Times provider pool was expanded
- `shovel_consensus_duration_seconds` - Time to reach consensus
- `shovel_provider_error_total` - RPC errors per provider URL

### Supporting Packages

- **eth/** - Ethereum type system with optimized JSON encoding/decoding
- **wpg/** - PostgreSQL utilities (connection pooling, migrations,
  DDL generation)
- **glf/** - Determines optimal RPC method based on required fields (eth_getLogs, eth_getBlockReceipts, trace_block, or full blocks)
- **bint/** - Efficient big integer encoding/decoding for ABI data
- **wctx/** - Typed context value accessors for request tracking
- **wslog/** - Structured logging with context-aware attributes
- **wos/** - Environment variable support in configuration

## Package Organization

Understanding where functionality lives helps navigate the codebase efficiently:

### cmd/

- **cmd/shovel/** - Main application entry point with CLI flag parsing and initialization
- **cmd/keccak/** - Keccak hashing utility
- **cmd/release/** - Release tooling

### Core Indexing (shovel/)

- **shovel/task.go** - Task and Manager implementation (convergence algorithm, reorg detection, orchestration)
- **shovel/config/** - Configuration loading, validation, schema generation, migrations
- **shovel/glf/** - Get Logs Filter optimization (determines minimal RPC method to use)
- **shovel/web/** - HTTP dashboard and API endpoints
- **shovel/schema.sql** - Internal table definitions for trackingand metadata

### Ethereum Integration (jrpc2/, eth/)

- **jrpc2/client.go** - JSON-RPC 2.0 client with caching, batching, WebSocket support
- **eth/** - Ethereum type definitions (Block, Receipt, Log, Transaction, Trace) with custom JSON marshaling

### Data Processing (dig/)

- **dig/dig.go** - Declarative integration engine (ABI decoding, filtering, row construction)

### Utilities (w\* packages)

Packages prefixed with 'w' are internal utilities (wrapper/helperpackages):

- **wpg/** - PostgreSQL wrappers (connection pool, migration utilities, DDL helpers)
- **wctx/** - Context value management (chain ID, integration name, source tracking)
- **wslog/** - Structured logging with context injection
- **wos/** - OS utilities (environment variable resolution)
- **wstrings/** - String manipulation helpers
- **bint/** - Big integer utilities for ABI encoding/decoding
- **tc/** - Test utilities

### TypeScript (shovel-config-ts/)

- **shovel-config-ts/src/** - TypeScript types and config builderfunctions
- **shovel-config-ts/test/** - TypeScript test suite

## Database Schema

### Internal Tables (shovel schema)

**shovel.task_updates** - Tracks indexing progress per (source, integration) pair. Stores latest block number/hash, performance metrics. Critical for reorg detection (compares stored hash with actual parent hash) and integration dependencies (ensures dependent integrations wait for prerequisites).

**shovel.integrations** - Stores integration configurations as JSON, enabling dynamic updates without file changes or restarts.

**shovel.sources** - Stores Ethereum source configurations (chainID, URLs).

**Views:**

- `shovel.source_updates` - Latest progress aggregated by source
- `shovel.latest` - Conservative latest block across all integrations (useful for knowing safe query bounds)

### User Tables (public schema)

User tables are generated dynamically from integration configs. Each table always includes these system columns:

- `ig_name` - Integration identifier (allows multiple integrations to share one table)
- `src_name` - Source identifier (enables multi-chain data in onetable)
- `block_num` - Block number (required for time-series queries and reorg cleanup)
- `tx_idx` - Transaction index within block (when applicable)
- `log_idx` - Log index within transaction (for event-based integrations)
- `abi_idx` - Index for array elements in ABI data (enables proper ordering of decoded arrays)

Plus user-defined columns from event ABI or block data.

**Automatic unique constraints:** Shovel creates unique indexes on `(ig_name, src_name, block_num, tx_idx, log_idx, abi_idx)` or the appropriate subset. This prevents duplicate data during reorg recovery and ensures idempotent indexing.

**Why this design:** System columns enable multi-chain, multi-integration tables while maintaining data integrity. The unique constraint ensures Shovel can safely retry failed batches without creating duplicates.

## Configuration

Configurations can be in JSON files or stored in PostgreSQL. See `cmd/shovel/demo.json` for a complete example.

Key configuration objects:

- **Source** - Ethereum RPC endpoint (URLs, chain ID, batch size,concurrency)
- **Integration** - Data extraction definition (event ABI, block fields, table schema, filters, dependencies)

Example minimal config:

```json
{
  "pg_url": "postgres:///shovel",
  "eth_sources": [
    {
      "name": "mainnet",
      "chain_id": 1,
      "url": "https://ethereum-rpc.publicnode.com"
    }
  ],
  "integrations": [
    {
      "name": "usdc-transfer",
      "enabled": true,
      "sources": [{ "name": "mainnet", "start": 0 }],
      "table": {
        "name": "usdc",
        "columns": [
          { "name": "log_addr", "type": "bytea" },
          { "name": "from", "type": "bytea" },
          { "name": "to", "type": "bytea" },
          { "name": "value", "type": "numeric" }
        ]
      },
      "block": [
        {
          "name": "log_addr",
          "column": "log_addr",
          "filter_op": "contains",
          "filter_arg": ["a0b86991c6218b36c1d19d4a2e9eb0ce3606eb48"]
        }
      ],
      "event": {
        "name": "Transfer",
        "type": "event",
        "inputs": [
          {
            "indexed": true,
            "name": "from",
            "type": "address",
            "column": "from"
          },
          { "indexed": true, "name": "to", "type": "address", "column": "to" },
          { "name": "value", "type": "uint256", "column": "value" }
        ]
      }
    }
  ]
}
```

## TypeScript Config Package

The `shovel-config-ts/` directory contains a TypeScript package for programmatically building Shovel configuration files. Publishedto npm as `@indexsupply/shovel-config`.

Use this when you need to generate configs dynamically or want type safety when writing configs. See `shovel-config-ts/readme.md` for usage examples.

## Concurrency and Performance

### Concurrency Model

**Task-Level Concurrency:** Each Task (source/integration pair) runs in its own goroutine, enabling parallel indexing of multiple integrations. PostgreSQL advisory locks prevent duplicate tasks from running simultaneously, ensuring data integrity even if multipleShovel instances connect to the same database.

**Request-Level Concurrency:** Within each task, RPC requests aremade concurrently based on configured concurrency limits. The shared block cache uses mutex protection to handle concurrent reads/writes safely.

**Why this architecture:** Task-level parallelism maximizes throughput when indexing multiple integrations, while request-level concurrency reduces RPC latency by fetching multiple block ranges simultaneously. Advisory locks allow horizontal scaling without coordination overhead.

### Performance Optimizations

- **Batch processing** - Configurable batch sizes reduce round trips (default tuned for Ethereum's 12s block time)
- **Concurrent RPC calls** - Multiple requests in flight reduce total indexing time
- **Multi-level caching** - Block and header caching prevents redundant RPC calls when multiple integrations need the same data
- **COPY protocol** - PostgreSQL COPY is orders of magnitude faster than individual INSERTs for bulk data
- **Smart filtering** - `glf` package analyzes configs to determine minimal RPC method (logs-only vs full blocks vs traces)
- **Connection pooling** - Reused HTTP and PostgreSQL connectionseliminate handshake overhead
- **Request coalescing** - Multiple integrations can share fetched block data when indexing the same range

## Testing

**Critical:** Tests must run sequentially using `-p 1` flag:

```bash
go test -p 1 ./...
```

**Why sequential execution is required:** Tests share database state and use PostgreSQL advisory locks. Running tests in parallel causes lock contention and state corruption, leading to flaky test failures.

Integration tests require a running PostgreSQL instance. Configure
the connection via `DATABASE_URL` environment variable.

### Writing Tests

- Follow existing test patterns in `*_test.go` files
- Use `testdata/` directories for test fixtures (JSON configs, sample blocks, receipts)
- Clean up database state after tests to prevent contamination
- Use the `check()` helper for error handling in tests
- Test files are co-located with implementation files (e.g., `task.go` and `task_test.go`)

## Error Handling Patterns

The codebase implements multiple layers of error handling for reliability:

- **Transient Errors:** Automatic retry with exponential backoff for temporary RPC failures
- **Reorgs:** Automatic detection via parent hash mismatch and rollback by deleting affected rows
- **RPC Failures:** URL rotation continues operation even when some endpoints fail
- **Data Validation:** Chain continuity checks via parent hash validation ensure data integrity
- **Timeout Protection:** Statement and transaction timeouts prevent hanging operations
- **Advisory Locks:** Prevent concurrent modification conflicts across multiple Shovel instances

### Error Propagation Convention

Always propagate errors up with context using `fmt.Errorf("context: %w", err)`. This creates a chain of context that makes debugging
easier:

```go
if err != nil {
    return fmt.Errorf("loading blocks %d-%d: %w", start, end, err)
}
```

The `check()` helper in `cmd/shovel/main.go` is used for fatal errors that should terminate the program.

## Web Dashboard

Shovel includes a built-in web dashboard on `localhost:8546` (configurable via `-l` flag):

- Real-time indexing status via WebSocket updates
- Prometheus metrics at `/metrics`
- Task management UI
- Source and integration configuration
- Debug pprof endpoints at `/debug/pprof/`

## Key Design Principles

These principles guide architectural decisions and code contributions:

1. **Declarative over Imperative** - Users describe what data they want, not how to get it. Config-driven design eliminates custom indexing code.

2. **Fail-Safe Operation** - Extensive error handling and automatic recovery. System should self-heal from transient failures.

3. **Data Integrity** - Reorg detection, transaction safety, unique constraints. Correctness is more important than speed.

4. **Performance** - Caching, batching, concurrency, minimal RPC calls. Optimize for throughput without sacrificing correctness.

5. **Observability** - Structured logging with context, metrics, dashboard. Operators should understand system state without reading code.

6. **Flexibility** - Multiple sources, multiple integrations, dynamic configuration. Support diverse use cases without code changes.

7. **Correctness** - Chain validation, data verification, type safety. Wrong data is worse than no data.

## Development Best Practices

### When Making Changes

1. **Run tests before and after changes:** `go test -p 1 ./...`
2. **Keep dependencies tidy:** Run `go mod tidy` and verify no changes with `git diff`
3. **Test against real data:** Use a test config with live Ethereum node to verify behavior
4. **Check for reorg handling:** Ensure changes don't break reorgdetection/recovery
5. **Update documentation:** Keep CLAUDE.md and inline comments current

### Code Style

- Follow idiomatic Go conventions (use `gofmt`)
- Use structured logging with appropriate context values via `wctx`
- Propagate errors with context: `fmt.Errorf("operation failed: %w", err)`
- Prefer explicit error handling over panic (except in tests with`check()`)
- Use PostgreSQL advisory locks for distributed coordination
- Write integration tests for complex flows, unit tests for isolated logic

### Performance Considerations

- Be mindful of RPC call volume (use caching, batching)
- Prefer PostgreSQL COPY over individual INSERTs for bulk data
- Use connection pooling for both HTTP and PostgreSQL
- Profile before optimizing (use `/debug/pprof/` endpoints)
- Consider memory allocation patterns in hot paths (especially ABI
  decoding)

### Database Migration Safety

- Schema changes are automatic but irreversible (adds columns/tables, never drops)
- Test migrations with `-print-schema` before running
- Advisory locks prevent concurrent migrations across instances
- Integration dependencies must form a DAG (no circular dependencies)
