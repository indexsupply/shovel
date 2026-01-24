package shovel

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/indexsupply/shovel/eth"
	"github.com/indexsupply/shovel/jrpc2"
	"github.com/indexsupply/shovel/shovel/glf"
)

var ErrReceiptMismatch = errors.New("receipt mismatch")

type ReceiptValidator struct {
	client  *jrpc2.Client
	enabled bool
	metrics *Metrics
}

func NewReceiptValidator(client *jrpc2.Client, enabled bool, metrics *Metrics) *ReceiptValidator {
	return &ReceiptValidator{
		client:  client,
		enabled: enabled,
		metrics: metrics,
	}
}

func (rv *ReceiptValidator) Enabled() bool {
	return rv.enabled
}

// FetchReceiptHash fetches actual receipts for a block and computes the hash.
// This is used to populate the receipt_hash field in block_verification.
// Returns nil hash on error (caller should decide whether to fail or continue).
func (rv *ReceiptValidator) FetchReceiptHash(ctx context.Context, filter *glf.Filter, blockNum uint64) []byte {
	if !rv.enabled {
		return nil
	}

	url := rv.client.NextURL()
	receiptFilter := receiptValidationFilter(filter)
	blocks, err := rv.client.Get(ctx, url.String(), &receiptFilter, blockNum, 1)
	if err != nil {
		slog.DebugContext(ctx, "receipt-hash-fetch-failed",
			"block", blockNum,
			"error", err,
		)
		return nil
	}

	blocks = filterBlocksByLogFilter(blocks, filter)

	// Use same hash computation as consensus for consistency
	if len(blocks) == 0 {
		return HashBlocksWithRange(nil, blockNum, 1)
	}
	return HashBlocks(blocks)
}

// validateReceipts contains the core hash comparison logic for receipt validation.
// It mirrors the helper-style validate() function in jrpc2/client.go, but is
// specialized for comparing a consensus hash against blocks built from receipts.
func validateReceipts(blocks []eth.Block, consensusHash []byte, blockNum uint64, metrics *Metrics) error {
	// No blocks returned: treat as an "empty" range and compare against the
	// canonical empty hash using HashBlocksWithRange to include block range metadata.
	// This matches consensus.go hash computation for empty ranges.
	if len(blocks) == 0 {
		emptyHash := HashBlocksWithRange(nil, blockNum, 1)
		if !bytes.Equal(emptyHash, consensusHash) {
			if metrics != nil {
				metrics.ReceiptMismatch()
			}
			return ErrReceiptMismatch
		}
		return nil
	}

	// Non-empty: hash all logs deterministically and compare.
	hash := HashBlocks(blocks)
	if !bytes.Equal(hash, consensusHash) {
		if metrics != nil {
			metrics.ReceiptMismatch()
		}
		return ErrReceiptMismatch
	}
	return nil
}

func (rv *ReceiptValidator) Validate(ctx context.Context, filter *glf.Filter, blockNum uint64, consensusHash []byte) error {
	if !rv.enabled {
		return nil
	}

	url := rv.client.NextURL()
	receiptFilter := receiptValidationFilter(filter)
	slog.DebugContext(ctx, "receipt-validation-start",
		"block", blockNum,
		"provider", url.String(),
	)

	blocks, err := rv.client.Get(ctx, url.String(), &receiptFilter, blockNum, 1)
	if err != nil {
		return fmt.Errorf("fetching receipts: %w", err)
	}

	blocks = filterBlocksByLogFilter(blocks, filter)

	if err := validateReceipts(blocks, consensusHash, blockNum, rv.metrics); err != nil {
		return err
	}

	empty := len(blocks) == 0
	slog.DebugContext(ctx, "receipt-validation-success",
		"block", blockNum,
		"provider", url.String(),
		"empty", empty,
	)
	return nil
}

func receiptValidationFilter(filter *glf.Filter) glf.Filter {
	receiptFilter := *filter
	receiptFilter.UseReceipts = true
	receiptFilter.UseLogs = false
	receiptFilter.UseBlocks = false
	receiptFilter.UseHeaders = false
	receiptFilter.UseTraces = false
	return receiptFilter
}

func filterBlocksByLogFilter(blocks []eth.Block, filter *glf.Filter) []eth.Block {
	addresses := filter.Addresses()
	topics := filter.Topics()
	if len(addresses) == 0 && len(topics) == 0 {
		return blocks
	}

	addrSet := make(map[string]struct{}, len(addresses))
	for _, addr := range addresses {
		if addr == "" {
			continue
		}
		addrSet[normalizeHex(addr)] = struct{}{}
	}

	topicSets := make([]map[string]struct{}, len(topics))
	for i, entries := range topics {
		if len(entries) == 0 {
			continue
		}
		set := make(map[string]struct{}, len(entries))
		for _, entry := range entries {
			if entry == "" {
				continue
			}
			set[normalizeHex(entry)] = struct{}{}
		}
		topicSets[i] = set
	}

	for bidx := range blocks {
		for tidx := range blocks[bidx].Txs {
			logs := blocks[bidx].Txs[tidx].Logs
			if len(logs) == 0 {
				continue
			}
			filtered := logs[:0]
			for _, l := range logs {
				if logMatchesFilter(l, addrSet, topicSets, topics) {
					filtered = append(filtered, l)
				}
			}
			blocks[bidx].Txs[tidx].Logs = filtered
		}
	}
	return blocks
}

func logMatchesFilter(l eth.Log, addrSet map[string]struct{}, topicSets []map[string]struct{}, topics [][]string) bool {
	if len(addrSet) > 0 {
		addr := eth.EncodeHex(l.Address.Bytes())
		if _, ok := addrSet[normalizeHex(addr)]; !ok {
			return false
		}
	}
	for idx, allowed := range topics {
		if len(allowed) == 0 {
			continue
		}
		if len(l.Topics) <= idx {
			return false
		}
		topic := eth.EncodeHex(l.Topics[idx].Bytes())
		if topicSets[idx] == nil {
			return false
		}
		if _, ok := topicSets[idx][normalizeHex(topic)]; !ok {
			return false
		}
	}
	return true
}

func normalizeHex(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	s = strings.ToLower(s)
	if strings.HasPrefix(s, "0x") {
		return s
	}
	return "0x" + s
}
