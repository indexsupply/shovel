package shovel

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/indexsupply/shovel/jrpc2"
	"github.com/indexsupply/shovel/shovel/config"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Auditor struct {
	pgp      *pgxpool.Pool
	conf     config.Root
	sources  map[string][]*jrpc2.Client
	tasks    []*Task
	interval time.Duration

	// inFlight tracks the number of active audit verifications. It uses the
	// same atomic increment/decrement pattern as Task.insert's nrows counter
	// (see task.go:667-686) to avoid races without additional locks.
	inFlight   int64
	maxInFlight int64
}

func NewAuditor(pgp *pgxpool.Pool, conf config.Root, tasks []*Task) *Auditor {
	a := &Auditor{
		pgp:     pgp,
		conf:    conf,
		tasks:   tasks,
		sources: make(map[string][]*jrpc2.Client),
	}
	for _, sc := range conf.Sources {
		var clients []*jrpc2.Client
		for _, u := range sc.URLs {
			clients = append(clients, jrpc2.New(u))
		}
		a.sources[sc.Name] = clients
		if sc.Audit.Enabled {
			if a.interval == 0 || sc.Audit.CheckInterval < a.interval {
				a.interval = sc.Audit.CheckInterval
			}
			// Derive a simple global backpressure limit from per-source
			// parallelism. This mirrors how Task.insert uses batchSize and
			// concurrency to bound work in flight.
			perSource := sc.Audit.Parallelism
			if perSource <= 0 {
				perSource = 4
			}
			a.maxInFlight += int64(perSource)
		}
	}
	if a.interval == 0 {
		a.interval = 5 * time.Second
	}
	if a.maxInFlight == 0 {
		// Fallback bound if all sources disabled; small, similar to a single
		// source with Parallelism=4.
		a.maxInFlight = 4
	}
	return a
}

func (a *Auditor) Run(ctx context.Context) {
	ticker := time.NewTicker(a.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := a.check(ctx); err != nil {
				slog.ErrorContext(ctx, "audit check", "error", err)
			}
		}
	}
}

type auditTask struct {
	srcName       string
	igName        string
	blockNum      uint64
	consensusHash []byte
}

const maxAuditRetries = 10

func (a *Auditor) check(ctx context.Context) error {
	// Global backpressure: if too many audits are in flight, skip this tick.
	if atomic.LoadInt64(&a.inFlight) >= a.maxInFlight {
		return nil
	}
	for _, sc := range a.conf.Sources {
		if !sc.Audit.Enabled {
			continue
		}
		// Query total pending count for accurate queue metric
		const countQ = `
			SELECT COUNT(*)
			FROM shovel.block_verification bv
			WHERE bv.src_name = $1
			  AND bv.audit_status IN ('pending', 'retrying')
			  AND bv.block_num <= (
			      SELECT MAX(tu.num) - $2
			      FROM shovel.task_updates tu
			      WHERE tu.src_name = bv.src_name AND tu.ig_name = bv.ig_name
			  )
		`
		var totalPending int
		if err := a.pgp.QueryRow(ctx, countQ, sc.Name, sc.Audit.Confirmations).Scan(&totalPending); err != nil {
			return fmt.Errorf("counting pending audits: %w", err)
		}

		// Track total queue length for this source
		AuditQueueLength.WithLabelValues(sc.Name).Set(float64(totalPending))

		if totalPending == 0 {
			continue
		}

		// Query blocks pending audit for this source. Instead of joining
		// directly against shovel.task_updates on every row, use a correlated
		// subquery to compare against the latest task update per (src, ig)
		// pair. This mirrors the "MAX(num)" pattern used elsewhere in the
		// schema migrations and keeps the planner focused on the index on
		// (ig_name, src_name, num DESC).
		const q = `
			SELECT bv.src_name, bv.ig_name, bv.block_num, bv.consensus_hash
			FROM shovel.block_verification bv
			WHERE bv.src_name = $1
			  AND bv.audit_status IN ('pending', 'retrying')
			  AND bv.block_num <= (
			      SELECT MAX(tu.num) - $2
			      FROM shovel.task_updates tu
			      WHERE tu.src_name = bv.src_name AND tu.ig_name = bv.ig_name
			  )
			ORDER BY bv.block_num ASC
			LIMIT 100
		`
		rows, err := a.pgp.Query(ctx, q, sc.Name, sc.Audit.Confirmations)
		if err != nil {
			return fmt.Errorf("querying pending audits: %w", err)
		}
		var batch []auditTask
		for rows.Next() {
			var t auditTask
			if err := rows.Scan(&t.srcName, &t.igName, &t.blockNum, &t.consensusHash); err != nil {
				rows.Close()
				return fmt.Errorf("scanning audit task: %w", err)
			}
			batch = append(batch, t)
		}
		rows.Close()

		// Process batch
		// Limit parallelism per source, while also tracking global in-flight
		// audits for backpressure.
		sem := make(chan struct{}, sc.Audit.Parallelism)
		var wg sync.WaitGroup
		for _, t := range batch {
			wg.Add(1)
			sem <- struct{}{}
			go func(t auditTask) {
				defer wg.Done()
				defer func() { <-sem }()
				atomic.AddInt64(&a.inFlight, 1)
				defer atomic.AddInt64(&a.inFlight, -1)
				if err := a.verify(ctx, sc, t); err != nil {
					slog.ErrorContext(ctx, "verifying block",
						"src", t.srcName,
						"ig", t.igName,
						"block", t.blockNum,
						"error", err,
					)
				}
			}(t)
		}
		// Best-effort shutdown: if the context is already cancelled, do not
		// block indefinitely on wg.Wait(). This mirrors the ctx.Err() short
		// circuit used in ConsensusEngine.FetchWithQuorum.
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			wg.Wait()
		}
	}
	return nil
}

func (a *Auditor) verify(ctx context.Context, sc config.Source, t auditTask) error {
	// Mark as verifying at start of audit attempt
	const startVerify = `
		UPDATE shovel.block_verification
		SET audit_status = 'verifying', last_verified_at = now()
		WHERE src_name = $1 AND ig_name = $2 AND block_num = $3
		  AND audit_status IN ('pending', 'retrying')
	`
	if _, err := a.pgp.Exec(ctx, startVerify, t.srcName, t.igName, t.blockNum); err != nil {
		return fmt.Errorf("marking audit as verifying: %w", err)
	}

	// Find the task to get filter
	var task *Task
	for _, tk := range a.tasks {
		if tk.srcName == t.srcName && tk.destConfig.Name == t.igName {
			task = tk
			break
		}
	}
	if task == nil {
		return fmt.Errorf("task not found for %s/%s", t.srcName, t.igName)
	}

	// Pick providers (round-robin based on block num)
	providers := a.sources[sc.Name]
	if len(providers) == 0 {
		return fmt.Errorf("no providers for source %s", sc.Name)
	}

	// Fetch from K providers
	k := sc.Audit.ProvidersPerBlock
	if k <= 0 {
		k = 1
	}
	if k > len(providers) {
		k = len(providers)
	}

	// Simple rotation: start at blockNum % len
	startIdx := int(t.blockNum) % len(providers)

	var (
		matches = 0
		mu      sync.Mutex
		wg      sync.WaitGroup
	)

	for i := 0; i < k; i++ {
		idx := (startIdx + i) % len(providers)
		p := providers[idx]
		wg.Add(1)
		go func(p *jrpc2.Client) {
			defer wg.Done()
			blocks, err := p.Get(ctx, p.NextURL().String(), &task.filter, t.blockNum, 1)
			if err != nil {
				slog.WarnContext(ctx, "audit fetch failed", "provider", p.NextURL().Hostname(), "error", err)
				return
			}
			h := HashBlocks(blocks)
			if bytes.Equal(h, t.consensusHash) {
				mu.Lock()
				matches++
				mu.Unlock()
			}
		}(p)
	}
	wg.Wait()

	// After normal K providers fail, try all providers before giving up.
	if matches < k {
		var (
			allMatches int
			muAll      sync.Mutex
			wgAll      sync.WaitGroup
		)
		for _, p := range providers {
			// Check for context cancellation before spawning more goroutines
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			wgAll.Add(1)
			p := p
			go func(p *jrpc2.Client) {
				defer wgAll.Done()
				blocks, err := p.Get(ctx, p.NextURL().String(), &task.filter, t.blockNum, 1)
				if err != nil {
					slog.WarnContext(ctx, "audit fetch failed (escalated)", "provider", p.NextURL().Hostname(), "error", err)
					return
				}
				h := HashBlocks(blocks)
				if bytes.Equal(h, t.consensusHash) {
					muAll.Lock()
					allMatches++
					muAll.Unlock()
				}
			}(p)
		}
		wgAll.Wait()
		// Use threshold-based consensus instead of unanimity
		// If we have k-of-n agreement (where k is the consensus threshold),
		// accept the result. This matches the consensus engine's logic.
		threshold := sc.Audit.ProvidersPerBlock
		if threshold <= 0 {
			threshold = 1
		}
		if allMatches >= threshold {
			matches = k
		}
	}

	// Record verification attempt (all audits, success or failure)
	AuditVerifications.WithLabelValues(t.srcName, t.igName).Inc()

	// If all K providers match, mark healthy
	if matches == k {
		const u = `
			UPDATE shovel.block_verification
			SET audit_status = 'healthy', last_verified_at = now()
			WHERE src_name = $1 AND ig_name = $2 AND block_num = $3
		`
		_, err := a.pgp.Exec(ctx, u, t.srcName, t.igName, t.blockNum)
		return err
	}

	// Mismatch or failure -> Trigger Reindex
	AuditFailures.WithLabelValues(t.srcName, t.igName).Inc()
	slog.WarnContext(ctx, "audit mismatch",
		"src", t.srcName,
		"ig", t.igName,
		"block", t.blockNum,
		"matches", matches,
		"required", k,
	)

	// Begin transaction and acquire lock for atomic retry check and delete.
	// This ensures that the retry_count check and subsequent operations happen
	// atomically, preventing concurrent audits from bypassing maxAuditRetries.
	tx, err := a.pgp.Begin(ctx)
	if err != nil {
		return fmt.Errorf("starting verify tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, "select pg_advisory_xact_lock($1)", task.lockid); err != nil {
		return fmt.Errorf("acquiring task lock for verify: %w", err)
	}

	// Enforce a max retry limit before scheduling another delete/reindex.
	const checkRetryQ = `
		SELECT retry_count FROM shovel.block_verification
		WHERE src_name = $1 AND ig_name = $2 AND block_num = $3
	`
	var retryCount int
	if err := tx.QueryRow(ctx, checkRetryQ, t.srcName, t.igName, t.blockNum).Scan(&retryCount); err != nil {
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			retryCount = 0
		default:
			return fmt.Errorf("checking audit retry count: %w", err)
		}
	}
	if retryCount >= maxAuditRetries {
		const fail = `
			UPDATE shovel.block_verification
			SET audit_status = 'failed', last_verified_at = now()
			WHERE src_name = $1 AND ig_name = $2 AND block_num = $3
		`
		if _, err := tx.Exec(ctx, fail, t.srcName, t.igName, t.blockNum); err != nil {
			return fmt.Errorf("marking audit failed: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("committing failed status: %w", err)
		}
		return nil
	}

	// Update status to retrying
	const r = `
		UPDATE shovel.block_verification
		SET audit_status = 'retrying', retry_count = retry_count + 1, last_verified_at = now()
		WHERE src_name = $1 AND ig_name = $2 AND block_num = $3
	`
	if _, err := tx.Exec(ctx, r, t.srcName, t.igName, t.blockNum); err != nil {
		return fmt.Errorf("updating audit status: %w", err)
	}

	// Trigger Delete/Reindex
	if err := task.Delete(tx, t.blockNum); err != nil {
		return fmt.Errorf("deleting block for reindex: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing delete tx: %w", err)
	}

	return nil
}
