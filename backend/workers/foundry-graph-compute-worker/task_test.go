package foundrygraphcomputeworker

import (
	"strings"
	"testing"

	"github.com/helixdb/helix-foundry/backend/store/model"
)

func TestDegreeQuery(t *testing.T) {
	query, err := degreeQuery("load-1")
	if err != nil {
		t.Fatalf("degreeQuery: %v", err)
	}
	text := string(query)
	for _, want := range []string{"graph_degree", "out_degrees", "in_degrees", "$from", "$to", "foundry_load", "load-1"} {
		if !strings.Contains(text, want) {
			t.Fatalf("degreeQuery does not contain %q: %s", want, text)
		}
	}
}

func TestEncodeScores(t *testing.T) {
	data, err := encodeScores([]model.FoundryGraphComputeScore{
		{NodeKey: "a", Score: 2, Rank: 1},
		{NodeKey: "b", Score: 1, Rank: 2},
	})
	if err != nil {
		t.Fatalf("encodeScores: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("encodeScores wrote %d lines, want 2: %s", len(lines), data)
	}
	for _, want := range []string{`"node_key":"a"`, `"score":2`, `"rank":1`} {
		if !strings.Contains(lines[0], want) {
			t.Fatalf("first line %q does not contain %q", lines[0], want)
		}
	}
}

func TestParseDegree(t *testing.T) {
	body := []byte(`{
		"out_degrees": [{"$from": 1, "count": 2}, {"$from": 2, "count": 1}],
		"in_degrees": [{"$to": 1, "count": 1}, {"$to": 3, "count": 1}]
	}`)
	scores, edges, err := parseDegree("run-1", body)
	if err != nil {
		t.Fatalf("parseDegree: %v", err)
	}
	if edges != 3 {
		t.Fatalf("edges = %d, want 3", edges)
	}
	want := []struct {
		node  string
		score float64
		rank  int
	}{
		{"1", 3, 1},
		{"2", 1, 2},
		{"3", 1, 3},
	}
	if len(scores) != len(want) {
		t.Fatalf("scores = %#v, want %d rows", scores, len(want))
	}
	for i, w := range want {
		if scores[i].NodeKey != w.node || scores[i].Score != w.score || scores[i].Rank != w.rank {
			t.Fatalf("score %d = %#v, want %#v", i, scores[i], w)
		}
	}
}
