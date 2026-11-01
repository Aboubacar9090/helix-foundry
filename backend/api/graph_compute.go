package api

import (
	"context"
	"encoding/json"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	foundryv1 "github.com/helixdb/helix-foundry/backend/pb/gen/go/helix/foundry/v1"
	"github.com/helixdb/helix-foundry/backend/pkg/id"
	foundrystore "github.com/helixdb/helix-foundry/backend/store"
	foundrymodel "github.com/helixdb/helix-foundry/backend/store/model"
)

// StartGraphCompute queues a read-only computation over one load. The
// foundry-graph-compute-worker claims it, reads the load's edges and persists
// the scores; nothing writes the graph. Authz has checked the caller may read
// the database.
//  1. The same client token returns an existing run's queue entry (the store
//     refuses a duplicate).
//  2. The load must be one of the ontology's and not deleted or deleting.
//  3. Only degree is implemented for now; the other algorithms follow.
func (s *Service) StartGraphCompute(ctx context.Context, req *foundryv1.StartGraphComputeRequest) (*foundryv1.StartGraphComputeResponse, error) {
	if req.GetLoadId() == "" {
		return nil, status.Error(codes.InvalidArgument, "load_id is required")
	}
	if req.GetAlgorithm() != "degree" {
		return nil, status.Error(codes.InvalidArgument, "algorithm must be degree for now")
	}
	if len(req.GetClientToken()) > maxClientTokenLength {
		return nil, status.Errorf(codes.InvalidArgument, "client_token is at most %d characters", maxClientTokenLength)
	}
	o, err := s.liveFoundryOntology(ctx, req.GetWorkspaceId(), req.GetOntologyId())
	if err != nil {
		return nil, err
	}
	load, err := s.deps.DB.GetFoundryLoad(ctx, o.ID, req.GetLoadId())
	switch {
	case errors.Is(err, foundrystore.ErrNotFound):
		return nil, status.Error(codes.NotFound, "load not found")
	case err != nil:
		return nil, status.Errorf(codes.Internal, "get load: %v", err)
	}
	switch load.Status {
	case foundrymodel.FoundryLoadDeleted, foundrymodel.FoundryLoadDeleting, foundrymodel.FoundryLoadDeleteFailed:
		return nil, status.Error(codes.FailedPrecondition, "the load is gone or being deleted")
	}
	name := req.GetName()
	if name == "" {
		name = req.GetAlgorithm()
	}
	run := &foundrymodel.FoundryGraphComputeRun{
		ID:          id.New(),
		WorkspaceID: o.WorkspaceID,
		OntologyID:  o.ID,
		LoadID:      load.ID,
		Algorithm:   req.GetAlgorithm(),
		Name:        name,
	}
	if raw := req.GetParamsJson(); raw != "" {
		run.Params = json.RawMessage(raw)
	}
	if token := req.GetClientToken(); token != "" {
		run.ClientToken = &token
	}
	if userID := actor(ctx).UserID; userID != "" {
		run.RequestedByUserID = &userID
	}
	switch err := s.deps.DB.CreateFoundryGraphComputeRun(ctx, run); {
	case errors.Is(err, foundrystore.ErrConflict):
		return nil, status.Error(codes.AlreadyExists, "a compute run with this client token already exists")
	case err != nil:
		return nil, status.Errorf(codes.Internal, "queue graph compute: %v", err)
	}
	return &foundryv1.StartGraphComputeResponse{Run: graphComputeRunProto(run)}, nil
}

func (s *Service) GetGraphCompute(ctx context.Context, req *foundryv1.GetGraphComputeRequest) (*foundryv1.GetGraphComputeResponse, error) {
	o, err := s.liveFoundryOntology(ctx, req.GetWorkspaceId(), req.GetOntologyId())
	if err != nil {
		return nil, err
	}
	run, err := s.deps.DB.GetFoundryGraphComputeRun(ctx, o.ID, req.GetRunId())
	switch {
	case errors.Is(err, foundrystore.ErrNotFound):
		return nil, status.Error(codes.NotFound, "graph compute run not found")
	case err != nil:
		return nil, status.Errorf(codes.Internal, "get graph compute run: %v", err)
	}
	return &foundryv1.GetGraphComputeResponse{Run: graphComputeRunProto(run)}, nil
}

func (s *Service) ListGraphComputes(ctx context.Context, req *foundryv1.ListGraphComputesRequest) (*foundryv1.ListGraphComputesResponse, error) {
	o, err := s.liveFoundryOntology(ctx, req.GetWorkspaceId(), req.GetOntologyId())
	if err != nil {
		return nil, err
	}
	runs, err := s.deps.DB.ListFoundryGraphComputeRuns(ctx, o.ID, int(req.GetLimit()))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list graph compute runs: %v", err)
	}
	out := make([]*foundryv1.GraphComputeRun, 0, len(runs))
	for _, run := range runs {
		out = append(out, graphComputeRunProto(run))
	}
	return &foundryv1.ListGraphComputesResponse{Runs: out}, nil
}

func (s *Service) ListGraphComputeScores(ctx context.Context, req *foundryv1.ListGraphComputeScoresRequest) (*foundryv1.ListGraphComputeScoresResponse, error) {
	o, err := s.liveFoundryOntology(ctx, req.GetWorkspaceId(), req.GetOntologyId())
	if err != nil {
		return nil, err
	}
	// The run must belong to this ontology; the store's score query alone
	// would answer for a run id from another one.
	if _, err := s.deps.DB.GetFoundryGraphComputeRun(ctx, o.ID, req.GetRunId()); err != nil {
		if errors.Is(err, foundrystore.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "graph compute run not found")
		}
		return nil, status.Errorf(codes.Internal, "get graph compute run: %v", err)
	}
	scores, err := s.deps.DB.ListFoundryGraphComputeScores(ctx, req.GetRunId(), int(req.GetLimit()), int(req.GetOffset()))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list graph compute scores: %v", err)
	}
	out := make([]*foundryv1.GraphComputeScore, 0, len(scores))
	for _, score := range scores {
		out = append(out, &foundryv1.GraphComputeScore{
			NodeKey: score.NodeKey,
			Score:   score.Score,
			Rank:    int32(score.Rank),
		})
	}
	return &foundryv1.ListGraphComputeScoresResponse{Scores: out}, nil
}

func graphComputeRunProto(run *foundrymodel.FoundryGraphComputeRun) *foundryv1.GraphComputeRun {
	out := &foundryv1.GraphComputeRun{
		Id:          run.ID,
		WorkspaceId: run.WorkspaceID,
		OntologyId:  run.OntologyID,
		LoadId:      run.LoadID,
		Algorithm:   run.Algorithm,
		Name:        run.Name,
		Status:      string(run.Status),
		Phase:       string(run.Phase),
		NodeCount:   run.NodeCount,
		EdgeCount:   run.EdgeCount,
	}
	if run.TopScore != nil {
		out.TopScore = *run.TopScore
	}
	if run.ResultURI != nil {
		out.ResultUri = *run.ResultURI
	}
	if run.Error != nil {
		out.Error = *run.Error
	}
	if !run.CreatedAt.IsZero() {
		out.CreatedAt = timestamppb.New(run.CreatedAt)
	}
	if run.StartedAt != nil {
		out.StartedAt = timestamppb.New(*run.StartedAt)
	}
	if run.FinishedAt != nil {
		out.FinishedAt = timestamppb.New(*run.FinishedAt)
	}
	return out
}
