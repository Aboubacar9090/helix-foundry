package model

import (
	"encoding/json"
	"time"
)

// FoundryGraphComputeStatus is where a compute run stands in its queue.
type FoundryGraphComputeStatus string

const (
	FoundryGraphComputeQueued    FoundryGraphComputeStatus = "queued"
	FoundryGraphComputeRunning   FoundryGraphComputeStatus = "running"
	FoundryGraphComputeSucceeded FoundryGraphComputeStatus = "succeeded"
	FoundryGraphComputeFailed    FoundryGraphComputeStatus = "failed"
	FoundryGraphComputeCancelled FoundryGraphComputeStatus = "cancelled"
)

// FoundryGraphComputePhase is the step a running compute is in.
type FoundryGraphComputePhase string

const (
	FoundryGraphComputePhaseQueued    FoundryGraphComputePhase = "queued"
	FoundryGraphComputePhaseComputing FoundryGraphComputePhase = "computing"
	FoundryGraphComputePhaseWriting   FoundryGraphComputePhase = "writing"
	FoundryGraphComputePhaseFinishing FoundryGraphComputePhase = "finishing"
)

// FoundryGraphComputeRun is one read-only graph computation over a load.
type FoundryGraphComputeRun struct {
	ID                string                    `json:"id"`
	WorkspaceID       string                    `json:"workspaceId"`
	OntologyID        string                    `json:"ontologyId"`
	LoadID            string                    `json:"loadId"`
	Algorithm         string                    `json:"algorithm"`
	Name              string                    `json:"name"`
	Params            json.RawMessage           `json:"params,omitempty"`
	Status            FoundryGraphComputeStatus `json:"status"`
	Phase             FoundryGraphComputePhase  `json:"phase"`
	ClientToken       *string                   `json:"clientToken,omitempty"`
	LeaseOwner        *string                   `json:"leaseOwner,omitempty"`
	LeaseUntil        *time.Time                `json:"leaseUntil,omitempty"`
	Attempts          int                       `json:"attempts"`
	NextAttemptAt     time.Time                 `json:"nextAttemptAt"`
	Error             *string                   `json:"error,omitempty"`
	NodeCount         int64                     `json:"nodeCount"`
	EdgeCount         int64                     `json:"edgeCount"`
	TopScore          *float64                  `json:"topScore,omitempty"`
	ResultURI         *string                   `json:"resultUri,omitempty"`
	RequestedByUserID *string                   `json:"requestedByUserId,omitempty"`
	CreatedAt         time.Time                 `json:"createdAt"`
	StartedAt         *time.Time                `json:"startedAt,omitempty"`
	FinishedAt        *time.Time                `json:"finishedAt,omitempty"`
}

// FoundryGraphComputeScore is one node's score in a run.
type FoundryGraphComputeScore struct {
	RunID   string          `json:"runId"`
	NodeKey string          `json:"nodeKey"`
	Score   float64         `json:"score"`
	Rank    int             `json:"rank"`
	Extra   json.RawMessage `json:"extra,omitempty"`
}

// FoundryGraphComputeFinding is one fraud finding in a run.
type FoundryGraphComputeFinding struct {
	RunID       string          `json:"runId"`
	FindingID   string          `json:"findingId"`
	Recipe      string          `json:"recipe"`
	Severity    string          `json:"severity"`
	Score       float64         `json:"score"`
	NodeKeys    json.RawMessage `json:"nodeKeys,omitempty"`
	Amount      *int64          `json:"amount,omitempty"`
	Explanation *string         `json:"explanation,omitempty"`
}
