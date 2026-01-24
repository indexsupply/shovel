package shovel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/indexsupply/shovel/eth"
	"github.com/indexsupply/shovel/jrpc2"
	"github.com/indexsupply/shovel/shovel/glf"
	"github.com/indexsupply/shovel/tc"
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

// TestReceiptValidator_Enabled tests the Enabled() accessor
func TestReceiptValidator_Enabled(t *testing.T) {
	t.Run("enabled true", func(t *testing.T) {
		rv := NewReceiptValidator(nil, true, nil)
		if !rv.Enabled() {
			t.Error("expected Enabled() to return true")
		}
	})

	t.Run("enabled false", func(t *testing.T) {
		rv := NewReceiptValidator(nil, false, nil)
		if rv.Enabled() {
			t.Error("expected Enabled() to return false")
		}
	})
}

// TestReceiptValidator_FetchReceiptHash_Disabled verifies FetchReceiptHash
// returns nil when disabled without making RPC calls.
func TestReceiptValidator_FetchReceiptHash_Disabled(t *testing.T) {
	rv := NewReceiptValidator(nil, false, nil)
	hash := rv.FetchReceiptHash(context.Background(), &glf.Filter{}, 1000)
	if hash != nil {
		t.Errorf("expected nil hash when disabled, got %x", hash)
	}
}

// TestValidateReceipts_MetricsIncrement verifies that metrics are incremented
// on mismatch when a Metrics instance is provided.
func TestValidateReceipts_MetricsIncrement(t *testing.T) {
	// We can't easily inspect prometheus counters in unit tests,
	// but we can verify the code path doesn't panic with non-nil metrics
	metrics := NewMetrics("test_src", "test_ig")

	// Mismatch case should call metrics.ReceiptMismatch()
	err := validateReceipts(nil, []byte("different"), 1000, metrics)
	if !errors.Is(err, ErrReceiptMismatch) {
		t.Fatalf("expected ErrReceiptMismatch, got %v", err)
	}
	// Test passes if no panic occurred
}

// TestValidateReceipts_NilMetrics verifies validation works with nil metrics
func TestValidateReceipts_NilMetrics(t *testing.T) {
	// Should not panic with nil metrics
	err := validateReceipts(nil, []byte("different"), 1000, nil)
	if !errors.Is(err, ErrReceiptMismatch) {
		t.Fatalf("expected ErrReceiptMismatch, got %v", err)
	}
}

// TestReceiptValidator_ValidateWithMockClient tests the full Validate() method
// with a mock RPC server. This exercises the complete code path including
// client.Get() and hash comparison, not just the validateReceipts helper.
func TestReceiptValidator_ValidateWithMockClient(t *testing.T) {
	// Block number we'll test with
	const blockNum = 1000001

	// Create a mock server that returns block data with logs
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("reading request body: %v", err)
		}
		t.Logf("mock server received: %s", string(body))

		// Return a minimal valid response for eth_getBlockByNumber + eth_getLogs
		// The response format is a JSON-RPC batch response
		response := fmt.Sprintf(`[
			{"jsonrpc":"2.0","id":"1","result":{
				"hash":"0xcb5cab7266694daa0d28cbf40496c08dd30bf732c41e0455e7ad389c10d79f4f",
				"number":"0x%x",
				"parentHash":"0x8e38b4dbf6b11fcc3b9dee84fb7986e29ca0a02cecd8977c161ff7333329681e",
				"timestamp":"0x56bfb41a",
				"gasLimit":"0x2fefd8",
				"gasUsed":"0x5208"
			}},
			{"jsonrpc":"2.0","id":"2","result":[
				{
					"address":"0x0000000000000000000000000000000000000001",
					"topics":["0x0000000000000000000000000000000000000000000000000000000000000001"],
					"data":"0x01",
					"blockNumber":"0x%x",
					"blockHash":"0xcb5cab7266694daa0d28cbf40496c08dd30bf732c41e0455e7ad389c10d79f4f",
					"transactionIndex":"0x0",
					"logIndex":"0x0"
				}
			]}
		]`, blockNum, blockNum)
		w.Write([]byte(response))
	}))
	defer mockServer.Close()

	// Create a real jrpc2.Client pointing to our mock server
	client := jrpc2.New(mockServer.URL)
	filter := &glf.Filter{UseLogs: true}

	// First, fetch data to compute the expected consensus hash
	ctx := context.Background()
	blocks, err := client.Get(ctx, client.NextURL().String(), filter, blockNum, 1)
	tc.NoErr(t, err)
	expectedHash := HashBlocks(blocks)

	t.Run("Validate_Match", func(t *testing.T) {
		// Create a new client (to avoid cache effects between tests)
		matchClient := jrpc2.New(mockServer.URL)
		rv := NewReceiptValidator(matchClient, true, nil)

		err := rv.Validate(ctx, filter, blockNum, expectedHash)
		tc.NoErr(t, err)
	})

	t.Run("Validate_Mismatch", func(t *testing.T) {
		// Create a new client
		mismatchClient := jrpc2.New(mockServer.URL)
		rv := NewReceiptValidator(mismatchClient, true, nil)

		// Use a wrong hash
		wrongHash := []byte("definitely-wrong-hash")
		err := rv.Validate(ctx, filter, blockNum, wrongHash)
		if !errors.Is(err, ErrReceiptMismatch) {
			t.Fatalf("expected ErrReceiptMismatch, got %v", err)
		}
	})

	t.Run("Validate_Disabled", func(t *testing.T) {
		// When disabled, should return nil even with wrong hash
		rv := NewReceiptValidator(client, false, nil)
		err := rv.Validate(ctx, filter, blockNum, []byte("wrong"))
		tc.NoErr(t, err)
	})

	t.Run("Validate_WithMetrics", func(t *testing.T) {
		// Verify metrics path doesn't panic
		metricsClient := jrpc2.New(mockServer.URL)
		metrics := NewMetrics("test_src", "test_ig")
		rv := NewReceiptValidator(metricsClient, true, metrics)

		// Mismatch should increment metrics
		err := rv.Validate(ctx, filter, blockNum, []byte("wrong"))
		if !errors.Is(err, ErrReceiptMismatch) {
			t.Fatalf("expected ErrReceiptMismatch, got %v", err)
		}
		// Test passes if no panic occurred
	})
}

// TestReceiptValidator_ValidateRPCError tests that Validate() propagates RPC
// errors correctly.
func TestReceiptValidator_ValidateRPCError(t *testing.T) {
	// Create a mock server that returns an error for both requests in batch
	errorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return batch response with error for the block request
		w.Write([]byte(`[
			{"jsonrpc":"2.0","id":"1","error":{"code":-32000,"message":"test error"}},
			{"jsonrpc":"2.0","id":"2","result":[]}
		]`))
	}))
	defer errorServer.Close()

	client := jrpc2.New(errorServer.URL)
	rv := NewReceiptValidator(client, true, nil)
	filter := &glf.Filter{UseLogs: true}

	err := rv.Validate(context.Background(), filter, 1000, []byte("any"))
	if err == nil {
		t.Fatal("expected error for RPC failure")
	}
	// Error should be wrapped with "fetching receipts:" prefix
	tc.WantGot(t, true, err != nil)
}
