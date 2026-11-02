package graphcompute

import (
	"sort"
	"strings"
)

// Components assigns each node a weakly connected component id. The ids
// follow the smallest node key in each component, so two runs over the same
// edges agree.
func Components(edges []Edge) map[string]int {
	parent := make(map[string]string)
	var find func(string) string
	find = func(node string) string {
		p, ok := parent[node]
		if !ok {
			parent[node] = node
			return node
		}
		if p != node {
			parent[node] = find(p)
		}
		return parent[node]
	}
	for _, e := range edges {
		find(e.From)
		find(e.To)
		ra, rb := find(e.From), find(e.To)
		if ra == rb {
			continue
		}
		if ra < rb {
			parent[rb] = ra
		} else {
			parent[ra] = rb
		}
	}
	nodes := make([]string, 0, len(parent))
	for node := range parent {
		nodes = append(nodes, node)
	}
	sort.Strings(nodes)
	ids := make(map[string]int)
	out := make(map[string]int, len(nodes))
	for _, node := range nodes {
		root := find(node)
		id, ok := ids[root]
		if !ok {
			id = len(ids)
			ids[root] = id
		}
		out[node] = id
	}
	return out
}

// PageRank scores each node with damping and iterations (0 uses 0.85 and 20).
// Dangling nodes redistribute their rank evenly, so the scores sum to one.
func PageRank(edges []Edge, damping float64, iterations int) map[string]float64 {
	if damping <= 0 || damping >= 1 {
		damping = 0.85
	}
	if iterations <= 0 {
		iterations = 20
	}
	nodes := make(map[string]bool)
	out := make(map[string][]string)
	for _, e := range edges {
		nodes[e.From] = true
		nodes[e.To] = true
		out[e.From] = append(out[e.From], e.To)
	}
	n := len(nodes)
	rank := make(map[string]float64, n)
	if n == 0 {
		return rank
	}
	for node := range nodes {
		rank[node] = 1 / float64(n)
	}
	for i := 0; i < iterations; i++ {
		dangling := 0.0
		for node := range nodes {
			if len(out[node]) == 0 {
				dangling += rank[node]
			}
		}
		base := (1-damping)/float64(n) + damping*dangling/float64(n)
		next := make(map[string]float64, n)
		for node := range nodes {
			next[node] = base
		}
		for node := range nodes {
			if len(out[node]) == 0 {
				continue
			}
			share := damping * rank[node] / float64(len(out[node]))
			for _, to := range out[node] {
				next[to] += share
			}
		}
		rank = next
	}
	return rank
}

// ShortestPath returns the shortest path from to, walking edges in both
// directions, or nil when none exists.
func ShortestPath(edges []Edge, from, to string) []string {
	if from == to {
		return []string{from}
	}
	adj := make(map[string][]string)
	for _, e := range edges {
		adj[e.From] = append(adj[e.From], e.To)
		adj[e.To] = append(adj[e.To], e.From)
	}
	prev := map[string]string{from: ""}
	queue := []string{from}
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		if node == to {
			break
		}
		for _, next := range adj[node] {
			if _, seen := prev[next]; seen {
				continue
			}
			prev[next] = node
			queue = append(queue, next)
		}
	}
	if _, ok := prev[to]; !ok {
		return nil
	}
	path := []string{}
	for node := to; node != ""; node = prev[node] {
		path = append([]string{node}, path...)
	}
	return path
}

// Collusion finds directed triangles (a→b→c→a), each once, sorted.
func Collusion(edges []Edge) [][]string {
	adj := make(map[string]map[string]bool)
	for _, e := range edges {
		if adj[e.From] == nil {
			adj[e.From] = map[string]bool{}
		}
		adj[e.From][e.To] = true
	}
	seen := map[string]bool{}
	var out [][]string
	for a, neighbors := range adj {
		for b := range neighbors {
			for c := range adj[b] {
				if !adj[c][a] {
					continue
				}
				triangle := []string{a, b, c}
				sort.Strings(triangle)
				key := strings.Join(triangle, "\x00")
				if seen[key] {
					continue
				}
				seen[key] = true
				out = append(out, triangle)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.Join(out[i], "\x00") < strings.Join(out[j], "\x00")
	})
	return out
}
