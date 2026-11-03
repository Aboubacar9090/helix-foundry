package computetest

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/helixdb/helix-foundry/backend/store"
	"github.com/helixdb/helix-foundry/backend/store/model"
)

// workspaces is the host's table stub: the compute queue never reads it.
type workspaces struct{}

func (workspaces) Table() string { return "workspaces" }
func (workspaces) Lock(context.Context, store.Executor, string) error {
	return nil
}
func (workspaces) Deleting(context.Context, store.Executor, string) (bool, error) {
	return false, nil
}

// TestFoundryGraphComputeRunLifecycle exercises the compute queue on a real
// MySQL: it creates its own schema and applies only the compute migration,
// so it does not depend on the rest of the migration stack.
func TestFoundryGraphComputeRunLifecycle(t *testing.T) {
	dsn := os.Getenv("HELIX_TEST_ROOT_DSN")
	if dsn == "" {
		t.Skip("HELIX_TEST_ROOT_DSN is not set")
	}
	root, err := sql.Open("mysql", dsn)
	require.NoError(t, err)
	defer root.Close()
	_, err = root.Exec("DROP DATABASE IF EXISTS foundry_graph_compute_test")
	require.NoError(t, err)
	_, err = root.Exec("CREATE DATABASE foundry_graph_compute_test")
	require.NoError(t, err)

	db, err := sql.Open("mysql", strings.TrimSuffix(dsn, "/")+"/foundry_graph_compute_test?multiStatements=true&parseTime=true&clientFoundRows=true&loc=UTC")
	require.NoError(t, err)
	defer db.Close()
	raw, err := os.ReadFile("../migrations/20261110000000_foundry_graph_compute.sql")
	require.NoError(t, err)
	up := strings.SplitN(string(raw), "-- +goose Down", 2)[0]
	up = strings.ReplaceAll(up, "-- +goose Up", "")
	_, err = db.Exec(up)
	require.NoError(t, err)

	ctx := context.Background()
	st := store.NewMySQL(db, workspaces{})
	token := "degree-run-1"
	run := &model.FoundryGraphComputeRun{
		ID:          "GR-1",
		WorkspaceID: "WS-1",
		OntologyID:  "GO-1",
		LoadID:      "GL-1",
		Algorithm:   "degree",
		Name:        "Degree",
		Params:      json.RawMessage(`{}`),
		ClientToken: &token,
	}
	require.NoError(t, st.CreateFoundryGraphComputeRun(ctx, run))

	duplicate := *run
	duplicate.ID = "GR-2"
	require.ErrorIs(t, st.CreateFoundryGraphComputeRun(ctx, &duplicate), store.ErrConflict)

	got, err := st.GetFoundryGraphComputeRun(ctx, run.OntologyID, run.ID)
	require.NoError(t, err)
	assert.Equal(t, model.FoundryGraphComputeQueued, got.Status)
	assert.Equal(t, model.FoundryGraphComputePhaseQueued, got.Phase)

	runs, err := st.ListFoundryGraphComputeRuns(ctx, run.OntologyID, 10)
	require.NoError(t, err)
	require.Len(t, runs, 1)

	claimed, err := st.ClaimFoundryGraphComputeRun(ctx, "worker-1", time.Minute)
	require.NoError(t, err)
	require.NotNil(t, claimed)
	assert.Equal(t, run.ID, claimed.ID)
	assert.Equal(t, model.FoundryGraphComputeRunning, claimed.Status)
	assert.Equal(t, model.FoundryGraphComputePhaseComputing, claimed.Phase)
	assert.Equal(t, 1, claimed.Attempts)

	none, err := st.ClaimFoundryGraphComputeRun(ctx, "worker-2", time.Minute)
	require.NoError(t, err)
	assert.Nil(t, none)

	scores := []model.FoundryGraphComputeScore{
		{RunID: run.ID, NodeKey: "a", Score: 2, Rank: 1},
		{RunID: run.ID, NodeKey: "b", Score: 1, Rank: 2},
	}
	require.NoError(t, st.InsertFoundryGraphComputeScores(ctx, run.ID, scores))
	listed, err := st.ListFoundryGraphComputeScores(ctx, run.ID, 10, 0)
	require.NoError(t, err)
	require.Len(t, listed, 2)
	assert.Equal(t, "a", listed[0].NodeKey)

	uri := "s3://bucket/graph-compute/GR-1/scores.jsonl"
	top := 2.0
	require.NoError(t, st.FinishFoundryGraphComputeRun(ctx, run.ID, "worker-1", model.FoundryGraphComputeOutcome{
		Status:    model.FoundryGraphComputeSucceeded,
		NodeCount: 2,
		EdgeCount: 1,
		TopScore:  &top,
		ResultURI: &uri,
	}))
	finished, err := st.GetFoundryGraphComputeRun(ctx, run.OntologyID, run.ID)
	require.NoError(t, err)
	assert.Equal(t, model.FoundryGraphComputeSucceeded, finished.Status)
	assert.Equal(t, model.FoundryGraphComputePhaseDone, finished.Phase)
	require.NotNil(t, finished.ResultURI)
	assert.Equal(t, uri, *finished.ResultURI)

	require.ErrorIs(t, st.FinishFoundryGraphComputeRun(ctx, run.ID, "worker-2", model.FoundryGraphComputeOutcome{
		Status: model.FoundryGraphComputeCancelled,
	}), store.ErrNotFound)
}
