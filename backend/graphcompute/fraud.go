package graphcompute

import (
	"sort"
	"time"
)

// Transfer is one money movement, for the fraud recipes.
type Transfer struct {
	From   string
	To     string
	Amount float64
	At     time.Time
}

// StructuringFinding is one source sending many transfers under a threshold
// in a window.
type StructuringFinding struct {
	Node  string
	Count int
	Total float64
	From  time.Time
	To    time.Time
}

// Structuring finds sources with at least minCount transfers under threshold
// inside one window. It reports at most one finding per source: the first
// window that qualifies, after sorting its transfers by time.
func Structuring(transfers []Transfer, threshold float64, window time.Duration, minCount int) []StructuringFinding {
	if threshold <= 0 || window <= 0 || minCount < 2 {
		return nil
	}
	bySource := make(map[string][]Transfer)
	for _, transfer := range transfers {
		if transfer.Amount < threshold {
			bySource[transfer.From] = append(bySource[transfer.From], transfer)
		}
	}
	var out []StructuringFinding
	for node, list := range bySource {
		sort.Slice(list, func(i, j int) bool { return list[i].At.Before(list[j].At) })
		for i := 0; i < len(list); i++ {
			total := 0.0
			j := i
			for j < len(list) && list[j].At.Sub(list[i].At) <= window {
				total += list[j].Amount
				j++
			}
			if j-i >= minCount {
				out = append(out, StructuringFinding{
					Node:  node,
					Count: j - i,
					Total: total,
					From:  list[i].At,
					To:    list[j-1].At,
				})
				break
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Total != out[j].Total {
			return out[i].Total > out[j].Total
		}
		return out[i].Node < out[j].Node
	})
	return out
}
