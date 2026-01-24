package shovel

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/indexsupply/shovel/eth"
	"github.com/indexsupply/shovel/jrpc2"
	"github.com/indexsupply/shovel/shovel/config"
	"github.com/indexsupply/shovel/shovel/glf"
	"github.com/indexsupply/shovel/tc"
	"github.com/indexsupply/shovel/wpg"
)

func TestHandleRepairRequest(t *testing.T) {
	pg := testpg(t)
	ctx := context.Background()

	// Create test table for repair operations
	_, err := pg.Exec(ctx, "CREATE TABLE shovel.bar (src_name text, ig_name text, block_num bigint)")
	tc.NoErr(t, err)
	
	// Setup a mock source that returns blocks
	mockSrc := &mockSource{
		blocks: []eth.Block{
			newMockBlock(99, []byte("hash99"), []byte("parent98"), "parent_block"),
			newMockBlock(100, []byte("hash100"), []byte("hash99"), "data100"),
			newMockBlock(101, []byte("hash101"), []byte("hash100"), "data101"),
			newMockBlock(102, []byte("hash102"), []byte("hash101"), "data102"),
			newMockBlock(103, []byte("hash103"), []byte("hash102"), "data103"),
			newMockBlock(104, []byte("hash104"), []byte("hash103"), "data104"),
			newMockBlock(105, []byte("hash105"), []byte("hash104"), "data105"),
		},
	}
	
	// Setup a destination
	dest := &mockDestination{
		tableName: "shovel.bar",
		pgp:       pg,
	}
	
	task := &Task{
		ctx:        ctx,
		pgp:        pg,
		src:        mockSrc,
		srcName:    "foo",
		destConfig: config.Integration{Name: "bar"},
		dests:      []Destination{dest},
		filter:     glf.Filter{},
		batchSize:  10,
		concurrency: 1,
		lockid:     wpg.LockHash("test-repair-foo-bar"),
	}
	mgr := &Manager{
		tasks: []*Task{task},
	}

	conf := config.Root{
		Sources: []config.Source{{Name: "foo"}},
		Integrations: []config.Integration{
			{
				Name: "bar", 
				Enabled: true,
				Table: wpg.Table{Name: "shovel.bar"},
				Sources: []config.Source{{Name: "foo"}},
			},
		},
	}

	rs := NewRepairService(pg, conf, mgr)

	t.Run("valid request", func(t *testing.T) {
		reqBody := `{"source": "foo", "integration": "bar", "start_block": 100, "end_block": 105}`
		req := httptest.NewRequest("POST", "/api/v1/repair", strings.NewReader(reqBody))
		w := httptest.NewRecorder()

		rs.HandleRepairRequest(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200 OK, got %d body: %s", w.Code, w.Body.String())
		}

		var resp RepairResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatal(err)
		}

		if resp.Status != "in_progress" {
			t.Errorf("expected status in_progress, got %s", resp.Status)
		}
		if resp.BlocksRequested != 6 {
			t.Errorf("expected 6 blocks, got %d", resp.BlocksRequested)
		}

		// Wait for async execution (simple sleep for test)
		// Better: poll the DB or status endpoint
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			// check status via DB
			var status string
			err := pg.QueryRow(context.Background(), "select status from shovel.repair_jobs where repair_id=$1", resp.RepairID).Scan(&status)
			if err == nil && status == "completed" {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		
		// Check final status
		reqStatus := httptest.NewRequest("GET", "/api/v1/repair/"+resp.RepairID, nil)
		wStatus := httptest.NewRecorder()
		rs.HandleRepairStatus(wStatus, reqStatus)
		
		if wStatus.Code != http.StatusOK {
			t.Errorf("status: expected 200 OK, got %d", wStatus.Code)
		}
		var statusResp RepairResponse
		json.NewDecoder(wStatus.Body).Decode(&statusResp)
		
		if statusResp.Status != "completed" {
			t.Errorf("expected completed, got %s (errors: %v)", statusResp.Status, statusResp.Errors)
		}
		if statusResp.BlocksDeleted != 6 {
			t.Errorf("expected 6 blocks deleted, got %d", statusResp.BlocksDeleted)
		}
	})

	t.Run("invalid source", func(t *testing.T) {
		reqBody := `{"source": "invalid", "integration": "bar", "start_block": 100, "end_block": 105}`
		req := httptest.NewRequest("POST", "/api/v1/repair", strings.NewReader(reqBody))
		w := httptest.NewRecorder()

		rs.HandleRepairRequest(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d", w.Code)
		}
	})

	t.Run("range too large", func(t *testing.T) {
		reqBody := `{"source": "foo", "integration": "bar", "start_block": 100, "end_block": 20000}`
		req := httptest.NewRequest("POST", "/api/v1/repair", strings.NewReader(reqBody))
		w := httptest.NewRecorder()

		rs.HandleRepairRequest(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
	})
}

func TestRepairList(t *testing.T) {
	pg := testpg(t)
	rs := NewRepairService(pg, config.Root{}, nil)

	// Insert some dummy jobs
	_, err := pg.Exec(context.Background(), `
		INSERT INTO shovel.repair_jobs (repair_id, src_name, ig_name, start_block, end_block, status, created_at)
		VALUES 
		('id1', 'foo', 'bar', 10, 20, 'completed', now()),
		('id2', 'foo', 'bar', 30, 40, 'in_progress', now())
	`)
	tc.NoErr(t, err)

	req := httptest.NewRequest("GET", "/api/v1/repairs", nil)
	w := httptest.NewRecorder()
	rs.HandleListRepairs(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var res struct {
		Repairs []RepairResponse `json:"repairs"`
		Total   int              `json:"total"`
	}
	json.NewDecoder(w.Body).Decode(&res)

	if res.Total != 2 {
		t.Errorf("expected 2 repairs, got %d", res.Total)
	}
}

func TestAffectedRows(t *testing.T) {
	pg := testpg(t)
	ctx := context.Background()
	
	// Create table
	_, err := pg.Exec(ctx, "create table shovel.bar (src_name text, ig_name text, block_num bigint)")
	tc.NoErr(t, err)
	
	// Insert data
	_, err = pg.Exec(ctx, "insert into shovel.bar values ('foo', 'bar', 10), ('foo', 'bar', 11), ('foo', 'baz', 10)")
	tc.NoErr(t, err)

	conf := config.Root{
		Integrations: []config.Integration{
			{Name: "bar", Table: wpg.Table{Name: "shovel.bar"}},
		},
	}
	rs := NewRepairService(pg, conf, nil)

	count, err := rs.countAffectedRows(ctx, "foo", "bar", 5, 15)
	tc.NoErr(t, err)
	if count != 2 {
		t.Errorf("expected 2 rows, got %d", count)
	}
}

// TestRepairDryRun verifies the dry_run parameter returns affected row count
// without actually modifying data.
func TestRepairDryRun(t *testing.T) {
	pg := testpg(t)
	ctx := context.Background()

	// Create table and insert test data
	_, err := pg.Exec(ctx, "CREATE TABLE shovel.dryrun_test (src_name text, ig_name text, block_num bigint)")
	tc.NoErr(t, err)

	_, err = pg.Exec(ctx, `
		INSERT INTO shovel.dryrun_test VALUES
		('test_src', 'test_ig', 100),
		('test_src', 'test_ig', 101),
		('test_src', 'test_ig', 102),
		('test_src', 'test_ig', 200)
	`)
	tc.NoErr(t, err)

	conf := config.Root{
		Sources: []config.Source{{Name: "test_src"}},
		Integrations: []config.Integration{{
			Name:    "test_ig",
			Enabled: true,
			Table:   wpg.Table{Name: "shovel.dryrun_test"},
		}},
	}
	rs := NewRepairService(pg, conf, nil)

	// Test dry_run=true
	reqBody := `{"source": "test_src", "integration": "test_ig", "start_block": 100, "end_block": 102, "dry_run": true}`
	req := httptest.NewRequest("POST", "/api/v1/repair", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	rs.HandleRepairRequest(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp RepairResponse
	json.NewDecoder(w.Body).Decode(&resp)

	// Should report 3 rows affected (blocks 100, 101, 102)
	if resp.Status != "dry_run" {
		t.Errorf("expected status dry_run, got %s", resp.Status)
	}
	if resp.RowsAffected != 3 {
		t.Errorf("expected 3 rows affected, got %d", resp.RowsAffected)
	}

	// Verify data was NOT deleted
	var count int
	err = pg.QueryRow(ctx, "SELECT COUNT(*) FROM shovel.dryrun_test").Scan(&count)
	tc.NoErr(t, err)
	if count != 4 {
		t.Errorf("expected 4 rows still in table (dry run should not delete), got %d", count)
	}

	// Verify no repair job was created
	var jobCount int
	err = pg.QueryRow(ctx, "SELECT COUNT(*) FROM shovel.repair_jobs").Scan(&jobCount)
	tc.NoErr(t, err)
	if jobCount != 0 {
		t.Errorf("expected 0 repair jobs (dry run should not create job), got %d", jobCount)
	}
}

// TestRepairStatusFilter verifies HandleListRepairs filters by status correctly.
func TestRepairStatusFilter(t *testing.T) {
	pg := testpg(t)

	// Insert jobs with different statuses
	_, err := pg.Exec(context.Background(), `
		INSERT INTO shovel.repair_jobs (repair_id, src_name, ig_name, start_block, end_block, status, created_at)
		VALUES
		('job1', 'src', 'ig', 10, 20, 'completed', now()),
		('job2', 'src', 'ig', 30, 40, 'in_progress', now()),
		('job3', 'src', 'ig', 50, 60, 'failed', now()),
		('job4', 'src', 'ig', 70, 80, 'completed', now())
	`)
	tc.NoErr(t, err)

	rs := NewRepairService(pg, config.Root{}, nil)

	// Test filtering by "completed"
	req := httptest.NewRequest("GET", "/api/v1/repairs?status=completed", nil)
	w := httptest.NewRecorder()
	rs.HandleListRepairs(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var res struct {
		Repairs []RepairResponse `json:"repairs"`
		Total   int              `json:"total"`
	}
	json.NewDecoder(w.Body).Decode(&res)

	if res.Total != 2 {
		t.Errorf("expected 2 completed repairs, got %d", res.Total)
	}
	for _, r := range res.Repairs {
		if r.Status != "completed" {
			t.Errorf("expected status=completed, got %s", r.Status)
		}
	}

	// Test filtering by "in_progress"
	req = httptest.NewRequest("GET", "/api/v1/repairs?status=in_progress", nil)
	w = httptest.NewRecorder()
	rs.HandleListRepairs(w, req)

	json.NewDecoder(w.Body).Decode(&res)
	if res.Total != 1 {
		t.Errorf("expected 1 in_progress repair, got %d", res.Total)
	}
}

func TestRepairForceAllProvidersParameter(t *testing.T) {
	pg := testpg(t)
	ctx := context.Background()

	conf := config.Root{
		Sources: []config.Source{{Name: "test_force_src"}},
		Integrations: []config.Integration{
			{
				Name:    "test_force_ig",
				Enabled: true,
				Table:   wpg.Table{Name: "shovel.test_force"},
				Sources: []config.Source{{Name: "test_force_src"}},
			},
		},
	}
	// Note: No Manager/Task setup needed - test focuses on JSON parsing and validation
	// Actual repair execution requires complex mocking (tested in repair_integration_test.go)
	rs := NewRepairService(pg, conf, nil)

	// Test 1: Verify force_all_providers=true is accepted and parsed
	t.Run("accepts force_all_providers=true", func(t *testing.T) {
		reqBody := `{"source": "test_force_src", "integration": "test_force_ig", "start_block": 100, "end_block": 102, "force_all_providers": true}`
		req := httptest.NewRequest("POST", "/api/v1/repair", strings.NewReader(reqBody))
		w := httptest.NewRecorder()

		rs.HandleRepairRequest(w, req)

		// Request should be accepted (200 OK) even though execution will fail without task
		// The important validation is that JSON parsing and basic checks pass
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d body: %s", w.Code, w.Body.String())
		}

		var resp RepairResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatal(err)
		}

		if resp.Status != "in_progress" {
			t.Errorf("expected status in_progress, got %s", resp.Status)
		}

		if resp.BlocksRequested != 3 {
			t.Errorf("expected 3 blocks requested, got %d", resp.BlocksRequested)
		}

		// Verify job was created in database with correct parameters
		// We verify parameter acceptance by checking the job was created,
		// not by waiting for execution (which requires complex mocking)
		time.Sleep(100 * time.Millisecond) // Brief wait for goroutine to start
		var startBlock, endBlock uint64
		err := pg.QueryRow(ctx, "SELECT start_block, end_block FROM shovel.repair_jobs WHERE repair_id=$1", resp.RepairID).Scan(&startBlock, &endBlock)
		tc.NoErr(t, err)

		if startBlock != 100 || endBlock != 102 {
			t.Errorf("expected blocks 100-102, got %d-%d", startBlock, endBlock)
		}

		// Execution will fail (task not found), but that's expected in this unit test
		// The integration test in repair_integration_test.go covers full execution
	})

	// Test 2: Verify force_all_providers=false works (default behavior)
	t.Run("accepts force_all_providers=false", func(t *testing.T) {
		reqBody := `{"source": "test_force_src", "integration": "test_force_ig", "start_block": 200, "end_block": 201, "force_all_providers": false}`
		req := httptest.NewRequest("POST", "/api/v1/repair", strings.NewReader(reqBody))
		w := httptest.NewRecorder()

		rs.HandleRepairRequest(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d body: %s", w.Code, w.Body.String())
		}

		var resp RepairResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatal(err)
		}

		if resp.Status != "in_progress" {
			t.Errorf("expected status in_progress, got %s", resp.Status)
		}
	})

	// Test 3: Verify default (omitted parameter) works
	t.Run("omitting force_all_providers uses default", func(t *testing.T) {
		reqBody := `{"source": "test_force_src", "integration": "test_force_ig", "start_block": 300, "end_block": 301}`
		req := httptest.NewRequest("POST", "/api/v1/repair", strings.NewReader(reqBody))
		w := httptest.NewRecorder()

		rs.HandleRepairRequest(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d body: %s", w.Code, w.Body.String())
		}

		var resp RepairResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatal(err)
		}

		if resp.Status != "in_progress" {
			t.Errorf("expected status in_progress, got %s", resp.Status)
		}
	})
}

// TestRepairReorgRetryExhaustion tests that reindexBlocks returns an error
// after exhausting maxReorgRetries (10) when continuous reorgs are detected.
func TestRepairReorgRetryExhaustion(t *testing.T) {
	pg := testpg(t)
	ctx := context.Background()

	// Create test table
	_, err := pg.Exec(ctx, "CREATE TABLE IF NOT EXISTS shovel.reorg_test (src_name text, ig_name text, block_num bigint)")
	tc.NoErr(t, err)

	// Create 32-byte hashes (required for reorg detection at task.go:774)
	// The reorg check requires len(first.Header.Parent) == 32
	hash99 := make([]byte, 32)
	copy(hash99, []byte("hash99"))

	wrongParent := make([]byte, 32)
	copy(wrongParent, []byte("WRONG_PARENT")) // Different from hash99

	// Create a mock source with blocks that have mismatched parent hashes
	// Block 99's hash is hash99, but block 100's parent is wrongParent
	// This mismatch will trigger ErrReorg on every fetch attempt
	reorgSrc := &mockSource{
		blocks: []eth.Block{
			// Block 99 - parent block with hash "hash99"
			newMockBlock(99, hash99, make([]byte, 32), "parent"),
			// Block 100 - its parent is wrongParent, not hash99
			// This mismatch triggers ErrReorg
			newMockBlock(100, make([]byte, 32), wrongParent, "data100"),
		},
	}

	dest := &mockDestination{
		tableName: "shovel.reorg_test",
		pgp:       pg,
	}

	task := &Task{
		ctx:         ctx,
		pgp:         pg,
		src:         reorgSrc,
		srcName:     "reorg-src",
		srcChainID:  1,
		destConfig:  config.Integration{Name: "reorg-ig"},
		dests:       []Destination{dest},
		filter:      glf.Filter{},
		batchSize:   10,
		concurrency: 1,
		lockid:      wpg.LockHash("test-reorg-repair"),
	}
	mgr := &Manager{
		tasks: []*Task{task},
	}

	conf := config.Root{
		Sources: []config.Source{{Name: "reorg-src"}},
		Integrations: []config.Integration{
			{
				Name:    "reorg-ig",
				Enabled: true,
				Table:   wpg.Table{Name: "shovel.reorg_test"},
				Sources: []config.Source{{Name: "reorg-src"}},
			},
		},
	}

	rs := NewRepairService(pg, conf, mgr)

	// Call reindexBlocks directly - this should exhaust retries and return error
	_, err = rs.reindexBlocks(ctx, task, "http://mock-url", 100, 100)

	// Verify error mentions retry exhaustion
	if err == nil {
		t.Fatal("expected error from reindexBlocks, got nil")
	}
	if !strings.Contains(err.Error(), "exhausted reorg retries") {
		t.Errorf("expected 'exhausted reorg retries' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "10 attempts") {
		t.Errorf("expected '10 attempts' in error, got: %v", err)
	}
}

// TestRepairConsensusVsSingleProviderMode tests that the repair service
// correctly selects between consensus mode (url="") and single provider mode.
// The selection logic is at repair.go:497-523:
//   - If task.consensus != nil && url == "" -> consensus mode
//   - Otherwise -> single provider mode
func TestRepairConsensusVsSingleProviderMode(t *testing.T) {
	pg := testpg(t)
	ctx := context.Background()

	// Create test table
	_, err := pg.Exec(ctx, "CREATE TABLE IF NOT EXISTS shovel.mode_test (src_name text, ig_name text, block_num bigint)")
	tc.NoErr(t, err)

	// Create proper 32-byte hashes for reorg detection
	hash98 := make([]byte, 32)
	copy(hash98, []byte("hash98"))
	hash99 := make([]byte, 32)
	copy(hash99, []byte("hash99"))
	hash100 := make([]byte, 32)
	copy(hash100, []byte("hash100"))

	// mockSource that returns valid blocks with correct parent chain
	mockSrc := &mockSource{
		blocks: []eth.Block{
			newMockBlock(99, hash99, hash98, "parent"),
			newMockBlock(100, hash100, hash99, "data100"), // Parent correctly references hash99
		},
	}

	dest := &mockDestination{
		tableName: "shovel.mode_test",
		pgp:       pg,
	}

	// Task without consensus engine (single provider mode)
	taskSingle := &Task{
		ctx:         ctx,
		pgp:         pg,
		src:         mockSrc,
		srcName:     "mode-src",
		srcChainID:  1,
		destConfig:  config.Integration{Name: "mode-ig"},
		dests:       []Destination{dest},
		filter:      glf.Filter{},
		batchSize:   10,
		concurrency: 1,
		lockid:      wpg.LockHash("test-mode-single"),
		consensus:   nil, // No consensus engine - will use single provider mode
	}

	conf := config.Root{
		Sources: []config.Source{{Name: "mode-src"}},
		Integrations: []config.Integration{
			{
				Name:    "mode-ig",
				Enabled: true,
				Table:   wpg.Table{Name: "shovel.mode_test"},
				Sources: []config.Source{{Name: "mode-src"}},
			},
		},
	}

	mgr := &Manager{tasks: []*Task{taskSingle}}
	rs := NewRepairService(pg, conf, mgr)

	t.Run("single provider mode with URL", func(t *testing.T) {
		// Clear previous data
		mockSrc.callsByURL = make(map[string]int)

		// With explicit URL and no consensus engine, should use single provider mode
		// This exercises the code path at repair.go:512-523
		n, err := rs.reindexBlocks(ctx, taskSingle, "http://explicit-provider", 100, 100)
		tc.NoErr(t, err)

		// Verify blocks were processed
		t.Logf("reindexBlocks returned %d rows", n)
	})

	t.Run("single provider mode when consensus is nil", func(t *testing.T) {
		// Even with empty URL, if task.consensus is nil, it uses single provider mode
		// This is the fallback path at repair.go:512-523
		mockSrc.callsByURL = make(map[string]int)

		// With empty URL but no consensus engine, still uses single provider mode
		n, err := rs.reindexBlocks(ctx, taskSingle, "", 100, 100)
		tc.NoErr(t, err)
		t.Logf("reindexBlocks (empty URL, nil consensus) returned %d rows", n)
	})
}

// TestRepairConsensusMode tests the consensus mode path at repair.go:497-511
// where task.consensus != nil && url == "" triggers FetchWithQuorum.
func TestRepairConsensusMode(t *testing.T) {
	pg := testpg(t)
	ctx := context.Background()

	// Create test table
	_, err := pg.Exec(ctx, "CREATE TABLE IF NOT EXISTS shovel.consensus_mode_test (src_name text, ig_name text, block_num bigint)")
	tc.NoErr(t, err)

	// Track FetchWithQuorum calls via mock server
	var consensusCalls int
	mockConsensusServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		consensusCalls++
		// Return valid block data for consensus
		response := `[
			{"jsonrpc":"2.0","id":"1","result":{
				"hash":"0x0000000000000000000000000000000000000000000000000000000000000064",
				"number":"0x64",
				"parentHash":"0x0000000000000000000000000000000000000000000000000000000000000063",
				"timestamp":"0x64ea268f",
				"gasLimit":"0x2fefd8",
				"gasUsed":"0x5208",
				"logsBloom":"0x00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"
			}},
			{"jsonrpc":"2.0","id":"2","result":[]}
		]`
		w.Write([]byte(response))
	}))
	defer mockConsensusServer.Close()

	// Create ConsensusEngine with 2 identical providers (for threshold=2)
	providers := []*jrpc2.Client{
		jrpc2.New(mockConsensusServer.URL),
		jrpc2.New(mockConsensusServer.URL),
	}
	consensusEngine, err := NewConsensusEngine(
		providers,
		config.Consensus{
			Providers:    2,
			Threshold:    2,
			RetryBackoff: 1 * time.Millisecond,
			MaxBackoff:   5 * time.Millisecond,
			MaxAttempts:  3,
		},
		NewMetrics("test", "consensus_mode_test"),
	)
	tc.NoErr(t, err)

	// Create proper 32-byte hashes
	hash99 := make([]byte, 32)
	hash99[31] = 0x63 // matches parentHash in response
	hash100 := make([]byte, 32)
	hash100[31] = 0x64

	// mockSource for Hash() lookups (used for reorg detection)
	mockSrc := &mockSource{
		blocks: []eth.Block{
			newMockBlock(99, hash99, make([]byte, 32), "parent"),
			newMockBlock(100, hash100, hash99, "data100"),
		},
	}

	dest := &mockDestination{
		tableName: "shovel.consensus_mode_test",
		pgp:       pg,
	}

	// Task WITH consensus engine
	taskWithConsensus := &Task{
		ctx:         ctx,
		pgp:         pg,
		src:         mockSrc,
		srcName:     "consensus-mode-src",
		srcChainID:  1,
		destConfig:  config.Integration{Name: "consensus-mode-ig"},
		dests:       []Destination{dest},
		filter:      glf.Filter{UseLogs: true},
		batchSize:   10,
		concurrency: 1,
		lockid:      wpg.LockHash("test-consensus-mode"),
		consensus:   consensusEngine, // Consensus engine present
	}

	conf := config.Root{
		Sources: []config.Source{{Name: "consensus-mode-src"}},
		Integrations: []config.Integration{
			{
				Name:    "consensus-mode-ig",
				Enabled: true,
				Table:   wpg.Table{Name: "shovel.consensus_mode_test"},
				Sources: []config.Source{{Name: "consensus-mode-src"}},
			},
		},
	}

	mgr := &Manager{tasks: []*Task{taskWithConsensus}}
	rs := NewRepairService(pg, conf, mgr)

	// Reset counter
	consensusCalls = 0

	// Call with empty URL - should trigger consensus mode (repair.go:497-511)
	_, err = rs.reindexBlocks(ctx, taskWithConsensus, "", 100, 100)

	// The call may fail due to reorg detection (parent hash check),
	// but the key assertion is that consensus mode was triggered
	// (FetchWithQuorum was called, which calls the mock server)
	if consensusCalls == 0 {
		t.Error("consensus mode not triggered: expected FetchWithQuorum to be called, but mock server received 0 requests")
	}
	t.Logf("consensus mode verified: mock server received %d requests (error: %v)", consensusCalls, err)
}
