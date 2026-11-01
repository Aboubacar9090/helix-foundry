package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/helixdb/helix-foundry/backend/store/model"
)

const foundryGraphComputeRunColumns = `id, workspace_id, ontology_id, load_id, algorithm, name, params, status, phase,
       client_token, lease_owner, lease_until, attempts, next_attempt_at, error, node_count, edge_count, top_score,
       result_uri, requested_by_user_id, created_at, started_at, finished_at`

// computeScanner is *sql.Row or *sql.Rows.
type computeScanner interface {
	Scan(dest ...any) error
}

func scanFoundryGraphComputeRun(row computeScanner) (*model.FoundryGraphComputeRun, error) {
	var run model.FoundryGraphComputeRun
	var params, clientToken, leaseOwner, errorText, resultURI, requestedBy sql.NullString
	var leaseUntil, startedAt, finishedAt sql.NullTime
	var topScore sql.NullFloat64
	var status, phase string
	err := row.Scan(&run.ID, &run.WorkspaceID, &run.OntologyID, &run.LoadID, &run.Algorithm, &run.Name,
		&params, &status, &phase, &clientToken, &leaseOwner, &leaseUntil, &run.Attempts, &run.NextAttemptAt,
		&errorText, &run.NodeCount, &run.EdgeCount, &topScore, &resultURI, &requestedBy, &run.CreatedAt,
		&startedAt, &finishedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	run.Status, run.Phase = model.FoundryGraphComputeStatus(status), model.FoundryGraphComputePhase(phase)
	if params.Valid {
		run.Params = json.RawMessage(params.String)
	}
	if clientToken.Valid {
		run.ClientToken = &clientToken.String
	}
	if leaseOwner.Valid {
		run.LeaseOwner = &leaseOwner.String
	}
	if leaseUntil.Valid {
		run.LeaseUntil = &leaseUntil.Time
	}
	if errorText.Valid {
		run.Error = &errorText.String
	}
	if topScore.Valid {
		run.TopScore = &topScore.Float64
	}
	if resultURI.Valid {
		run.ResultURI = &resultURI.String
	}
	if requestedBy.Valid {
		run.RequestedByUserID = &requestedBy.String
	}
	if startedAt.Valid {
		run.StartedAt = &startedAt.Time
	}
	if finishedAt.Valid {
		run.FinishedAt = &finishedAt.Time
	}
	return &run, nil
}

// CreateFoundryGraphComputeRun queues a run. ErrConflict means the ontology
// already has a run with the same client token.
func (s *mysqlDB) CreateFoundryGraphComputeRun(ctx context.Context, run *model.FoundryGraphComputeRun) error {
	var params any
	if len(run.Params) > 0 {
		params = string(run.Params)
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO foundry_graph_compute_runs
  (id, workspace_id, ontology_id, load_id, algorithm, name, params, status, phase, client_token, requested_by_user_id)
VALUES (?, ?, ?, ?, ?, ?, CAST(? AS JSON), 'queued', 'queued', ?, ?)`,
		run.ID, run.WorkspaceID, run.OntologyID, run.LoadID, run.Algorithm, run.Name, params,
		run.ClientToken, run.RequestedByUserID)
	if isDuplicateKey(err) {
		return ErrConflict
	}
	return err
}

func (s *mysqlDB) GetFoundryGraphComputeRun(ctx context.Context, ontologyID, id string) (*model.FoundryGraphComputeRun, error) {
	return scanFoundryGraphComputeRun(s.db.QueryRowContext(ctx, "SELECT "+foundryGraphComputeRunColumns+`
FROM foundry_graph_compute_runs WHERE id=? AND ontology_id=?`, id, ontologyID))
}

func (s *mysqlDB) ListFoundryGraphComputeRuns(ctx context.Context, ontologyID string, limit int) ([]*model.FoundryGraphComputeRun, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, "SELECT "+foundryGraphComputeRunColumns+`
FROM foundry_graph_compute_runs WHERE ontology_id=? ORDER BY created_at DESC, id DESC LIMIT ?`, ontologyID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*model.FoundryGraphComputeRun, 0, limit)
	for rows.Next() {
		run, err := scanFoundryGraphComputeRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, rows.Err()
}

// ClaimFoundryGraphComputeRun leases the oldest due run to workerID: a queued
// one whose retry time has come, or a running one whose lease expired. Nil
// means none is due.
func (s *mysqlDB) ClaimFoundryGraphComputeRun(ctx context.Context, workerID string, lease time.Duration) (_ *model.FoundryGraphComputeRun, err error) {
	if s.root == nil {
		return nil, fmt.Errorf("foundry graph compute claim requires a root database connection")
	}
	tx, err := s.root.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin foundry graph compute claim: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	run, err := scanFoundryGraphComputeRun(tx.QueryRowContext(ctx, "SELECT "+foundryGraphComputeRunColumns+`
FROM foundry_graph_compute_runs
WHERE (status='queued' AND next_attempt_at <= NOW(6)) OR (status='running' AND lease_until < NOW(6))
ORDER BY next_attempt_at, id
LIMIT 1 FOR UPDATE SKIP LOCKED`))
	if errors.Is(err, ErrNotFound) {
		_ = tx.Rollback()
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("select foundry graph compute claim: %w", err)
	}
	now := time.Now().UTC()
	leaseUntil := now.Add(lease)
	if _, err = tx.ExecContext(ctx, `
UPDATE foundry_graph_compute_runs
SET status='running', phase='computing', attempts=attempts+1, lease_owner=?, lease_until=?,
    started_at=COALESCE(started_at, NOW(6))
WHERE id=?`, workerID, leaseUntil, run.ID); err != nil {
		return nil, fmt.Errorf("claim foundry graph compute run: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit foundry graph compute claim: %w", err)
	}
	run.Status, run.Phase = model.FoundryGraphComputeRunning, model.FoundryGraphComputePhaseComputing
	run.LeaseOwner, run.LeaseUntil = &workerID, &leaseUntil
	run.Attempts++
	if run.StartedAt == nil {
		run.StartedAt = &now
	}
	return run, nil
}

// FinishFoundryGraphComputeRun records a run's outcome while workerID holds
// its lease; ErrNotFound means the lease was lost and nothing was written.
func (s *mysqlDB) FinishFoundryGraphComputeRun(ctx context.Context, id, workerID string, outcome model.FoundryGraphComputeOutcome) error {
	var errText, resultURI, topScore any
	if outcome.Error != nil {
		errText = *outcome.Error
	}
	if outcome.ResultURI != nil {
		resultURI = *outcome.ResultURI
	}
	if outcome.TopScore != nil {
		topScore = *outcome.TopScore
	}
	return execOrNotFound(ctx, s.db, `UPDATE foundry_graph_compute_runs
SET status=?, phase='done', error=?, node_count=?, edge_count=?, top_score=?, result_uri=?,
    finished_at=NOW(6), lease_owner=NULL, lease_until=NULL
WHERE id=? AND status='running' AND lease_owner=?`,
		string(outcome.Status), errText, outcome.NodeCount, outcome.EdgeCount, topScore, resultURI, id, workerID)
}

// InsertFoundryGraphComputeScores writes a run's scores in batches, replacing
// a node's row on a retry.
func (s *mysqlDB) InsertFoundryGraphComputeScores(ctx context.Context, runID string, scores []model.FoundryGraphComputeScore) error {
	const batch = 200
	for start := 0; start < len(scores); start += batch {
		end := start + batch
		if end > len(scores) {
			end = len(scores)
		}
		var b strings.Builder
		args := make([]any, 0, (end-start)*5)
		b.WriteString(`INSERT INTO foundry_graph_compute_scores (run_id, node_key, score, rank_no, extra) VALUES `)
		for i, score := range scores[start:end] {
			if i > 0 {
				b.WriteString(",")
			}
			b.WriteString("(?, ?, ?, ?, CAST(? AS JSON))")
			var extra any
			if len(score.Extra) > 0 {
				extra = string(score.Extra)
			}
			args = append(args, runID, score.NodeKey, score.Score, score.Rank, extra)
		}
		b.WriteString(` ON DUPLICATE KEY UPDATE score=VALUES(score), rank_no=VALUES(rank_no), extra=VALUES(extra)`)
		if _, err := s.db.ExecContext(ctx, b.String(), args...); err != nil {
			return err
		}
	}
	return nil
}

func (s *mysqlDB) ListFoundryGraphComputeScores(ctx context.Context, runID string, limit, offset int) ([]model.FoundryGraphComputeScore, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.db.QueryContext(ctx, `SELECT run_id, node_key, score, rank_no, extra
FROM foundry_graph_compute_scores WHERE run_id=? ORDER BY rank_no, node_key LIMIT ? OFFSET ?`, runID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]model.FoundryGraphComputeScore, 0, limit)
	for rows.Next() {
		var score model.FoundryGraphComputeScore
		var extra sql.NullString
		if err := rows.Scan(&score.RunID, &score.NodeKey, &score.Score, &score.Rank, &extra); err != nil {
			return nil, err
		}
		if extra.Valid {
			score.Extra = json.RawMessage(extra.String)
		}
		out = append(out, score)
	}
	return out, rows.Err()
}
