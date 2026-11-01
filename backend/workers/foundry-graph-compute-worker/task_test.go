package foundrygraphcomputeworker

import (
	"strings"
	"testing"
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
