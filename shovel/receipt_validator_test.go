package shovel

import (
	"context"
	"errors"
	"testing"

	"github.com/indexsupply/shovel/eth"
	"github.com/indexsupply/shovel/shovel/glf"
)

func TestReceiptValidator(t *testing.T) {
	t.Run("Disabled", func(t *testing.T) {
		rv := NewReceiptValidator(nil, false, nil)
		filter := &glf.Filter{}
		if err := rv.Validate(context.Background(), filter, 1, nil); err != nil {
			t.Errorf("expected nil, got %v", err)
		}
	})
}

// The following tests mirror the helper-style testing pattern used in
// jrpc2/client_test.go for validate(), but exercise validateReceipts and
// HashBlocks in the shovel package.

func TestValidateReceipts_EmptyMatch(t *testing.T) {
	consensus := HashBlocksWithRange(nil, 1000, 1)
	if err := validateReceipts(nil, consensus, 1000, nil); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestValidateReceipts_EmptyMismatch(t *testing.T) {
	if err := validateReceipts(nil, []byte("different"), 1000, nil); !errors.Is(err, ErrReceiptMismatch) {
		t.Fatalf("expected ErrReceiptMismatch, got %v", err)
	}
}

func TestValidateReceipts_Match(t *testing.T) {
	// Pattern taken from consensus_test.go: construct blocks with logs and
	// ensure HashBlocks is deterministic and used consistently.
	l1 := eth.Log{BlockNumber: eth.Uint64(1), Idx: 1, Data: []byte{1}}
	l2 := eth.Log{BlockNumber: eth.Uint64(1), Idx: 2, Data: []byte{2}}
	b := eth.Block{Txs: eth.Txs{{Receipt: eth.Receipt{Logs: eth.Logs{l1, l2}}}}}

	blocks := []eth.Block{b}
	consensus := HashBlocks(blocks)
	if err := validateReceipts(blocks, consensus, 1000, nil); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestValidateReceipts_Mismatch(t *testing.T) {
	// Two blocks with different logs should yield different hashes.
	l1 := eth.Log{BlockNumber: eth.Uint64(1), Idx: 1, Data: []byte{1}}
	l2 := eth.Log{BlockNumber: eth.Uint64(1), Idx: 2, Data: []byte{2}}
	b1 := eth.Block{Txs: eth.Txs{{Receipt: eth.Receipt{Logs: eth.Logs{l1, l2}}}}}

	l3 := eth.Log{BlockNumber: eth.Uint64(1), Idx: 1, Data: []byte{3}}
	b2 := eth.Block{Txs: eth.Txs{{Receipt: eth.Receipt{Logs: eth.Logs{l3}}}}}

	blocks := []eth.Block{b1}
	consensus := HashBlocks([]eth.Block{b2})

	if err := validateReceipts(blocks, consensus, 1000, nil); !errors.Is(err, ErrReceiptMismatch) {
		t.Fatalf("expected ErrReceiptMismatch, got %v", err)
	}
}
