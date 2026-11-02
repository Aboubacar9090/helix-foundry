package graphcompute_test

import (
	"testing"
	"time"

	"github.com/helixdb/helix-foundry/backend/graphcompute"
)

func TestStructuring(t *testing.T) {
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	got := graphcompute.Structuring([]graphcompute.Transfer{
		{From: "a", To: "m1", Amount: 9000, At: start},
		{From: "a", To: "m2", Amount: 9500, At: start.Add(10 * time.Minute)},
		{From: "a", To: "m3", Amount: 9800, At: start.Add(20 * time.Minute)},
		// At or above the threshold: not structuring.
		{From: "a", To: "big", Amount: 12000, At: start.Add(30 * time.Minute)},
		// Two transfers only: below minCount.
		{From: "b", To: "m1", Amount: 4000, At: start},
		{From: "b", To: "m2", Amount: 5000, At: start.Add(5 * time.Minute)},
	}, 10000, time.Hour, 3)
	want := []graphcompute.StructuringFinding{{
		Node:  "a",
		Count: 3,
		Total: 28300,
		From:  start,
		To:    start.Add(20 * time.Minute),
	}}
	if len(got) != 1 || got[0].Node != want[0].Node || got[0].Count != want[0].Count || got[0].Total != want[0].Total {
		t.Fatalf("Structuring = %#v, want %#v", got, want)
	}
}

func TestStructuringIgnoresShortParameters(t *testing.T) {
	if got := graphcompute.Structuring(nil, 0, time.Hour, 3); got != nil {
		t.Fatalf("Structuring with no threshold = %#v, want nil", got)
	}
}
