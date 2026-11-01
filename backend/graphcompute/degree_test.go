package graphcompute_test

import (
	"reflect"
	"testing"

	"github.com/helixdb/helix-foundry/backend/graphcompute"
)

func TestDegrees(t *testing.T) {
	got := graphcompute.Degrees([]graphcompute.Edge{
		{From: "a", To: "b"},
		{From: "a", To: "c"},
		{From: "b", To: "c"},
	})
	want := []graphcompute.Degree{
		{Node: "a", In: 0, Out: 2, Total: 2},
		{Node: "b", In: 1, Out: 1, Total: 2},
		{Node: "c", In: 2, Out: 0, Total: 2},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Degrees() = %#v, want %#v", got, want)
	}
}

func TestDegreesSelfLoop(t *testing.T) {
	got := graphcompute.Degrees([]graphcompute.Edge{{From: "a", To: "a"}})
	want := []graphcompute.Degree{{Node: "a", In: 1, Out: 1, Total: 2}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Degrees() = %#v, want %#v", got, want)
	}
}

func TestDegreesEmpty(t *testing.T) {
	if got := graphcompute.Degrees(nil); len(got) != 0 {
		t.Fatalf("Degrees(nil) = %#v, want empty", got)
	}
}
