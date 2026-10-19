package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"

	"burrow/spikes/semanticrecall/builtin"
)

// reference runs the text-search example from the onnx-community model card
// and prints the scores next to the card's. The card ran the q4 model in
// Transformers.js, whose text vectors are at worst 0.988 cosine from fp32, so
// the scores should agree to about 0.02. A broken rotary embedding or a wrong
// norm would move them much further.
func reference(ctx context.Context, e *builtin.Embedder) {
	texts := []string{
		"task: search result | query: Which planet is known as the Red Planet?",
		"title: none | text: Venus is often called Earth's twin because of its similar size and proximity.",
		"title: none | text: Mars, known for its reddish appearance, is often referred to as the Red Planet.",
		"title: none | text: Jupiter, the largest planet in our solar system, has a prominent red spot.",
		"title: none | text: Saturn, famous for its rings, is sometimes mistaken for the Red Planet.",
	}
	card768 := []float64{0.684, 0.854, 0.752, 0.783}
	card256 := []float64{0.706, 0.871, 0.775, 0.807}
	fmt.Println("ids[0]:", e.TokenIDs(texts[0]))
	v := must(e.Embed(ctx, texts))
	if path := os.Getenv("DUMP_VECTORS"); path != "" {
		b, _ := json.Marshal(v)
		_ = os.WriteFile(path, b, 0o644)
	}
	worst := 0.0
	for _, w := range []struct {
		dims int
		card []float64
	}{{768, card768}, {256, card256}} {
		q := truncate(v[0], w.dims)
		for i, want := range w.card {
			got := cosine(q, truncate(v[i+1], w.dims))
			worst = math.Max(worst, math.Abs(got-want))
			fmt.Printf("  %d wide, %-8s card %.3f, here %.3f\n", w.dims, []string{"Venus", "Mars", "Jupiter", "Saturn"}[i], want, got)
		}
	}
	fmt.Printf("reference: largest gap to the model card %.3f\n", worst)
}

func truncate(v []float32, n int) []float32 {
	if n > len(v) {
		n = len(v)
	}
	return v[:n]
}
