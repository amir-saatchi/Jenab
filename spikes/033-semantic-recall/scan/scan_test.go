package main

import (
	"math/rand"
	"sort"
	"testing"
)

func TestScanMatchesBruteForce(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	const n, w = 5000, 64
	s := &store{n: n, w: w, f32: make([]float32, n*w), i8: make([]int8, n*w), scales: make([]float32, n)}
	for i := 0; i < n; i++ {
		v := unit(r, w)
		copy(s.f32[i*w:], v)
		s.scales[i] = quantize(v, s.i8[i*w:(i+1)*w])
	}
	q := unit(r, w)
	all := make([]hit, n)
	for i := range all {
		all[i] = hit{i, dotF32(q, s.f32[i*w:(i+1)*w])}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].score > all[j].score })
	for _, workers := range []int{1, 7} {
		got := run(s.f32Scanner(q), n, workers)
		for i := range got {
			if got[i].id != all[i].id {
				t.Fatalf("f32 workers=%d rank %d: got %d want %d", workers, i, got[i].id, all[i].id)
			}
		}
		// int8 is lossy: expect most of the float top 50 back.
		in := map[int]bool{}
		for _, h := range all[:topK] {
			in[h.id] = true
		}
		hits := 0
		for _, h := range run(s.i8Scanner(q), n, workers) {
			if in[h.id] {
				hits++
			}
		}
		if hits < 40 {
			t.Fatalf("int8 workers=%d recovered only %d/50", workers, hits)
		}
	}
}
