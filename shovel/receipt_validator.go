package shovel

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"

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
	// Use provided filter to ensure consistency with consensus
	blocks, err := rv.client.Get(ctx, url.String(), filter, blockNum, 1)
	if err != nil {
		slog.DebugContext(ctx, "receipt-hash-fetch-failed",
			"block", blockNum,
			"error", err,
		)
		return nil
	}

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
	// Use the same filter as consensus to ensure we fetch identical data
	// The filter determines what fields are populated (logs vs receipts vs blocks)
	slog.DebugContext(ctx, "receipt-validation-start",
		"block", blockNum,
		"provider", url.String(),
	)

	blocks, err := rv.client.Get(ctx, url.String(), filter, blockNum, 1)
	if err != nil {
		return fmt.Errorf("fetching receipts: %w", err)
	}

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
