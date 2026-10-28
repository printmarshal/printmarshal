package discovery

import (
	"context"
	"sync"
)

// Prober reports whether an address looks like a manageable device (for
// example, answers an SNMP system-group read). It must be cheap and read-only.
type Prober func(ctx context.Context, address string) bool

// ActiveScan probes every address with a bounded worker pool (spec §8.1) and
// returns the responders in input order.
func ActiveScan(ctx context.Context, addrs []string, workers int, probe Prober) []Candidate {
	if workers <= 0 {
		workers = 32
	}
	hits := make([]bool, len(addrs))
	var wg sync.WaitGroup
	sem := make(chan struct{}, workers)
	for i, a := range addrs {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, a string) {
			defer wg.Done()
			defer func() { <-sem }()
			hits[i] = probe(ctx, a)
		}(i, a)
	}
	wg.Wait()
	var out []Candidate
	for i, ok := range hits {
		if ok {
			out = append(out, Candidate{Address: addrs[i], Methods: []string{"scan"}})
		}
	}
	return out
}

// MergeCandidates folds candidate lists together by address, preserving order of first sight.
func MergeCandidates(lists ...[]Candidate) []Candidate {
	byIP := map[string]*Candidate{}
	var order []string
	for _, l := range lists {
		for _, c := range l {
			cur, ok := byIP[c.Address]
			if !ok {
				cc := c
				byIP[c.Address] = &cc
				order = append(order, c.Address)
				continue
			}
			cur.Merge(c)
		}
	}
	out := make([]Candidate, 0, len(order))
	for _, ip := range order {
		out = append(out, *byIP[ip])
	}
	return out
}
