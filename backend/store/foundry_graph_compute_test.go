package store_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/helixdb/helix-foundry/backend/store"
	"github.com/helixdb/helix-foundry/backend/store/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFoundryGraphComputeRunLifecycle(t *testing.T) {
	ctx := context.Background()
	db := newTestStore(t)
	token := "degree-run-1"
	run := &model.FoundryGraphComputeRun{
		ID:          id26("GR-1"),
		WorkspaceID: id26("WS-1"),
		OntologyID:  id26("GO-1"),
		LoadID:      id26("GL-1"),
		Algorithm:   "degree",
		Name:        "Degree",
		Params:      json.RawMessage(`{}`),
		ClientToken: &token,
	}
	require.NoError(t, db.CreateFoundryGraphComputeRun(ctx, run))

	// The same client token in the same ontology is a conflict.
	duplicate := *run
	duplicate.ID = id26("GR-2")
	require.ErrorIs(t, db.CreateFoundryGraphComputeRun(ctx, &duplicate), store.ErrConflict)

	got, err := db.GetFoundryGraphComputeRun(ctx, run.OntologyID, run.ID)
	require.NoError(t, err)
	assert.Equal(t, model.FoundryGraphComputeQueued, got.Status)
	assert.Equal(t, model.FoundryGraphComputePhaseQueued, got.Phase)
	assert.Equal(t, "Degree", got.Name)

	runs, err := db.ListFoundryGraphComputeRuns(ctx, run.OntologyID, 10)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	assert.Equal(t, run.ID, runs[0].ID)

	claimed, err := db.ClaimFoundryGraphComputeRun(ctx, "worker-1", time.Minute)
	require.NoError(t, err)
	require.NotNil(t, claimed)
	assert.Equal(t, run.ID, claimed.ID)
	assert.Equal(t, model.FoundryGraphComputeRunning, claimed.Status)
	assert.Equal(t, model.FoundryGraphComputePhaseComputing, claimed.Phase)
	assert.Equal(t, 1, claimed.Attempts)
	require.NotNil(t, claimed.LeaseOwner)
	assert.Equal(t, "worker-1", *claimed.LeaseOwner)

	// The only run is now leased, so a second worker finds nothing.
	none, err := db.ClaimFoundryGraphComputeRun(ctx, "worker-2", time.Minute)
	require.NoError(t, err)
	assert.Nil(t, none)

	scores := []model.FoundryGraphComputeScore{
		{RunID: run.ID, NodeKey: "a", Score: 2, Rank: 1},
		{RunID: run.ID, NodeKey: "b", Score: 1, Rank: 2},
	}
	require.NoError(t, db.InsertFoundryGraphComputeScores(ctx, run.ID, scores))
	listed, err := db.ListFoundryGraphComputeScores(ctx, run.ID, 10, 0)
	require.NoError(t, err)
	require.Len(t, listed, 2)
	assert.Equal(t, "a", listed[0].NodeKey)
	assert.Equal(t, 2.0, listed[0].Score)

	uri := "s3://bucket/graph-compute/GR-1/scores.jsonl"
	top := 2.0
	require.NoError(t, db.FinishFoundryGraphComputeRun(ctx, run.ID, "worker-1", model.FoundryGraphComputeOutcome{
		Status:    model.FoundryGraphComputeSucceeded,
		NodeCount: 2,
		EdgeCount: 1,
		TopScore:  &top,
		ResultURI: &uri,
	}))
	finished, err := db.GetFoundryGraphComputeRun(ctx, run.OntologyID, run.ID)
	require.NoError(t, err)
	assert.Equal(t, model.FoundryGraphComputeSucceeded, finished.Status)
	assert.Equal(t, model.FoundryGraphComputePhaseDone, finished.Phase)
	assert.Equal(t, int64(2), finished.NodeCount)
	assert.Equal(t, int64(1), finished.EdgeCount)
	require.NotNil(t, finished.ResultURI)
	assert.Equal(t, uri, *finished.ResultURI)

	// A worker that lost its lease cannot finish the run again.
	require.ErrorIs(t, db.FinishFoundryGraphComputeRun(ctx, run.ID, "worker-2", model.FoundryGraphComputeOutcome{
		Status: model.FoundryGraphComputeCancelled,
	}), store.ErrNotFound)
}
