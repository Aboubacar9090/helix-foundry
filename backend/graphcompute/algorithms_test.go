package graphcompute_test

import (
	"math"
	"reflect"
	"testing"

	"github.com/helixdb/helix-foundry/backend/graphcompute"
)

func TestComponents(t *testing.T) {
	got := graphcompute.Components([]graphcompute.Edge{
		{From: "a", To: "b"},
		{From: "b", To: "c"},
		{From: "d", To: "e"},
	})
	if got["a"] != got["b"] || got["b"] != got["c"] {
		t.Fatalf("a, b and c should share a component: %#v", got)
	}
	if got["d"] != got["e"] {
		t.Fatalf("d and e should share a component: %#v", got)
	}
	if got["a"] == got["d"] {
		t.Fatalf("the two components should differ: %#v", got)
	}
}

func TestPageRank(t *testing.T) {
	ranks := graphcompute.PageRank([]graphcompute.Edge{
		{From: "a", To: "b"},
		{From: "b", To: "c"},
		{From: "c", To: "a"},
	}, 0.85, 50)
	if len(ranks) != 3 {
		t.Fatalf("ranks = %#v, want 3 nodes", ranks)
	}
	sum := 0.0
	for _, rank := range ranks {
		sum += rank
		if math.Abs(rank-1.0/3.0) > 1e-6 {
			t.Fatalf("rank = %v, want about 1/3", rank)
		}
	}
	if math.Abs(sum-1) > 1e-6 {
		t.Fatalf("rank sum = %v, want 1", sum)
	}
}

func TestShortestPath(t *testing.T) {
	edges := []graphcompute.Edge{
		{From: "a", To: "b"},
		{From: "b", To: "c"},
		{From: "a", To: "d"},
		{From: "d", To: "c"},
	}
	got := graphcompute.ShortestPath(edges, "a", "c")
	if len(got) != 3 || got[0] != "a" || got[len(got)-1] != "c" {
		t.Fatalf("ShortestPath = %#v, want a 3-node path from a to c", got)
	}
	if graphcompute.ShortestPath(edges, "a", "z") != nil {
		t.Fatal("ShortestPath to an unknown node should be nil")
	}
}

func TestCollusion(t *testing.T) {
	got := graphcompute.Collusion([]graphcompute.Edge{
		{From: "a", To: "b"},
		{From: "b", To: "c"},
		{From: "c", To: "a"},
		{From: "a", To: "d"},
	})
	want := [][]string{{"a", "b", "c"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Collusion = %#v, want %#v", got, want)
	}
}
