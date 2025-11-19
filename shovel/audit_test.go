package shovel

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"testing"

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
