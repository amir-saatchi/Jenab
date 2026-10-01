package main

import (
	"math/rand"
	"strings"
)

// generator makes synthetic chat messages from three made-up vocabularies with
// Zipf-distributed word frequencies.
type generator struct {
	rng           *rand.Rand
	en, de, fa    []string
	zen, zde, zfa *rand.Zipf
}

func word(rng *rand.Rand, letters []rune, min, max int) string {
	n := min + rng.Intn(max-min+1)
	r := make([]rune, n)
	for i := range r {
		r[i] = letters[rng.Intn(len(letters))]
	}
	return string(r)
}

func newGenerator(rng *rand.Rand) *generator {
	enL := []rune("abcdefghijklmnopqrstuvwxyzeeeaaoitns")
	deL := []rune("abcdefghijklmnopqrstuvwxyzäöüßeeennrrstt")
	faL := []rune("ابپتثجچحخدذرزژسشصضطظعغفقکگلمنوهیی")
	g := &generator{rng: rng}
	for i := 0; i < 20000; i++ {
		g.en = append(g.en, word(rng, enL, 2, 10))
		w := word(rng, deL, 3, 11)
		if i > 500 && rng.Intn(5) == 0 { // compounds of two earlier words
			w = g.de[rng.Intn(500)] + g.de[rng.Intn(i)]
		}
		g.de = append(g.de, w)
		f := word(rng, faL, 2, 8)
		switch rng.Intn(8) {
		case 0:
			f = "می‌" + f
		case 1:
			f = f + "‌ها"
		}
		g.fa = append(g.fa, f)
	}
	g.zen = rand.NewZipf(rng, 1.1, 2, uint64(len(g.en)-1))
	g.zde = rand.NewZipf(rng, 1.1, 2, uint64(len(g.de)-1))
	g.zfa = rand.NewZipf(rng, 1.1, 2, uint64(len(g.fa)-1))
	return g
}

func (g *generator) message() string {
	var vocab []string
	var z *rand.Zipf
	switch p := g.rng.Intn(4); p {
	case 0, 1:
		vocab, z = g.en, g.zen
	case 2:
		vocab, z = g.de, g.zde
	default:
		vocab, z = g.fa, g.zfa
	}
	// lengths: mostly short chat turns, some long replies
	n := 5 + g.rng.Intn(30)
	if g.rng.Intn(4) == 0 {
		n = 40 + g.rng.Intn(260)
	}
	var b strings.Builder
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(vocab[z.Uint64()])
		if g.rng.Intn(12) == 0 {
			b.WriteString(".")
		}
	}
	if g.rng.Intn(20) == 0 {
		b.WriteString(" https://api." + g.en[z.Uint64()%2000] + ".example/v1/" + g.en[g.rng.Intn(2000)] + "?coin=btc")
	}
	if g.rng.Intn(20) == 0 {
		b.WriteString(" SELECT avg(price) FROM " + g.en[g.rng.Intn(2000)] + "_prices WHERE coin = 'btc'")
	}
	return b.String()
}

// queries: frequent, middle and rare words from each vocabulary, a few
// two-word queries, and two-letter Persian words (too short for trigram).
func (g *generator) queries(rng *rand.Rand, n int) []string {
	var qs []string
	pick := func(v []string, lo, hi int) string { return v[lo+rng.Intn(hi-lo)] }
	for len(qs) < n {
		for _, v := range [][]string{g.en, g.de, g.fa} {
			qs = append(qs, pick(v, 0, 20), pick(v, 100, 1000), pick(v, 5000, 20000))
		}
		qs = append(qs, pick(g.en, 0, 200)+" "+pick(g.en, 0, 200), pick(g.de, 0, 200)+" "+pick(g.de, 0, 200))
		for _, w := range g.fa {
			if len([]rune(w)) == 2 {
				qs = append(qs, w)
				break
			}
		}
	}
	return qs[:n]
}
