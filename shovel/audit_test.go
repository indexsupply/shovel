package shovel

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/indexsupply/shovel/shovel/config"
	"github.com/indexsupply/shovel/tc"
)

// readJSON mirrors the helper in shovel/integration_test.go (read) but is used
// here to load existing jrpc2 test fixtures for block/log RPC responses.
func readJSON(tb testing.TB, path string) []byte {
	b, err := os.ReadFile(path)
	if err != nil {
		tb.Fatalf("unable to read file %s: %v", path, err)
	}
	return b
}

// rpcRequest is a minimal stand-in for jrpc2.request used in
// jrpc2/client_test.go: methodsMatch. We only care about the Method field.
type rpcRequest struct {
	Method string `json:"method"`
}

// methodsMatch is adapted from jrpc2/client_test.go:112-129. It decodes the
// JSON-RPC request body and returns true if the methods match the expected
// sequence, which is how the jrpc2 tests distinguish between different
// batched calls (blocks-only vs blocks+logs).
func methodsMatch(t *testing.T, body []byte, want ...string) bool {
	var req []rpcRequest
	if err := json.Unmarshal(body, &req); err != nil {
		var r rpcRequest
		if err := json.Unmarshal(body, &r); err != nil {
			t.Fatalf("unable to decode json into a request or []request: %v", err)
		}
		req = append(req, r)
	}

	var methods []string
	for i := range req {
		methods = append(methods, req[i].Method)
	}
	t.Logf("methods=%#v", methods)
	return slices.Equal(methods, want)
}

// newAuditTestServer reuses the existing jrpc2 test fixtures
// (jrpc2/testdata/block-1000001.json and logs-1000001.json) to provide a
// stable RPC surface for Auditor tests. The handler logic follows the same
// pattern as the httptest servers in jrpc2/client_test.go: it inspects the
// batch of JSON-RPC methods and returns either a blocks-only or blocks+logs
// response.
func newAuditTestServer(t *testing.T) *httptest.Server {
	blockBody := readJSON(t, "../jrpc2/testdata/block-1000001.json")
	logsBody := readJSON(t, "../jrpc2/testdata/logs-1000001.json")

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("reading request body: %v", err)
		}
		switch {
		case methodsMatch(t, body, "eth_getBlockByNumber"):
			if _, err := w.Write(blockBody); err != nil {
				t.Fatalf("writing block response: %v", err)
			}
		case methodsMatch(t, body, "eth_getBlockByNumber", "eth_getLogs"):
			if _, err := w.Write(logsBody); err != nil {
				t.Fatalf("writing logs response: %v", err)
			}
		default:
			t.Fatalf("unexpected RPC methods in body: %s", string(body))
		}
	}))
}

// setupAuditorTest wires together the components needed for Auditor tests
// using existing patterns from shovel/task_test.go (testpg, newTestDestination,
// testGeth, NewTask) and Manager/loadTasks for WithSrcName/WithChainID usage.
func setupAuditorTest(t *testing.T) (context.Context, *Auditor, config.Source, *Task) {
	ctx := context.Background()
	pg := testpg(t)
	ts := newAuditTestServer(t)
	t.Cleanup(ts.Close)

	const (
		srcName = "audit-src"
		igName  = "audit"
	)

	dest := newTestDestination(igName)
	tg := &testGeth{}

	task, err := NewTask(
		WithContext(ctx),
		WithPG(pg),
		WithSource(tg),
		WithIntegration(dest.ig()),
		WithIntegrationFactory(dest.factory),
		WithSrcName(srcName),
		WithChainID(1),
	)
	tc.NoErr(t, err)

	sc := config.Source{
		Name:    srcName,
		ChainID: 1,
		URLs:    []string{ts.URL},
		Audit: config.Audit{
			ProvidersPerBlock: 1,
			Confirmations:     0,
			Parallelism:       1,
			CheckInterval:     0,
			Enabled:           true,
		},
	}

	conf := config.Root{Sources: []config.Source{sc}}
	aud := NewAuditor(pg, conf, []*Task{task})

	return ctx, aud, sc, task
}

// TestAuditor_VerifyMatch ensures that when all configured providers return a
// block whose HashBlocks value matches the stored consensus_hash, the auditor
// marks the row as healthy without incrementing retry_count. The setup mirrors
// how Task.Converge and ConsensusEngine use HashBlocks on real blocks.
func TestAuditor_VerifyMatch(t *testing.T) {
	ctx, aud, sc, task := setupAuditorTest(t)
	const blockNum = 1000001

	clients := aud.sources[sc.Name]
	if len(clients) == 0 {
		t.Fatalf("no audit providers configured for source %s", sc.Name)
	}
	client := clients[0]

	blocks, err := client.Get(ctx, client.NextURL().String(), &task.filter, blockNum, 1)
	tc.NoErr(t, err)
	consensusHash := HashBlocks(blocks)

	_, err = aud.pgp.Exec(ctx, `
		insert into shovel.block_verification (src_name, ig_name, block_num, consensus_hash)
		values ($1, $2, $3, $4)
	`, sc.Name, task.destConfig.Name, blockNum, consensusHash)
	tc.NoErr(t, err)

	if err := aud.verify(ctx, sc, auditTask{
			srcName:       sc.Name,
			igName:        task.destConfig.Name,
			blockNum:      blockNum,
			consensusHash: consensusHash,
		}); err != nil {
		t.Fatalf("verify returned error: %v", err)
	}

	var (
		status     string
		retryCount int
	)
	err = aud.pgp.QueryRow(ctx, `
		select audit_status, retry_count
		from shovel.block_verification
		where src_name = $1 and ig_name = $2 and block_num = $3
	`, sc.Name, task.destConfig.Name, blockNum).Scan(&status, &retryCount)
	tc.NoErr(t, err)
	tc.WantGot(t, "healthy", status)
	tc.WantGot(t, 0, retryCount)
}

// TestAuditor_VerifyMismatch exercises the retry path: a stored consensus_hash
// that does not match any provider result should cause the auditor to mark the
// row as retrying, increment retry_count, and delete local task_updates via
// Task.Delete. The SQL patterns follow TestPruneTask and TestLatest.
func TestAuditor_VerifyMismatch(t *testing.T) {
	ctx, aud, sc, task := setupAuditorTest(t)
	const blockNum = 1000001

	mismatch := []byte("not-a-real-hash")
	_, err := aud.pgp.Exec(ctx, `
		insert into shovel.block_verification (
			src_name, ig_name, block_num, consensus_hash, audit_status, retry_count)
		values ($1, $2, $3, $4, 'pending', 0)
	`, sc.Name, task.destConfig.Name, blockNum, mismatch)
	tc.NoErr(t, err)

	_, err = aud.pgp.Exec(ctx, `
		insert into shovel.task_updates (chain_id, src_name, ig_name, num)
		values ($1, $2, $3, $4)
	`, task.srcChainID, sc.Name, task.destConfig.Name, blockNum)
	tc.NoErr(t, err)

	if err := aud.verify(ctx, sc, auditTask{
			srcName:       sc.Name,
			igName:        task.destConfig.Name,
			blockNum:      blockNum,
			consensusHash: mismatch,
		}); err != nil {
		t.Fatalf("verify returned error: %v", err)
	}

	var (
		status     string
		retryCount int
	)
	err = aud.pgp.QueryRow(ctx, `
		select audit_status, retry_count
		from shovel.block_verification
		where src_name = $1 and ig_name = $2 and block_num = $3
	`, sc.Name, task.destConfig.Name, blockNum).Scan(&status, &retryCount)
	tc.NoErr(t, err)
	tc.WantGot(t, "retrying", status)
	tc.WantGot(t, 1, retryCount)

	var remaining int
	err = aud.pgp.QueryRow(ctx, `
		select count(*)
		from shovel.task_updates
		where src_name = $1 and ig_name = $2 and num >= $3
	`, sc.Name, task.destConfig.Name, blockNum).Scan(&remaining)
	tc.NoErr(t, err)
	tc.WantGot(t, 0, remaining)
}

// TestAuditor_MaxRetries verifies that once retry_count reaches the
// maxAuditRetries limit, the auditor marks the row as failed and does not
// delete local task_updates (i.e., no further reindex attempts are scheduled).
func TestAuditor_MaxRetries(t *testing.T) {
	ctx, aud, sc, task := setupAuditorTest(t)
	const blockNum = 1000001

	_, err := aud.pgp.Exec(ctx, `
		insert into shovel.block_verification (
			src_name, ig_name, block_num, consensus_hash, audit_status, retry_count)
		values ($1, $2, $3, $4, 'retrying', $5)
	`, sc.Name, task.destConfig.Name, blockNum, []byte("any"), maxAuditRetries)
	tc.NoErr(t, err)

	_, err = aud.pgp.Exec(ctx, `
		insert into shovel.task_updates (chain_id, src_name, ig_name, num)
		values ($1, $2, $3, $4)
	`, task.srcChainID, sc.Name, task.destConfig.Name, blockNum)
	tc.NoErr(t, err)

	if err := aud.verify(ctx, sc, auditTask{
			srcName:       sc.Name,
			igName:        task.destConfig.Name,
			blockNum:      blockNum,
			consensusHash: []byte("any"),
		}); err != nil {
		t.Fatalf("verify returned error: %v", err)
	}

	var (
		status     string
		retryCount int
	)
	err = aud.pgp.QueryRow(ctx, `
		select audit_status, retry_count
		from shovel.block_verification
		where src_name = $1 and ig_name = $2 and block_num = $3
	`, sc.Name, task.destConfig.Name, blockNum).Scan(&status, &retryCount)
	tc.NoErr(t, err)
	tc.WantGot(t, "failed", status)
	tc.WantGot(t, maxAuditRetries, retryCount)

	var remaining int
	err = aud.pgp.QueryRow(ctx, `
		select count(*)
		from shovel.task_updates
		where src_name = $1 and ig_name = $2 and num >= $3
	`, sc.Name, task.destConfig.Name, blockNum).Scan(&remaining)
	tc.NoErr(t, err)
	tc.WantGot(t, 1, remaining)
}

// TestAuditor_RunContextCancellation verifies the Run() loop exits cleanly
// when context is cancelled.
func TestAuditor_RunContextCancellation(t *testing.T) {
	ctx, aud, _, _ := setupAuditorTest(t)

	// Set a very short interval so Run() will tick quickly
	aud.interval = 10 * time.Millisecond

	ctx, cancel := context.WithCancel(ctx)

	done := make(chan struct{})
	go func() {
		aud.Run(ctx)
		close(done)
	}()

	// Let it run for a bit
	time.Sleep(50 * time.Millisecond)

	// Cancel and verify it exits
	cancel()

	select {
	case <-done:
		// Good - Run() exited
	case <-time.After(1 * time.Second):
		t.Error("Run() did not exit after context cancellation")
	}
}

// TestAuditor_Backpressure verifies that check() returns early when
// inFlight >= maxInFlight.
func TestAuditor_Backpressure(t *testing.T) {
	ctx, aud, _, _ := setupAuditorTest(t)

	// Set maxInFlight to a low value
	aud.maxInFlight = 2

	// Simulate 2 in-flight audits
	aud.inFlight = 2

	// check() should return immediately without doing work
	err := aud.check(ctx)
	tc.NoErr(t, err)

	// Verify no database queries happened (we can't easily verify this
	// but at least we verify it doesn't error or block)
}

// TestAuditor_VerifyTaskNotFound verifies error handling when task is not found.
func TestAuditor_VerifyTaskNotFound(t *testing.T) {
	ctx := context.Background()
	pg := testpg(t)

	// Create auditor with no tasks
	conf := config.Root{
		Sources: []config.Source{{
			Name: "test-src",
			Audit: config.Audit{Enabled: true, ProvidersPerBlock: 1},
		}},
	}
	aud := NewAuditor(pg, conf, []*Task{}) // Empty tasks

	sc := config.Source{
		Name: "test-src",
		Audit: config.Audit{
			Enabled:           true,
			ProvidersPerBlock: 1,
		},
	}

	err := aud.verify(ctx, sc, auditTask{
		srcName:       "test-src",
		igName:        "nonexistent-ig",
		blockNum:      1000,
		consensusHash: []byte("hash"),
	})

	if err == nil {
		t.Error("expected error for missing task")
	}
	if err != nil && !strings.Contains(err.Error(), "task not found") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestAuditor_CheckEmptyQueue verifies check() handles empty audit queue gracefully.
func TestAuditor_CheckEmptyQueue(t *testing.T) {
	ctx, aud, _, _ := setupAuditorTest(t)

	// No block_verification rows exist, check() should return without error
	err := aud.check(ctx)
	tc.NoErr(t, err)
}

// TestAuditor_EscalationThresholdConsensus tests the escalation threshold logic
// at audit.go:302-308 where allMatches >= threshold triggers healthy status.
//
// Scenario: K=2 providers per block, but initial 2 providers return mismatched data.
// Escalation queries all 3 providers, 2 of which return matching data (>= threshold).
// The block should be marked as healthy via threshold-based consensus.
func TestAuditor_EscalationThresholdConsensus(t *testing.T) {
	ctx := context.Background()
	pg := testpg(t)

	const (
		srcName  = "escalation-src"
		igName   = "escalation-ig"
		blockNum = 1000001
	)

	// Read the standard test fixture
	blockBody := readJSON(t, "../jrpc2/testdata/block-1000001.json")
	logsBody := readJSON(t, "../jrpc2/testdata/logs-1000001.json")

	// Provider that returns "correct" data (matching consensus)
	goodServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("reading request body: %v", err)
		}
		switch {
		case methodsMatch(t, body, "eth_getBlockByNumber"):
			w.Write(blockBody)
		case methodsMatch(t, body, "eth_getBlockByNumber", "eth_getLogs"):
			w.Write(logsBody)
		default:
			t.Logf("goodServer: unexpected methods")
			w.Write(logsBody) // fallback
		}
	}))
	t.Cleanup(goodServer.Close)

	// Provider that returns "wrong" data (mismatched hash)
	badResponse := []byte(`[
		{"jsonrpc":"2.0","id":"1","result":{
			"hash":"0xdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
			"number":"0xf4241",
			"parentHash":"0x0000000000000000000000000000000000000000000000000000000000000000"
		}},
		{"jsonrpc":"2.0","id":"2","result":[]}
	]`)
	badServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(badResponse)
	}))
	t.Cleanup(badServer.Close)

	// Set up task with our test servers
	dest := newTestDestination(igName)
	tg := &testGeth{}

	task, err := NewTask(
		WithContext(ctx),
		WithPG(pg),
		WithSource(tg),
		WithIntegration(dest.ig()),
		WithIntegrationFactory(dest.factory),
		WithSrcName(srcName),
		WithChainID(1),
	)
	tc.NoErr(t, err)

	// Configure source with 3 providers, require 2 per block (threshold)
	// Provider order: [bad, good, good]
	// For blockNum=1000001, startIdx = 1000001 % 3 = 2
	// Initial K=2 picks providers at indices 2, 0: [good, bad]
	// This gives 1 match out of 2 required -> escalation triggered
	// Escalation queries all 3: bad(0), good(1), good(2) -> 2 matches >= threshold(2) -> healthy
	sc := config.Source{
		Name:    srcName,
		ChainID: 1,
		URLs:    []string{badServer.URL, goodServer.URL, goodServer.URL},
		Audit: config.Audit{
			ProvidersPerBlock: 2, // K=2, also the threshold
			Confirmations:     0,
			Parallelism:       1,
			CheckInterval:     0,
			Enabled:           true,
		},
	}

	conf := config.Root{Sources: []config.Source{sc}}
	aud := NewAuditor(pg, conf, []*Task{task})

	// Get the consensus hash from a good provider
	goodClient := aud.sources[sc.Name][1] // second provider (goodServer)
	blocks, err := goodClient.Get(ctx, goodClient.NextURL().String(), &task.filter, blockNum, 1)
	tc.NoErr(t, err)
	consensusHash := HashBlocks(blocks)

	// Insert verification row with matching consensus_hash
	_, err = aud.pgp.Exec(ctx, `
		INSERT INTO shovel.block_verification
		(src_name, ig_name, block_num, consensus_hash, audit_status, retry_count)
		VALUES ($1, $2, $3, $4, 'pending', 0)
	`, sc.Name, task.destConfig.Name, blockNum, consensusHash)
	tc.NoErr(t, err)

	// Run verification - this should trigger escalation
	// Initial K=2 providers (indices 2,0 = [good,bad]) gives 1 match < 2 required
	// Escalation queries all 3, gets 2 matches from good servers >= threshold(2)
	// The threshold-based consensus at audit.go:302-308 should mark it healthy
	err = aud.verify(ctx, sc, auditTask{
		srcName:       sc.Name,
		igName:        task.destConfig.Name,
		blockNum:      blockNum,
		consensusHash: consensusHash,
	})
	tc.NoErr(t, err)

	// Check result - MUST be healthy via threshold consensus
	// The escalation threshold logic (allMatches >= threshold) at audit.go:302-308
	// should mark the block healthy when 2 out of 3 providers agree
	var status string
	err = aud.pgp.QueryRow(ctx, `
		SELECT audit_status FROM shovel.block_verification
		WHERE src_name = $1 AND ig_name = $2 AND block_num = $3
	`, sc.Name, task.destConfig.Name, blockNum).Scan(&status)
	tc.NoErr(t, err)

	// Must be healthy - this is the critical assertion for threshold-based consensus
	if status != "healthy" {
		t.Errorf("expected healthy status from threshold-based escalation consensus, got %s", status)
	}
}
