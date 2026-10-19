package main

import (
	"sort"
	"sync"
)

const topK = 50

type hit struct {
	id    int
	score float32
}

// top keeps the k best hits in a min-heap on score.
type top struct{ h []hit }

func (t *top) add(id int, s float32) {
	if len(t.h) < topK {
		t.h = append(t.h, hit{id, s})
		for i := len(t.h) - 1; i > 0; { // sift up
			p := (i - 1) / 2
			if t.h[p].score <= t.h[i].score {
				break
			}
			t.h[p], t.h[i] = t.h[i], t.h[p]
			i = p
		}
		return
	}
	if s <= t.h[0].score {
		return
	}
	t.h[0] = hit{id, s}
	for i := 0; ; { // sift down
		l, r, m := 2*i+1, 2*i+2, i
		if l < len(t.h) && t.h[l].score < t.h[m].score {
			m = l
		}
		if r < len(t.h) && t.h[r].score < t.h[m].score {
			m = r
		}
		if m == i {
			return
		}
		t.h[m], t.h[i] = t.h[i], t.h[m]
		i = m
	}
}

// store is n vectors of width w, in one flat slice per form.
type store struct {
	n, w   int
	f32    []float32
	i8     []int8
	scales []float32 // int8 only
}

// scanRange scores rows [lo,hi) against the query. Scan is the form's own:
// float32 dot for f32; int8 dot with a quantized query, rescaled, for i8.
type scanner func(lo, hi int, t *top)

func (s *store) f32Scanner(q []float32) scanner {
	return func(lo, hi int, t *top) {
		for i := lo; i < hi; i++ {
			t.add(i, dotF32(q, s.f32[i*s.w:(i+1)*s.w]))
		}
	}
}

func (s *store) i8Scanner(q []float32) scanner {
	qq := make([]int8, len(q))
	qs := quantize(q, qq)
	return func(lo, hi int, t *top) {
		for i := lo; i < hi; i++ {
			d := dotI8(qq, s.i8[i*s.w:(i+1)*s.w])
			t.add(i, float32(d)*qs*s.scales[i])
		}
	}
}

// run scans n rows with `workers` goroutines and merges to the global top K.
func run(sc scanner, n, workers int) []hit {
	parts := make([]top, workers)
	var wg sync.WaitGroup
	for k := 0; k < workers; k++ {
		wg.Add(1)
		go func(k int) {
			defer wg.Done()
			sc(n*k/workers, n*(k+1)/workers, &parts[k])
		}(k)
	}
	wg.Wait()
	var all top
	for _, p := range parts {
		for _, h := range p.h {
			all.add(h.id, h.score)
		}
	}
	sort.Slice(all.h, func(i, j int) bool { return all.h[i].score > all.h[j].score })
	return all.h
}
