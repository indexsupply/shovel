package shovel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/indexsupply/shovel/eth"
	"github.com/indexsupply/shovel/shovel/config"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RepairService struct {
	pgp     *pgxpool.Pool
	conf    config.Root
	manager *Manager
}

func NewRepairService(pgp *pgxpool.Pool, conf config.Root, mgr *Manager) *RepairService {
	return &RepairService{
		pgp:     pgp,
		conf:    conf,
		manager: mgr,
	}
}

type RepairRequest struct {
	Source            string `json:"source"`
	Integration       string `json:"integration"`
	StartBlock        uint64 `json:"start_block"`
	EndBlock          uint64 `json:"end_block"`
	ForceAllProviders bool   `json:"force_all_providers"`
	DryRun            bool   `json:"dry_run"`
}

type RepairResponse struct {
	RepairID          string     `json:"repair_id"`
	Source            string     `json:"source"`
	Integration       string     `json:"integration"`
	StartBlock        uint64     `json:"start_block"`
	EndBlock          uint64     `json:"end_block"`
	Status            string     `json:"status"` // in_progress, completed, failed
	BlocksRequested   int        `json:"blocks_requested"`
	BlocksDeleted     int        `json:"blocks_deleted"`
	BlocksReprocessed int        `json:"blocks_reprocessed"`
	BlocksRemaining   int        `json:"blocks_remaining"`
	RowsAffected      int        `json:"rows_affected"` // For dry-run
	Errors            []string   `json:"errors"`
	CreatedAt         time.Time  `json:"created_at"`
	CompletedAt       *time.Time `json:"completed_at,omitempty"`
}

func (rs *RepairService) HandleRepairRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req RepairRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	// Validate
	if req.Source == "" || req.Integration == "" {
		http.Error(w, "source and integration required", http.StatusBadRequest)
		return
	}
	if req.StartBlock > req.EndBlock {
		http.Error(w, "start_block must be <= end_block", http.StatusBadRequest)
		return
	}
	if req.EndBlock-req.StartBlock > 10000 {
		http.Error(w, "Range exceeds maximum of 10,000 blocks", http.StatusBadRequest)
		return
	}

	// Check source and integration exist
	sources, err := rs.conf.AllSources(r.Context(), rs.pgp)
	if err != nil {
		http.Error(w, fmt.Sprintf("Loading sources: %v", err), http.StatusInternalServerError)
		return
	}
	var srcExists bool
	for _, sc := range sources {
		if sc.Name == req.Source {
			srcExists = true
			break
		}
	}
	if !srcExists {
		http.Error(w, "Source not found", http.StatusNotFound)
		return
	}

	integrations, err := rs.conf.AllIntegrations(r.Context(), rs.pgp)
	if err != nil {
		http.Error(w, fmt.Sprintf("Loading integrations: %v", err), http.StatusInternalServerError)
		return
	}
	var igExists bool
	for _, ig := range integrations {
		if ig.Name == req.Integration {
			igExists = true
			break
		}
	}
	if !igExists {
		http.Error(w, "Integration not found", http.StatusNotFound)
		return
	}

	// Check for overlapping in-progress repairs
	const overlapQ = `
		SELECT repair_id, start_block, end_block
		FROM shovel.repair_jobs
		WHERE src_name = $1
		AND ig_name = $2
		AND status = 'in_progress'
		AND (
			(start_block <= $3 AND end_block >= $3) OR
			(start_block <= $4 AND end_block >= $4) OR
			(start_block >= $3 AND end_block <= $4)
		)
	`
	var existingRepairID string
	var existingStart, existingEnd uint64
	err = rs.pgp.QueryRow(r.Context(), overlapQ, req.Source, req.Integration, req.StartBlock, req.EndBlock).Scan(&existingRepairID, &existingStart, &existingEnd)
	if err == nil {
		// Found overlapping repair
		http.Error(w, fmt.Sprintf("Overlapping repair in progress: %s (blocks %d-%d)", existingRepairID, existingStart, existingEnd), http.StatusConflict)
		return
	} else if !errors.Is(err, pgx.ErrNoRows) {
		// Query error (not just "no rows")
		http.Error(w, fmt.Sprintf("Checking for overlaps: %v", err), http.StatusInternalServerError)
		return
	}
	// ErrNoRows is expected when no overlap found - proceed

	if req.DryRun {
		// Count affected rows
		count, err := rs.countAffectedRows(r.Context(), req.Source, req.Integration, req.StartBlock, req.EndBlock)
		if err != nil {
			http.Error(w, fmt.Sprintf("Dry run failed: %v", err), http.StatusInternalServerError)
			return
		}
		resp := RepairResponse{
			Status:       "dry_run",
			RowsAffected: count,
			CreatedAt:    time.Now(),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	}

	// Create repair job
	repairID := uuid.New().String()
	const insertQ = `
		INSERT INTO shovel.repair_jobs (repair_id, src_name, ig_name, start_block, end_block, status)
		VALUES ($1, $2, $3, $4, $5, 'in_progress')
	`
	if _, err := rs.pgp.Exec(r.Context(), insertQ, repairID, req.Source, req.Integration, req.StartBlock, req.EndBlock); err != nil {
		http.Error(w, fmt.Sprintf("Creating repair job: %v", err), http.StatusInternalServerError)
		return
	}

	RepairRequests.WithLabelValues(req.Source, req.Integration).Inc()

	// Log audit
	const auditQ = `
		INSERT INTO shovel.repair_audit (repair_id, requester, src_name, ig_name, start_block, end_block, dry_run)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	requester := r.RemoteAddr
	if _, err := rs.pgp.Exec(r.Context(), auditQ, repairID, requester, req.Source, req.Integration, req.StartBlock, req.EndBlock, req.DryRun); err != nil {
		slog.ErrorContext(r.Context(), "audit log failed", "error", err)
	}

	// Start repair in background with request-derived context
	// Use generous timeout to prevent indefinite repairs while allowing proper cancellation
	timeout := time.Duration((req.EndBlock-req.StartBlock)/1000) * time.Hour
	if timeout < 10*time.Minute {
		timeout = 10 * time.Minute
	}
	repairCtx, cancel := context.WithTimeout(r.Context(), timeout)
	go func() {
		defer cancel()
		rs.executeRepair(repairCtx, repairID, req)
	}()

	resp := RepairResponse{
		RepairID:        repairID,
		Status:          "in_progress",
		BlocksRequested: int(req.EndBlock - req.StartBlock + 1),
		CreatedAt:       time.Now(),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (rs *RepairService) HandleRepairStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract repair_id from path: /api/v1/repair/{repair_id}
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 5 {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}
	repairID := parts[4]

	const q = `
		SELECT repair_id, src_name, ig_name, start_block, end_block, status, 
		       blocks_deleted, blocks_reprocessed, errors, created_at, completed_at
		FROM shovel.repair_jobs
		WHERE repair_id = $1
	`
	var resp RepairResponse
	var errorsJSON []byte
	err := rs.pgp.QueryRow(r.Context(), q, repairID).Scan(
		&resp.RepairID, &resp.Source, &resp.Integration, &resp.StartBlock, &resp.EndBlock,
		&resp.Status, &resp.BlocksDeleted, &resp.BlocksReprocessed, &errorsJSON,
		&resp.CreatedAt, &resp.CompletedAt,
	)
	if err != nil {
		http.Error(w, "Repair job not found", http.StatusNotFound)
		return
	}

	if len(errorsJSON) > 0 {
		json.Unmarshal(errorsJSON, &resp.Errors)
	}

	resp.BlocksRequested = int(resp.EndBlock - resp.StartBlock + 1)
	resp.BlocksRemaining = resp.BlocksRequested - resp.BlocksReprocessed

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (rs *RepairService) HandleListRepairs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	status := r.URL.Query().Get("status")
	var q string
	var args []interface{}
	if status != "" {
		q = `SELECT repair_id, src_name, ig_name, start_block, end_block, status, 
		            blocks_deleted, blocks_reprocessed, errors, created_at, completed_at
		     FROM shovel.repair_jobs WHERE status = $1 ORDER BY created_at DESC LIMIT 100`
		args = append(args, status)
	} else {
		q = `SELECT repair_id, src_name, ig_name, start_block, end_block, status, 
		            blocks_deleted, blocks_reprocessed, errors, created_at, completed_at
		     FROM shovel.repair_jobs ORDER BY created_at DESC LIMIT 100`
	}

	rows, err := rs.pgp.Query(r.Context(), q, args...)
	if err != nil {
		http.Error(w, fmt.Sprintf("Query failed: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var repairs []RepairResponse
	for rows.Next() {
		var resp RepairResponse
		var errorsJSON []byte
		if err := rows.Scan(
			&resp.RepairID, &resp.Source, &resp.Integration, &resp.StartBlock, &resp.EndBlock,
			&resp.Status, &resp.BlocksDeleted, &resp.BlocksReprocessed, &errorsJSON,
			&resp.CreatedAt, &resp.CompletedAt,
		); err != nil {
			http.Error(w, fmt.Sprintf("Scan failed: %v", err), http.StatusInternalServerError)
			return
		}
		if len(errorsJSON) > 0 {
			json.Unmarshal(errorsJSON, &resp.Errors)
		}
		resp.BlocksRequested = int(resp.EndBlock - resp.StartBlock + 1)
		resp.BlocksRemaining = resp.BlocksRequested - resp.BlocksReprocessed
		repairs = append(repairs, resp)
	}

	result := struct {
		Repairs []RepairResponse `json:"repairs"`
		Total   int              `json:"total"`
	}{
		Repairs: repairs,
		Total:   len(repairs),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (rs *RepairService) countAffectedRows(ctx context.Context, srcName, igName string, start, end uint64) (int, error) {
	// Find the integration to get table name
	integrations, err := rs.conf.AllIntegrations(ctx, rs.pgp)
	if err != nil {
		return 0, err
	}
	var tableName string
	var validTable bool
	for _, ig := range integrations {
		if ig.Name == igName {
			tableName = ig.Table.Name
			validTable = true
			break
		}
	}
	if !validTable || tableName == "" {
		return 0, fmt.Errorf("integration table not found for %s", igName)
	}

	q := fmt.Sprintf(`
		SELECT COUNT(*) FROM %s
		WHERE src_name = $1 AND ig_name = $2 AND block_num >= $3 AND block_num <= $4
	`, tableName)
	
	var count int
	err = rs.pgp.QueryRow(ctx, q, srcName, igName, start, end).Scan(&count)
	return count, err
}

func (rs *RepairService) executeRepair(ctx context.Context, repairID string, req RepairRequest) {
	// Check if manager is initialized
	if rs.manager == nil {
		rs.updateRepairStatus(ctx, repairID, "failed", []string{"RepairService not initialized with Manager"})
		return
	}

	// Find task first (needed for lockid)
	var task *Task
	for _, t := range rs.manager.tasks {
		if t.srcName == req.Source && t.destConfig.Name == req.Integration {
			task = t
			break
		}
	}
	if task == nil {
		rs.updateRepairStatus(ctx, repairID, "failed", []string{"Task not found"})
		return
	}

	// Use transaction-scoped advisory lock to coordinate with Task.Converge()
	// This reuses task.lockid to prevent races between repair and live indexing
	pgtx, err := rs.pgp.Begin(ctx)
	if err != nil {
		rs.updateRepairStatus(ctx, repairID, "failed", []string{fmt.Sprintf("Begin tx: %v", err)})
		return
	}
	defer pgtx.Rollback(ctx)

	if _, err := pgtx.Exec(ctx, "select pg_advisory_xact_lock($1)", task.lockid); err != nil {
		rs.updateRepairStatus(ctx, repairID, "failed", []string{fmt.Sprintf("Acquiring task lock: %v", err)})
		return
	}

	// Delete data for the block range
	var blocksDeleted int
	for blockNum := req.StartBlock; blockNum <= req.EndBlock; blockNum++ {
		if err := task.Delete(pgtx, blockNum); err != nil {
			rs.updateRepairStatus(ctx, repairID, "failed", []string{fmt.Sprintf("Delete block %d: %v", blockNum, err)})
			return
		}
		blocksDeleted++
	}

	// Commit deletion before reindexing
	if err := pgtx.Commit(ctx); err != nil {
		rs.updateRepairStatus(ctx, repairID, "failed", []string{fmt.Sprintf("Commit delete tx: %v", err)})
		return
	}

	// Update blocks_deleted
	const updateDeletedQ = `UPDATE shovel.repair_jobs SET blocks_deleted = $1 WHERE repair_id = $2`
	if _, err := rs.pgp.Exec(ctx, updateDeletedQ, blocksDeleted, repairID); err != nil {
		slog.ErrorContext(ctx, "failed-to-update-blocks-deleted",
			"repair_id", repairID,
			"blocks_deleted", blocksDeleted,
			"error", err,
		)
	}

	// Reindex the deleted blocks
	var blocksReprocessed int
	var reindexErrors []string

	if req.ForceAllProviders && task.consensus != nil {
		// Force all providers mode: iterate through each provider individually
		// This exhaustively queries every configured RPC endpoint instead of using consensus
		for _, provider := range task.consensus.providers {
			url := provider.NextURL().String()
			slog.InfoContext(ctx, "repair-force-provider",
				"repair_id", repairID,
				"provider", url,
				"blocks", fmt.Sprintf("%d-%d", req.StartBlock, req.EndBlock),
			)

			// Reindex block range using this specific provider
			n, err := rs.reindexBlocks(ctx, task, url, req.StartBlock, req.EndBlock)
			if err != nil {
				reindexErrors = append(reindexErrors, fmt.Sprintf("Provider %s: %v", url, err))
				continue
			}
			blocksReprocessed += n
		}
	} else {
		// Normal mode: use consensus or single provider (whatever task is configured with)
		var url string
		if task.consensus != nil {
			// Consensus mode: let FetchWithQuorum handle provider selection
			url = "" // Not used in consensus path
		} else {
			// Single provider mode
			url = task.src.NextURL().String()
		}

		n, err := rs.reindexBlocks(ctx, task, url, req.StartBlock, req.EndBlock)
		if err != nil {
			reindexErrors = append(reindexErrors, fmt.Sprintf("Reindex: %v", err))
		} else {
			blocksReprocessed = n
		}
	}

	// Update blocks_reprocessed metric with actual reindexed count
	RepairBlocksReprocessed.WithLabelValues(req.Source, req.Integration).Add(float64(blocksReprocessed))

	// Update repair job status
	const updateReprocessedQ = `UPDATE shovel.repair_jobs SET blocks_reprocessed = $1 WHERE repair_id = $2`
	if _, err := rs.pgp.Exec(ctx, updateReprocessedQ, blocksReprocessed, repairID); err != nil {
		slog.ErrorContext(ctx, "failed-to-update-blocks-reprocessed",
			"repair_id", repairID,
			"blocks_reprocessed", blocksReprocessed,
			"error", err,
		)
	}

	if len(reindexErrors) > 0 {
		rs.updateRepairStatus(ctx, repairID, "failed", reindexErrors)
	} else {
		rs.updateRepairStatus(ctx, repairID, "completed", nil)
	}
}

// reindexBlocks fetches and inserts blocks for the given range
// Returns the number of blocks successfully reprocessed
func (rs *RepairService) reindexBlocks(ctx context.Context, task *Task, url string, start, end uint64) (int, error) {
	var totalReprocessed int

	// Process in batches to avoid overwhelming memory and transactions
	const batchSize = 100
	for blockNum := start; blockNum <= end; blockNum += batchSize {
		batchEnd := blockNum + batchSize - 1
		if batchEnd > end {
			batchEnd = end
		}
		limit := batchEnd - blockNum + 1

		// Start new transaction for this batch with advisory lock
		pgtx, err := rs.pgp.Begin(ctx)
		if err != nil {
			return totalReprocessed, fmt.Errorf("begin tx for batch %d-%d: %w", blockNum, batchEnd, err)
		}

		if _, err := pgtx.Exec(ctx, "select pg_advisory_xact_lock($1)", task.lockid); err != nil {
			pgtx.Rollback(ctx)
			return totalReprocessed, fmt.Errorf("acquiring task lock: %w", err)
		}

	// Fetch blocks with reorg retry logic (mirrors Task.Converge pattern)
	// Repair uses fewer retries (10) than live indexing (1000) since repairs
	// are typically run on stable historical ranges
	const maxReorgRetries = 10
	var blocks []eth.Block
	var fetchErr error

	for attempt := 0; attempt < maxReorgRetries; attempt++ {
		if task.consensus != nil && url == "" {
			// Consensus mode
			blocks, _, fetchErr = task.consensus.FetchWithQuorum(ctx, &task.filter, blockNum, limit)
			// Reorg detection for consensus path
			if fetchErr == nil && len(blocks) > 0 && blockNum > 0 {
				first := blocks[0]
				if len(first.Header.Parent) == 32 {
					// Need to fetch previous block hash to detect reorg
					var prevHash []byte
					prevHash, fetchErr = task.src.Hash(ctx, url, blockNum-1)
					if fetchErr == nil && !bytes.Equal(prevHash, first.Header.Parent) {
						fetchErr = ErrReorg
					}
				}
			}
		} else {
			// Single provider mode (or force_all_providers with specific URL)
			// Get previous block hash for reorg detection
			var prevHash []byte
			if blockNum > 0 {
				prevHash, fetchErr = task.src.Hash(ctx, url, blockNum-1)
				if fetchErr != nil {
					pgtx.Rollback(ctx)
					return totalReprocessed, fmt.Errorf("getting prev hash for %d: %w", blockNum-1, fetchErr)
				}
			}
			blocks, fetchErr = task.load(ctx, url, prevHash, blockNum, limit)
		}

		if errors.Is(fetchErr, ErrReorg) {
			slog.WarnContext(ctx, "reorg-during-repair",
				"block", blockNum,
				"attempt", attempt+1,
				"max_retries", maxReorgRetries,
			)
			// Retry with updated chain state
			continue
		}

		// Success or non-reorg error - break out of retry loop
		break
	}

	if fetchErr != nil {
		pgtx.Rollback(ctx)
		if errors.Is(fetchErr, ErrReorg) {
			return totalReprocessed, fmt.Errorf("exhausted reorg retries (%d attempts) for blocks %d-%d", maxReorgRetries, blockNum, batchEnd)
		}
		return totalReprocessed, fmt.Errorf("loading blocks %d-%d: %w", blockNum, batchEnd, fetchErr)
	}

		if len(blocks) == 0 {
			pgtx.Rollback(ctx)
			continue
		}

		// Insert blocks
		nrows, err := task.insert(ctx, pgtx, blocks)
		if err != nil {
			pgtx.Rollback(ctx)
			return totalReprocessed, fmt.Errorf("inserting blocks %d-%d: %w", blockNum, batchEnd, err)
		}

		// Update task_updates for this batch
		last := blocks[len(blocks)-1]
		const updateTaskQ = `
			INSERT INTO shovel.task_updates (src_name, ig_name, num, hash)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (ig_name, src_name, num)
			DO UPDATE SET hash = excluded.hash
		`
		if _, err := pgtx.Exec(ctx, updateTaskQ, task.srcName, task.destConfig.Name, last.Num(), last.Hash()); err != nil {
			pgtx.Rollback(ctx)
			return totalReprocessed, fmt.Errorf("updating task_updates: %w", err)
		}

		if err := pgtx.Commit(ctx); err != nil {
			return totalReprocessed, fmt.Errorf("committing batch %d-%d: %w", blockNum, batchEnd, err)
		}

		totalReprocessed += len(blocks)
		slog.InfoContext(ctx, "repair-batch-completed",
			"blocks", fmt.Sprintf("%d-%d", blockNum, batchEnd),
			"nrows", nrows,
			"nblocks", len(blocks),
		)
	}

	return totalReprocessed, nil
}

func (rs *RepairService) updateRepairStatus(ctx context.Context, repairID, status string, errs []string) {
	var errorsJSON []byte
	if len(errs) > 0 {
		errorsJSON, _ = json.Marshal(errs)
	}
	const q = `
		UPDATE shovel.repair_jobs
		SET status = $1, errors = $2, completed_at = now()
		WHERE repair_id = $3
	`
	if _, err := rs.pgp.Exec(ctx, q, status, errorsJSON, repairID); err != nil {
		slog.ErrorContext(ctx, "failed-to-update-repair-status",
			"repair_id", repairID,
			"status", status,
			"error", err,
		)
	}
}
