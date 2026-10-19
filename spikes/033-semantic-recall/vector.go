package main

import (
	"cmp"
	"math"
	"slices"
)

// A form is how vectors are kept (Method 5): the model's full width, cut
// to 256 and renormalized (Matryoshka models only), or int8.
type form string

const (
	formFull form = "full"
	form256  form = "256"
	formInt8 form = "int8"
)

const smallWidth = 256

func formsFor(m modelSpec, dims int) []form {
	fs := []form{formFull}
	if m.Matryoshka && dims > smallWidth {
		fs = append(fs, form256)
	}
	return append(fs, formInt8)
}

// bytesPerVector is the storage of one vector: float32s, or one byte a
// dimension plus a float32 scale.
func bytesPerVector(f form, dims int) int {
	switch f {
	case form256:
		return smallWidth * 4
	case formInt8:
		return dims + 4
	}
	return dims * 4
}

func normalized(v []float32) []float32 {
	var s float64
	for _, x := range v {
		s += float64(x) * float64(x)
	}
	out := make([]float32, len(v))
	if s == 0 {
		return out
	}
	n := float32(1 / math.Sqrt(s))
	for i, x := range v {
		out[i] = x * n
	}
	return out
}

// int8Vec is a vector quantized with one scale: x ≈ q × scale.
type int8Vec struct {
	q    []int8
	norm float32 // of q, for the cosine
}

func quantize(v []float32) int8Vec {
	var m float32
	for _, x := range v {
		m = max(m, float32(math.Abs(float64(x))))
	}
	q := make([]int8, len(v))
	if m > 0 {
		for i, x := range v {
			q[i] = int8(math.Round(float64(x / m * 127)))
		}
	}
	var s int64
	for _, x := range q {
		s += int64(x) * int64(x)
	}
	return int8Vec{q: q, norm: float32(math.Sqrt(float64(s)))}
}

// index holds one set of vectors in one form, for a brute-force scan.
type index struct {
	form form
	ids  []string
	f    [][]float32 // unit vectors, for full and 256
	q    []int8Vec
}

// prepare turns a model's raw vector into the form's: normalized, cut and
// normalized again, or normalized and quantized.
func prepare(f form, v []float32) ([]float32, int8Vec) {
	switch f {
	case form256:
		return normalized(normalized(v)[:min(smallWidth, len(v))]), int8Vec{}
	case formInt8:
		return nil, quantize(normalized(v))
	}
	return normalized(v), int8Vec{}
}

func newIndex(f form, ids []string, raw [][]float32) *index {
	ix := &index{form: f, ids: ids}
	for _, v := range raw {
		fv, qv := prepare(f, v)
		if f == formInt8 {
			ix.q = append(ix.q, qv)
		} else {
			ix.f = append(ix.f, fv)
		}
	}
	return ix
}

type scored struct {
	ID    string  `json:"id"`
	Score float64 `json:"score"`
}

// search returns the top n by cosine, best first.
func (ix *index) search(raw []float32, n int) []scored {
	fv, qv := prepare(ix.form, raw)
	out := make([]scored, len(ix.ids))
	for i, id := range ix.ids {
		var s float64
		if ix.form == formInt8 {
			var d int64
			for j, x := range ix.q[i].q {
				d += int64(x) * int64(qv.q[j])
			}
			if den := float64(ix.q[i].norm) * float64(qv.norm); den > 0 {
				s = float64(d) / den
			}
		} else {
			var d float32
			for j, x := range ix.f[i] {
				d += x * fv[j]
			}
			s = float64(d)
		}
		out[i] = scored{ID: id, Score: s}
	}
	slices.SortStableFunc(out, func(a, b scored) int { return cmp.Compare(b.Score, a.Score) })
	return out[:min(n, len(out))]
}

// rrfK is reciprocal rank fusion's constant (Method 4).
const rrfK = 60

// rrf merges ranked lists: score = Σ 1 / (60 + rank), ranks from 1. An item
// in one list keeps its one term. Ties go to the item with the better best
// rank, then to the earlier list.
func rrf(n int, lists ...[]scored) []scored {
	type acc struct {
		score float64
		best  int
		order int
	}
	m := map[string]*acc{}
	var order []string
	for _, l := range lists {
		for i, s := range l {
			a := m[s.ID]
			if a == nil {
				a = &acc{best: i + 1, order: len(order)}
				m[s.ID] = a
				order = append(order, s.ID)
			}
			a.score += 1 / float64(rrfK+i+1)
			a.best = min(a.best, i+1)
		}
	}
	out := make([]scored, len(order))
	for i, id := range order {
		out[i] = scored{ID: id, Score: m[id].score}
	}
	slices.SortStableFunc(out, func(a, b scored) int {
		if c := cmp.Compare(b.Score, a.Score); c != 0 {
			return c
		}
		if c := cmp.Compare(m[a.ID].best, m[b.ID].best); c != 0 {
			return c
		}
		return cmp.Compare(m[a.ID].order, m[b.ID].order)
	})
	return out[:min(n, len(out))]
}
