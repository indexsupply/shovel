package shovel

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/indexsupply/shovel/eth"
	"github.com/indexsupply/shovel/jrpc2"
	"github.com/indexsupply/shovel/shovel/config"
	"github.com/indexsupply/shovel/shovel/glf"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"
)

type ConsensusEngine struct {
	providers        []*jrpc2.Client // Full provider pool (M providers)
	initialProviders int             // Initial provider count to use (K <= M)
	config           config.Consensus
	metrics          *Metrics
}

func NewConsensusEngine(providers []*jrpc2.Client, conf config.Consensus, metrics *Metrics) (*ConsensusEngine, error) {
	if conf.Threshold > len(providers) {
		return nil, fmt.Errorf("threshold (%d) cannot exceed providers (%d)", conf.Threshold, len(providers))
	}
	if conf.Threshold < 1 {
		return nil, fmt.Errorf("threshold must be >= 1")
	}
	// Validate Byzantine Fault Tolerance requirements:
	// For BFT: N >= 3f + 1, where threshold >= 2f + 1
	// This means: threshold >= 2 * ((len(providers) - 1) / 3) + 1
	// Example: 3 providers requires threshold >= 2, 4 providers requires threshold >= 3
	minThreshold := 2*((len(providers)-1)/3) + 1
	if conf.Threshold < minThreshold {
		return nil, fmt.Errorf("threshold (%d) insufficient for BFT with %d providers (need >= %d)", conf.Threshold, len(providers), minThreshold)
	}
	// Initial provider count: use configured value or all providers if not specified
	initialProviders := conf.Providers
	if initialProviders <= 0 || initialProviders > len(providers) {
		initialProviders = len(providers)
	}
	return &ConsensusEngine{
		providers:        providers,
		initialProviders: initialProviders,
		config:           conf,
		metrics:          metrics,
	}, nil
}

func (ce *ConsensusEngine) FetchWithQuorum(ctx context.Context, filter *glf.Filter, start, limit uint64) ([]eth.Block, []byte, error) {
	ctx, span := Tracer.Start(ctx, "consensus.FetchWithQuorum",
		trace.WithAttributes(
			attribute.Int64("block.start", int64(start)),
			attribute.Int64("block.limit", int64(limit)),
			attribute.Int("providers.total", len(ce.providers)),
			attribute.Int("providers.initial", ce.initialProviders),
			attribute.Int("consensus.threshold", ce.config.Threshold),
		),
	)
	defer span.End()

	var (
		conf = ce.config
		t0   = time.Now()
	)
	// Default to 1000 if not configured
	maxAttempts := conf.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1000
	}

	ce.metrics.Start()
	defer ce.metrics.Stop()

	for attempt := 0; attempt < maxAttempts; attempt++ {
		// Short-circuit if context is already cancelled
		if ctx.Err() != nil {
			recordSpanError(span, ctx.Err(), "context cancelled")
			return nil, nil, ctx.Err()
		}

		// Backoff if this is a retry
		if attempt > 0 {
			delay := conf.RetryBackoff * time.Duration(1<<(attempt-1))
			if delay > conf.MaxBackoff {
				delay = conf.MaxBackoff
			}
			select {
			case <-ctx.Done():
				recordSpanError(span, ctx.Err(), "context cancelled during backoff")
				return nil, nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		// Provider expansion: start with initialProviders (K), add one per retry up to M
		numProviders := ce.initialProviders + attempt
		if numProviders > len(ce.providers) {
			numProviders = len(ce.providers)
		}
		activeProviders := ce.providers[:numProviders]

		if attempt > 0 && numProviders > ce.initialProviders {
			slog.DebugContext(ctx, "consensus-expanding-providers",
				"n", start,
				"attempt", attempt+1,
				"providers", numProviders,
				"initial", ce.initialProviders,
				"max", len(ce.providers),
			)
			ce.metrics.Expansion()
		}

		// Parallel fetch with context cancellation
		var (
			responses = make([][]eth.Block, numProviders)
			errs      = make([]error, numProviders)
		)
		eg, egCtx := errgroup.WithContext(ctx)
		for i, p := range activeProviders {
			i, p := i, p
			eg.Go(func() error {
				// Cache URL at start to avoid rotation drift across multiple calls
				url := p.NextURL()

				// Create child span for this provider fetch
				// Use host-only to avoid exposing API keys in URL path/query
				providerCtx, providerSpan := Tracer.Start(egCtx, "consensus.ProviderFetch",
					trace.WithAttributes(
						attribute.String("provider.host", url.Hostname()),
						attribute.Int("provider.index", i),
					),
				)
				defer providerSpan.End()

				// Use Get method directly, similar to how Task.load works
				// but without the batching loop since we want exact range
				providerStart := time.Now()
				blocks, err := p.Get(providerCtx, url.String(), filter, start, limit)
				ProviderLatency.Record(
					providerCtx,
					time.Since(providerStart).Seconds(),
					metric.WithAttributes(attribute.String("provider", url.Hostname())),
				)
				if err != nil {
					errs[i] = err
					ce.metrics.ProviderError(url.String())
					recordSpanError(providerSpan, err, "provider fetch failed")
					return nil // Don't fail the group, we handle errors individually
				}

				// Record success attributes
				providerSpan.SetAttributes(
					attribute.Int("blocks.count", len(blocks)),
				)
				responses[i] = blocks
				return nil
			})
		}
		if err := eg.Wait(); err != nil {
			recordSpanError(span, err, err.Error())
			return nil, nil, err // Should not happen given we return nil above
		}

		// Compute hashes and count votes
		var (
			counts    = make(map[string]int)
			canonHash string
			canonIdx  int = -1
		)
		for i, blocks := range responses {
			if errs[i] != nil {
				continue
			}
			h := HashBlocksWithRange(blocks, start, limit)
			s := hex.EncodeToString(h)
			counts[s]++
			if counts[s] >= conf.Threshold {
				canonHash = s
				canonIdx = i
				break
			}
		}

		if canonIdx >= 0 {
			elapsed := time.Since(t0)
			span.SetAttributes(
				attribute.Int("consensus.attempts", attempt+1),
				attribute.Int("consensus.active_providers", numProviders),
				attribute.Int("blocks.returned", len(responses[canonIdx])),
				attribute.String("consensus.hash", canonHash[:16]+"..."), // Truncate for readability
				attribute.Int64("duration_ms", elapsed.Milliseconds()),
			)
			slog.DebugContext(ctx, "consensus-reached",
				"n", start,
				"threshold", conf.Threshold,
				"active_providers", numProviders,
				"total_providers", len(ce.providers),
				"attempt", attempt+1,
				"elapsed", elapsed,
			)
			return responses[canonIdx], []byte(canonHash), nil
		}

		// Log failed providers for debugging
		var failedProviders []string
		for i := range responses {
			if errs[i] != nil {
				// Cache URL to get same one that was used in the request
				url := activeProviders[i].NextURL()
				failedProviders = append(failedProviders, url.String())
			}
		}
		if len(failedProviders) > 0 {
			slog.WarnContext(ctx, "providers-failed",
				"failed_count", len(failedProviders),
				"providers", failedProviders,
				"attempt", attempt+1,
			)
		}

		ce.metrics.Failure()
		slog.WarnContext(ctx, "consensus-failed",
			"n", start,
			"threshold", conf.Threshold,
			"votes", fmt.Sprintf("%v", counts),
			"attempt", attempt+1,
		)
	}

	err := fmt.Errorf("consensus not reached after %d attempts", maxAttempts)
	recordSpanError(span, err, err.Error())
	span.SetAttributes(attribute.Int("consensus.attempts", maxAttempts))
	return nil, nil, err
}

// HashBlocksWithRange computes a deterministic hash of the logs in the blocks,
// and for empty ranges includes the requested block range so distinct empty
// ranges do not collide.
func HashBlocksWithRange(blocks []eth.Block, start, limit uint64) []byte {
	if len(blocks) == 0 {
		return eth.Keccak([]byte(fmt.Sprintf("empty-range-%d-%d", start, limit)))
	}
	return HashBlocks(blocks)
}

// HashBlocks computes a deterministic hash of the logs in the blocks.
// It reuses the existing eth.Keccak/Hash logic.
// Logs are sorted by blockNum, txHash, logIdx before hashing to ensure
// determinism regardless of provider return order (though Get usually returns sorted).
//
// Hash computation: For each log, computes payloadHash = keccak256(data || topics), then
// concatenates address || blockNumber || txHash || logIdx || txIdx || payloadHash, and finally
// computes keccak256 of the full buffer.
//
// Full hash formula: keccak256(address || blockNum || txHash || logIdx || txIdx || payloadHash)
// where payloadHash = keccak256(data || topics)
//
// Note: abi_idx is not included as it doesn't exist at the raw log layer (only computed
// during ABI decoding when processing logs into user tables).
func HashBlocks(blocks []eth.Block) []byte {
	if len(blocks) == 0 {
		return eth.Keccak([]byte("empty"))
	}

	// Collect all logs
	var logs []eth.Log
	for _, b := range blocks {
		for _, tx := range b.Txs {
			logs = append(logs, tx.Logs...)
		}
	}

	// Sort logs deterministically
	slices.SortFunc(logs, func(a, b eth.Log) int {
		if uint64(a.BlockNumber) < uint64(b.BlockNumber) {
			return -1
		}
		if uint64(a.BlockNumber) > uint64(b.BlockNumber) {
			return 1
		}
		if n := bytes.Compare(a.TxHash, b.TxHash); n != 0 {
			return n
		}
		return int(a.Idx) - int(b.Idx)
	})

	// Hash the sorted logs
	var buf bytes.Buffer
	for _, l := range logs {
		// Compute payload hash (data + topics)
		var payloadBuf bytes.Buffer
		payloadBuf.Write(l.Data.Bytes())
		for _, t := range l.Topics {
			payloadBuf.Write(t.Bytes())
		}
		payloadHash := eth.Keccak(payloadBuf.Bytes())

		// Concatenate: address || blockNumber || txHash || logIdx || txIdx || payloadHash
		buf.Write(l.Address.Bytes())
		buf.Write([]byte(eth.EncodeUint64(uint64(l.BlockNumber))))
		buf.Write(l.TxHash.Bytes())
		buf.Write([]byte(eth.EncodeUint64(uint64(l.Idx))))
		buf.Write([]byte(eth.EncodeUint64(uint64(l.TxIdx))))
		buf.Write(payloadHash)
	}
	return eth.Keccak(buf.Bytes())
}
