package shovel

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/indexsupply/shovel/eth"
	"github.com/indexsupply/shovel/jrpc2"
	"github.com/indexsupply/shovel/shovel/config"
	"github.com/indexsupply/shovel/shovel/glf"
	"github.com/indexsupply/shovel/tc"
	"github.com/indexsupply/shovel/wpg"
)

// TestBugReproduction_FaultyProviderMissingLogs tests that Shovel correctly indexes
// all logs from block 17943843, which contains 4 ERC-721 Transfer events.
//
// BUG CONTEXT (SEN-68):
// Before multi-provider consensus (Phase 1-3), a single faulty RPC provider could return
// incomplete logs due to:
// - Bloom filter issues (https://github.com/ethereum/go-ethereum/issues/18198)
// - Receipt storage corruption (https://github.com/ethereum/go-ethereum/issues/21770)
// - Rate limiting returning partial results
// - Chain reorgs during query
// - Node sync lag
//
// When a faulty provider returns empty logs [], Shovel would:
// 1. Accept the empty array without validation
// 2. Insert 0 rows into the database
// 3. Mark the block as processed
// 4. Never recover the missing events
//
// TEST BEHAVIOR:
// - With correct implementation (Phase 1-3): Test PASSES (4 events indexed)
// - With buggy implementation (current): Test FAILS (0 events indexed)
//
// This test simulates a faulty provider to verify the bug is fixed.
func TestBugReproduction_FaultyProviderMissingLogs(t *testing.T) {
	var (
		ctx  = context.Background()
		pg   = wpg.TestPG(t, Schema)
		conf = config.Root{}
	)

	// Use erc721.json config - block 17943843 has 4 Transfer events
	decode(t, read(t, "erc721.json"), &conf.Integrations)
	tc.NoErr(t, config.ValidateFix(&conf))
	tc.NoErr(t, config.Migrate(ctx, pg, conf))

	const (
		targetBlock     = 17943843
		parentBlockHash = "0x1e2daaa5c2e7fa9632fe767eb2f1fe0b42d8cbe1e8d18a2a275bc2060e86a1be"
		blockHash       = "0x1f2daaa5c2e7fa9632fe767eb2f1fe0b42d8cbe1e8d18a2a275bc2060e86a1bf"
		srcName         = "faulty-provider"
		chainID         = 1
	)

	// Create a faulty provider that returns correct block structure but EMPTY logs
	// This simulates real-world eth_getLogs failures documented in:
	// - https://github.com/ethereum/go-ethereum/issues/18198
	// - https://github.com/ethereum/go-ethereum/issues/21770
	// - https://github.com/ethereum/go-ethereum/issues/15936
	var callCount int
	faultyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		body, _ := io.ReadAll(r.Body)
		
		var reqs []map[string]any
		if err := json.Unmarshal(body, &reqs); err != nil {
			// Single request
			var req map[string]any
			json.Unmarshal(body, &req)
			reqs = []map[string]any{req}
		}

		var responses []map[string]any
		for _, req := range reqs {
			method := req["method"].(string)
			id := req["id"]
			
		switch method {
		case "eth_getBlockByNumber":
			// Return valid block with transactions
			// This block SHOULD have 4 ERC-721 Transfer event logs
			responses = append(responses, map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]any{
					"number":     fmt.Sprintf("0x%x", targetBlock),
					"hash":       blockHash,
					"parentHash": parentBlockHash,
					"timestamp":  "0x64e43a9f",
					"gasLimit":   "0x1c9c380",
					"gasUsed":    "0x1234567",
					// Block has transactions - this is key!
					// Real block 17943843 has tx 0x713df81a... with 4 Transfer events
					"transactions": []any{
						map[string]any{
							"blockHash":        blockHash,
							"blockNumber":      fmt.Sprintf("0x%x", targetBlock),
							"from":             "0xce020e4bca3a181cacd771a4750e03f384779313",
							"gas":              "0x5208",
							"gasPrice":         "0x12a05f200",
							"hash":             "0x713df81a2ab53db1d01531106fc5de43012a401ddc3e0586d522e5c55a162d42",
							"input":            "0x",
							"nonce":            "0x1",
							"to":               "0x57f1887a8bf19b14fc0df6fd9b2acc9af147ea85",
							"transactionIndex": "0x0",
							"value":            "0x0",
							"type":             "0x2",
							"v":                "0x1",
							"r":                "0x1234567890abcdef",
							"s":                "0xfedcba0987654321",
						},
					},
				},
			})
		case "eth_getLogs":
			// Return EMPTY logs array - THIS IS THE BUG
			// Block has transaction 0x713df81a... which emitted 4 ERC-721 Transfer events
			// But faulty eth_getLogs returns [] (the known bug from GitHub issues)
			// This is the exact scenario from:
			// - https://github.com/ethereum/go-ethereum/issues/18198 (logs missing after restart)
			// - https://github.com/ethereum/go-ethereum/issues/21770 (wrong number of logs)
			responses = append(responses, map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result":  []any{}, // ← THE BUG: block has txs but eth_getLogs returns empty
			})
			case "eth_getBlockByHash":
				// For parent hash lookup
				responses = append(responses, map[string]any{
					"jsonrpc": "2.0",
					"id":      id,
					"result": map[string]any{
						"number":     fmt.Sprintf("0x%x", targetBlock-1),
						"hash":       parentBlockHash,
						"parentHash": "0x0000000000000000000000000000000000000000000000000000000000000000",
						"timestamp":  "0x64e43a9e",
					},
				})
			default:
				responses = append(responses, map[string]any{
					"jsonrpc": "2.0",
					"id":      id,
					"error": map[string]any{
						"code":    -32601,
						"message": "Method not found",
					},
				})
			}
		}

		w.Header().Set("Content-Type", "application/json")
		if len(responses) == 1 {
			json.NewEncoder(w).Encode(responses[0])
		} else {
			json.NewEncoder(w).Encode(responses)
		}
	}))
	defer faultyServer.Close()

	// Create task with the faulty provider
	// This is the current main branch behavior: single provider, no validation
	faultyProvider := jrpc2.New(faultyServer.URL)
	
	ig := conf.Integrations[0]
	task, err := NewTask(
		WithContext(ctx),
		WithPG(pg),
		WithSource(faultyProvider), // Single faulty provider - no consensus
		WithIntegration(ig),
		WithRange(targetBlock, targetBlock+1),
		WithSrcName(srcName),
		WithChainID(chainID),
	)
	tc.NoErr(t, err)

	// Execute the convergence
	err = task.Converge()
	tc.NoErr(t, err)

	// ASSERT: We should have indexed exactly 4 Transfer events
	// This is the expected correct behavior based on integration_test.go lines 57-72
	const expectedCount = 4
	var actualCount int
	query := fmt.Sprintf(`
		select count(*) from erc721_test
		where block_num = %d
		and tx_hash = '\x713df81a2ab53db1d01531106fc5de43012a401ddc3e0586d522e5c55a162d42'
		and contract = '\x57f1887a8bf19b14fc0df6fd9b2acc9af147ea85'
	`, targetBlock)
	
	tc.NoErr(t, pg.QueryRow(ctx, query).Scan(&actualCount))
	
	// Test PASSES when consensus/validation works (actualCount == 4)
	// Test FAILS when bug exists (actualCount == 0)
	if actualCount != expectedCount {
		t.Errorf("\n========== BUG DETECTED ==========\n")
		t.Errorf("Expected %d Transfer events, but got %d", expectedCount, actualCount)
		t.Errorf("")
		t.Errorf("FAILURE DETAILS:")
		t.Errorf("  Block: %d", targetBlock)
		t.Errorf("  Transaction: 0x713df81a2ab53db1d01531106fc5de43012a401ddc3e0586d522e5c55a162d42")
		t.Errorf("  Expected: %d ERC-721 Transfer events", expectedCount)
		t.Errorf("  Actual: %d events indexed", actualCount)
		t.Errorf("")
		t.Errorf("ROOT CAUSE:")
		t.Errorf("  - Faulty provider returned eth_getLogs = []")
		t.Errorf("  - Single provider (no consensus validation)")
		t.Errorf("  - No receipt-level cross-validation")
		t.Errorf("  - No post-confirmation audit")
		t.Errorf("")
		t.Errorf("IMPACT:")
		t.Errorf("  - %d critical events silently missing from database", expectedCount-actualCount)
		t.Errorf("  - Block marked complete, will never auto-recover")
		t.Errorf("  - User balances incorrect, compliance gaps")
		t.Errorf("")
		t.Errorf("SOLUTION: Implement SEN-68 Phases 1-3")
		t.Errorf("  Phase 1: Multi-provider consensus (2-of-3 agreement)")
		t.Errorf("  Phase 2: Receipt validation (eth_getBlockReceipts)")
		t.Errorf("  Phase 3: Confirmation audit loop")
		t.Errorf("")
		t.Errorf("See: https://github.com/ethereum/go-ethereum/issues/18198")
		t.Errorf("     https://github.com/ethereum/go-ethereum/issues/21770")
		t.Errorf("==================================\n")
	} else {
		t.Logf("✓ SUCCESS: Indexed %d/%d events correctly", actualCount, expectedCount)
		t.Logf("  Multi-provider consensus prevented data loss")
	}
}

// TestBugConditions_PartialLogs tests that Shovel detects and handles the case
// where a provider returns SOME logs but not ALL logs for a block.
//
// This is an even more subtle variant of the SEN-68 bug where incomplete data
// is returned (2 of 4 events). This is harder to detect without consensus validation.
//
// TEST BEHAVIOR:
// - With correct implementation: Test PASSES (detects partial data, requires consensus)
// - With buggy implementation: Test FAILS (accepts 2 events, missing 2)
func TestBugConditions_PartialLogs(t *testing.T) {
	var (
		ctx  = context.Background()
		pg   = wpg.TestPG(t, Schema)
		conf = config.Root{}
	)

	decode(t, read(t, "erc721.json"), &conf.Integrations)
	tc.NoErr(t, config.ValidateFix(&conf))
	tc.NoErr(t, config.Migrate(ctx, pg, conf))

	const (
		targetBlock     = 17943843
		parentBlockHash = "0x1e2daaa5c2e7fa9632fe767eb2f1fe0b42d8cbe1e8d18a2a275bc2060e86a1be"
		blockHash       = "0x1f2daaa5c2e7fa9632fe767eb2f1fe0b42d8cbe1e8d18a2a275bc2060e86a1bf"
		srcName         = "partial-provider"
		chainID         = 1
	)

	// Provider returns only 2 of 4 logs - even more subtle bug
	partialServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		
		var reqs []map[string]any
		if err := json.Unmarshal(body, &reqs); err != nil {
			var req map[string]any
			json.Unmarshal(body, &req)
			reqs = []map[string]any{req}
		}

		var responses []map[string]any
		for _, req := range reqs {
			method := req["method"].(string)
			id := req["id"]
			
			switch method {
			case "eth_getBlockByNumber":
				responses = append(responses, map[string]any{
					"jsonrpc": "2.0",
					"id":      id,
					"result": map[string]any{
						"number":       fmt.Sprintf("0x%x", targetBlock),
						"hash":         blockHash,
						"parentHash":   parentBlockHash,
						"timestamp":    "0x64e43a9f",
						"transactions": []any{},
					},
				})
			case "eth_getLogs":
				// Return only 2 of 4 logs - more subtle data corruption
				responses = append(responses, map[string]any{
					"jsonrpc": "2.0",
					"id":      id,
					"result": []any{
						// Minimal valid log structure - just 2 events instead of 4
						map[string]any{
							"address":     "0x57f1887a8bf19b14fc0df6fd9b2acc9af147ea85",
							"topics":      []string{"0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"},
							"data":        "0x",
							"blockNumber": fmt.Sprintf("0x%x", targetBlock),
							"logIndex":    "0x100",
						},
						map[string]any{
							"address":     "0x57f1887a8bf19b14fc0df6fd9b2acc9af147ea85",
							"topics":      []string{"0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"},
							"data":        "0x",
							"blockNumber": fmt.Sprintf("0x%x", targetBlock),
							"logIndex":    "0x101",
						},
						// Missing 2 more logs!
					},
				})
			case "eth_getBlockByHash":
				responses = append(responses, map[string]any{
					"jsonrpc": "2.0",
					"id":      id,
					"result": map[string]any{
						"number":     fmt.Sprintf("0x%x", targetBlock-1),
						"hash":       parentBlockHash,
						"parentHash": "0x0000000000000000000000000000000000000000000000000000000000000000",
					},
				})
			}
		}

		w.Header().Set("Content-Type", "application/json")
		if len(responses) == 1 {
			json.NewEncoder(w).Encode(responses[0])
		} else {
			json.NewEncoder(w).Encode(responses)
		}
	}))
	defer partialServer.Close()

	partialProvider := jrpc2.New(partialServer.URL)
	
	ig := conf.Integrations[0]
	task, err := NewTask(
		WithContext(ctx),
		WithPG(pg),
		WithSource(partialProvider),
		WithIntegration(ig),
		WithRange(targetBlock, targetBlock+1),
		WithSrcName(srcName),
		WithChainID(chainID),
	)
	tc.NoErr(t, err)

	err = task.Converge()
	tc.NoErr(t, err)

	// ASSERT: Should have indexed all 4 events, not just the 2 returned by faulty provider
	const expectedCount = 4
	var actualCount int
	query := fmt.Sprintf(`select count(*) from erc721_test where block_num = %d`, targetBlock)
	tc.NoErr(t, pg.QueryRow(ctx, query).Scan(&actualCount))
	
	if actualCount != expectedCount {
		t.Errorf("\n========== PARTIAL DATA BUG DETECTED ==========\n")
		t.Errorf("Expected %d events, got %d (missing %d)", expectedCount, actualCount, expectedCount-actualCount)
		t.Errorf("")
		t.Errorf("This is the SUBTLE variant of SEN-68:")
		t.Errorf("  - Provider returned SOME logs but not ALL")
		t.Errorf("  - Harder to detect than complete empty response")
		t.Errorf("  - Silent data corruption in production")
		t.Errorf("")
		t.Errorf("SOLUTION: Multi-provider consensus would detect:")
		t.Errorf("  - Provider A: 2 events")
		t.Errorf("  - Provider B: 4 events")
		t.Errorf("  - Provider C: 4 events")
		t.Errorf("  - Consensus: Reject Provider A, accept B/C (4 events)")
		t.Errorf("==============================================\n")
	} else {
		t.Logf("✓ SUCCESS: Detected partial data and indexed all %d events", expectedCount)
	}
}

// mockFaultySource implements Source interface for unit testing the bug
type mockFaultySource struct {
	returnEmptyLogs bool
}

func (m *mockFaultySource) Get(ctx context.Context, url string, filter *glf.Filter, start, limit uint64) ([]eth.Block, error) {
	if m.returnEmptyLogs {
		// Return valid block but with no transactions/logs
		block := eth.Block{
			Txs: eth.Txs{}, // Empty - the bug!
		}
		block.Header.Number = eth.Uint64(start)
		block.Header.Hash = mustHex("0x1f2daaa5c2e7fa9632fe767eb2f1fe0b42d8cbe1e8d18a2a275bc2060e86a1bf")
		block.Header.Parent = mustHex("0x1e2daaa5c2e7fa9632fe767eb2f1fe0b42d8cbe1e8d18a2a275bc2060e86a1be")
		return []eth.Block{block}, nil
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *mockFaultySource) Latest(ctx context.Context, url string, num uint64) (uint64, []byte, error) {
	return 17943844, mustHex("0x2f2daaa5c2e7fa9632fe767eb2f1fe0b42d8cbe1e8d18a2a275bc2060e86a1bf"), nil
}

func (m *mockFaultySource) Hash(ctx context.Context, url string, num uint64) ([]byte, error) {
	return mustHex("0x1e2daaa5c2e7fa9632fe767eb2f1fe0b42d8cbe1e8d18a2a275bc2060e86a1be"), nil
}

func (m *mockFaultySource) NextURL() *jrpc2.URL {
	u, _ := jrpc2.New("http://faulty-provider:8545").NextURL(), (*jrpc2.URL)(nil)
	return u
}

func mustHex(s string) []byte {
	if len(s) > 2 && s[0:2] == "0x" {
		s = s[2:]
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}
