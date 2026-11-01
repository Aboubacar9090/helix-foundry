// Package graphcompute holds pure graph algorithms used by Foundry's compute
// jobs. It has no store, agent or transport dependency: a job feeds it the
// edges of one load and persists the result.
package graphcompute

import "sort"

// Edge is one directed edge between two node keys.
type Edge struct {
	From string
	To   string
}

// Degree is one node's in, out and total degree.
type Degree struct {
	Node  string
	In    int
	Out   int
	Total int
}

// Degrees counts every node's in and out edges and returns the busiest first,
// then by node key. A self-loop counts once in each direction.
func Degrees(edges []Edge) []Degree {
	counts := make(map[string]*Degree)
	at := func(node string) *Degree {
		d := counts[node]
		if d == nil {
			d = &Degree{Node: node}
			counts[node] = d
		}
		return d
	}
	for _, e := range edges {
		at(e.From).Out++
		at(e.To).In++
	}
	out := make([]Degree, 0, len(counts))
	for _, d := range counts {
		d.Total = d.In + d.Out
		out = append(out, *d)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Total != out[j].Total {
			return out[i].Total > out[j].Total
		}
		return out[i].Node < out[j].Node
	})
	return out
}
