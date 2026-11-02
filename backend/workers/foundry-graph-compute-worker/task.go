// Package foundrygraphcomputeworker runs read-only graph computations over a
// load. It claims one run at a time, reads that load's edges through the
// graph reader, computes the requested algorithm and persists the scores.
// Compute runs never write the graph, so they do not hold the load writer's
// lease.
package foundrygraphcomputeworker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"go.uber.org/dig"

	"github.com/helixdb/helix-foundry/backend/config"
	"github.com/helixdb/helix-foundry/backend/graphcompute"
	"github.com/helixdb/helix-foundry/backend/pkg/id"
	"github.com/helixdb/helix-foundry/backend/pkg/log"
	"github.com/helixdb/helix-foundry/backend/ports"
	"github.com/helixdb/helix-foundry/backend/server/tasks"
	foundrystore "github.com/helixdb/helix-foundry/backend/store"
	"github.com/helixdb/helix-foundry/backend/store/model"
)

const Name = "foundry-graph-compute-worker"

const (
	pollInterval   = 5 * time.Second
	lease          = 2 * time.Minute
	processTimeout = 90 * time.Second
	finishTimeout  = 15 * time.Second
	// edgeLimit bounds one run's edge read; algorithms that need every edge
	// fail loudly rather than compute on a truncated graph.
	edgeLimit = 100_000
)

type Deps struct {
	dig.In

	Cfg *config.Config
	AWS aws.Config
	DB  foundrystore.Store
	// Graph runs read queries on the ontology's database with the API's
	// checks and limits.
	Graph ports.GraphReader
}

// Task runs queued compute runs one at a time.
type Task struct {
	db       foundrystore.Store
	graph    ports.GraphReader
	s3       *s3.Client
	bucket   string
	workerID string
}

func New(d Deps) (*Task, error) {
	if d.DB == nil {
		return nil, errors.New("foundry-graph-compute-worker: db not configured")
	}
	if d.Graph == nil {
		return nil, errors.New("foundry-graph-compute-worker: graph reader not configured")
	}
	if d.Cfg == nil || d.Cfg.Foundry.Sync.DataBucket == "" {
		return nil, errors.New("foundry-graph-compute-worker: data bucket not configured")
	}
	return &Task{
		db:       d.DB,
		graph:    d.Graph,
		s3:       s3.NewFromConfig(d.AWS),
		bucket:   d.Cfg.Foundry.Sync.DataBucket,
		workerID: id.NewWorkerID(),
	}, nil
}

func (t *Task) Name() string { return Name }

func (t *Task) Start(ctx context.Context) error {
	tasks.Tick(ctx, pollInterval, t.drain)
	return nil
}

func (t *Task) Stop(context.Context) error { return nil }

// drain runs due runs until none is left.
func (t *Task) drain(ctx context.Context) {
	for ctx.Err() == nil {
		run, err := t.db.ClaimFoundryGraphComputeRun(ctx, t.workerID, lease)
		if err != nil {
			log.From(ctx).Error("claim graph compute run failed", "err", err)
			return
		}
		if run == nil {
			return
		}
		t.process(ctx, run)
	}
}

func (t *Task) process(ctx context.Context, run *model.FoundryGraphComputeRun) {
	ctx, cancel := context.WithTimeout(ctx, processTimeout)
	defer cancel()
	fail := func(message string) {
		log.From(ctx).Warn("graph compute run failed", "run", run.ID, "err", message)
		t.finish(ctx, run, model.FoundryGraphComputeOutcome{
			Status: model.FoundryGraphComputeFailed,
			Error:  &message,
		})
	}
	ontology, err := t.db.GetFoundryOntology(ctx, run.WorkspaceID, run.OntologyID)
	if err != nil {
		fail("get ontology: " + err.Error())
		return
	}
	if ontology.TenantID == "" {
		fail("the ontology has no database")
		return
	}
	load, err := t.db.GetFoundryLoad(ctx, run.OntologyID, run.LoadID)
	if err != nil {
		fail("get load: " + err.Error())
		return
	}
	switch load.Status {
	case model.FoundryLoadDeleted, model.FoundryLoadDeleting, model.FoundryLoadDeleteFailed:
		fail("the load is gone or being deleted")
		return
	}
	var scores []model.FoundryGraphComputeScore
	var edgeCount int64
	switch run.Algorithm {
	case "degree":
		query, err := degreeQuery(load.ID)
		if err != nil {
			fail("encode degree query: " + err.Error())
			return
		}
		body, err := t.readGraph(ctx, ontology.TenantID, query)
		if err != nil {
			fail(err.Error())
			return
		}
		scores, edgeCount, err = parseDegree(run.ID, body)
		if err != nil {
			fail("parse degree: " + err.Error())
			return
		}
	case "wcc", "pagerank":
		query, err := edgesQuery(load.ID, edgeLimit)
		if err != nil {
			fail("encode edge query: " + err.Error())
			return
		}
		body, err := t.readGraph(ctx, ontology.TenantID, query)
		if err != nil {
			fail(err.Error())
			return
		}
		edges, err := parseEdges(body)
		if err != nil {
			fail("parse edges: " + err.Error())
			return
		}
		edgeCount = int64(len(edges))
		if run.Algorithm == "wcc" {
			scores = wccScores(run.ID, edges)
		} else {
			scores = pagerankScores(run.ID, edges)
		}
	default:
		fail(fmt.Sprintf("algorithm %q is not implemented yet", run.Algorithm))
		return
	}
	resultURI, err := t.writeScores(ctx, run.ID, scores)
	if err != nil {
		fail("save scores to S3: " + err.Error())
		return
	}
	if err := t.db.InsertFoundryGraphComputeScores(ctx, run.ID, scores); err != nil {
		fail("save scores: " + err.Error())
		return
	}
	outcome := model.FoundryGraphComputeOutcome{
		Status:    model.FoundryGraphComputeSucceeded,
		NodeCount: int64(len(scores)),
		EdgeCount: edgeCount,
		ResultURI: &resultURI,
	}
	if len(scores) > 0 {
		outcome.TopScore = &scores[0].Score
	}
	t.finish(ctx, run, outcome)
}

// writeScores stores the full, ranked result as JSON lines and returns its
// S3 URI. The table in MySQL serves the first pages; this is the complete
// result.
func (t *Task) writeScores(ctx context.Context, runID string, scores []model.FoundryGraphComputeScore) (string, error) {
	data, err := encodeScores(scores)
	if err != nil {
		return "", err
	}
	key := fmt.Sprintf("graph-compute/%s/scores.jsonl", runID)
	if _, err := t.s3.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(t.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String("application/x-ndjson"),
	}); err != nil {
		return "", err
	}
	return "s3://" + t.bucket + "/" + key, nil
}

func encodeScores(scores []model.FoundryGraphComputeScore) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	for _, score := range scores {
		if err := encoder.Encode(struct {
			NodeKey string  `json:"node_key"`
			Score   float64 `json:"score"`
			Rank    int     `json:"rank"`
		}{score.NodeKey, score.Score, score.Rank}); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

// finish records the outcome even when the attempt's context has ended, and
// ignores a run whose lease was lost.
func (t *Task) finish(ctx context.Context, run *model.FoundryGraphComputeRun, outcome model.FoundryGraphComputeOutcome) {
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), finishTimeout)
	defer cancel()
	if err := t.db.FinishFoundryGraphComputeRun(finishCtx, run.ID, t.workerID, outcome); err != nil &&
		!errors.Is(err, foundrystore.ErrNotFound) {
		log.From(ctx).Error("finish graph compute run failed", "run", run.ID, "err", err)
	}
}

// readGraph runs one read query and returns its body, mapping a non-2xx
// database answer to an error with a short body.
func (t *Task) readGraph(ctx context.Context, tenantID string, query []byte) ([]byte, error) {
	status, body, err := t.graph.ReadQuery(ctx, tenantID, query)
	if err != nil {
		return nil, fmt.Errorf("read graph: %w", err)
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("read graph returned %d: %s", status, truncate(body, 200))
	}
	return body, nil
}

// edgesQuery reads the load's edges with their two ends, bounded by limit.
func edgesQuery(loadID string, limit int) ([]byte, error) {
	var source map[string]any = map[string]any{
		"edges_where": map[string]any{
			"predicate": map[string]any{
				"eq": map[string]any{
					"left":  map[string]any{"property": "foundry_load"},
					"right": map[string]any{"constant": map[string]any{"string": loadID}},
				},
			},
		},
	}
	if limit > 0 {
		source = map[string]any{
			"limit": map[string]any{"input": source, "count": map[string]any{"literal": limit}},
		}
	}
	return json.Marshal(map[string]any{
		"request_type": "read",
		"query_name":   "graph_edges",
		"query": map[string]any{
			"read": map[string]any{
				"entries": []any{map[string]any{
					"query": map[string]any{
						"name": "edges",
						"root": map[string]any{"edge_properties": map[string]any{"input": source}},
					},
				}},
				"returns": []string{"edges"},
			},
		},
	})
}

func parseEdges(body []byte) ([]graphcompute.Edge, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(raw["edges"], &rows); err != nil {
		return nil, err
	}
	out := make([]graphcompute.Edge, 0, len(rows))
	for _, row := range rows {
		from, to := rawID(row["$from"]), rawID(row["$to"])
		if from == "" || to == "" {
			continue
		}
		out = append(out, graphcompute.Edge{From: from, To: to})
	}
	return out, nil
}

func wccScores(runID string, edges []graphcompute.Edge) []model.FoundryGraphComputeScore {
	components := graphcompute.Components(edges)
	sizes := make(map[int]int, len(components))
	for _, id := range components {
		sizes[id]++
	}
	scores := make([]model.FoundryGraphComputeScore, 0, len(components))
	for node, id := range components {
		scores = append(scores, model.FoundryGraphComputeScore{
			RunID:   runID,
			NodeKey: node,
			Score:   float64(sizes[id]),
		})
	}
	sortScores(scores)
	return scores
}

func pagerankScores(runID string, edges []graphcompute.Edge) []model.FoundryGraphComputeScore {
	ranks := graphcompute.PageRank(edges, 0.85, 20)
	scores := make([]model.FoundryGraphComputeScore, 0, len(ranks))
	for node, rank := range ranks {
		scores = append(scores, model.FoundryGraphComputeScore{
			RunID:   runID,
			NodeKey: node,
			Score:   rank,
		})
	}
	sortScores(scores)
	return scores
}

func sortScores(scores []model.FoundryGraphComputeScore) {
	sort.Slice(scores, func(i, j int) bool {
		if scores[i].Score != scores[j].Score {
			return scores[i].Score > scores[j].Score
		}
		return scores[i].NodeKey < scores[j].NodeKey
	})
	for i := range scores {
		scores[i].Rank = i + 1
	}
}

// degreeQuery reads the load's edges grouped by their two ends: one
// group_count for out-degree ($from) and one for in-degree ($to).
func degreeQuery(loadID string) ([]byte, error) {
	edges := map[string]any{
		"edges_where": map[string]any{
			"predicate": map[string]any{
				"eq": map[string]any{
					"left":  map[string]any{"property": "foundry_load"},
					"right": map[string]any{"constant": map[string]any{"string": loadID}},
				},
			},
		},
	}
	entry := func(name, property string) map[string]any {
		return map[string]any{
			"query": map[string]any{
				"name": name,
				"root": map[string]any{
					"group_count": map[string]any{"input": edges, "property": property},
				},
			},
		}
	}
	return json.Marshal(map[string]any{
		"request_type": "read",
		"query_name":   "graph_degree",
		"query": map[string]any{
			"read": map[string]any{
				"entries": []any{entry("out_degrees", "$from"), entry("in_degrees", "$to")},
				"returns": []string{"out_degrees", "in_degrees"},
			},
		},
	})
}

type degreeCounts struct {
	in  int
	out int
}

// parseDegree merges the two group counts into one score per node, ranked by
// total degree, and returns the number of edges (one $from per edge).
func parseDegree(runID string, body []byte) ([]model.FoundryGraphComputeScore, int64, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, 0, err
	}
	out, err := degreeRows(raw["out_degrees"], "$from")
	if err != nil {
		return nil, 0, err
	}
	in, err := degreeRows(raw["in_degrees"], "$to")
	if err != nil {
		return nil, 0, err
	}
	merged := make(map[string]*degreeCounts, len(out)+len(in))
	var edges int64
	for node, count := range out {
		merged[node] = &degreeCounts{out: count}
		edges += int64(count)
	}
	for node, count := range in {
		c := merged[node]
		if c == nil {
			c = &degreeCounts{}
			merged[node] = c
		}
		c.in = count
	}
	scores := make([]model.FoundryGraphComputeScore, 0, len(merged))
	for node, c := range merged {
		scores = append(scores, model.FoundryGraphComputeScore{
			RunID:   runID,
			NodeKey: node,
			Score:   float64(c.in + c.out),
		})
	}
	sort.Slice(scores, func(i, j int) bool {
		if scores[i].Score != scores[j].Score {
			return scores[i].Score > scores[j].Score
		}
		return scores[i].NodeKey < scores[j].NodeKey
	})
	for i := range scores {
		scores[i].Rank = i + 1
	}
	return scores, edges, nil
}

func degreeRows(raw json.RawMessage, key string) (map[string]int, error) {
	out := map[string]int{}
	if len(raw) == 0 {
		return out, nil
	}
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, err
	}
	for _, row := range rows {
		id, ok := row[key]
		if !ok {
			continue
		}
		var count int
		if err := json.Unmarshal(row["count"], &count); err != nil {
			continue
		}
		out[rawID(id)] += count
	}
	return out, nil
}

func rawID(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var n json.Number
	if json.Unmarshal(raw, &n) == nil {
		return n.String()
	}
	return string(raw)
}

func truncate(body []byte, max int) string {
	s := strings.TrimSpace(string(body))
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
