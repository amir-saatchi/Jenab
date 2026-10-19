package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"burrow/spikes/semanticrecall/builtin"
)

// compareOllama embeds the same texts with hugot and with Ollama's native
// /api/embed, and prints the cosine between the two vectors of each text.
// Close to 1 means the hugot path computes the same model.
func compareOllama(ctx context.Context, e *builtin.Embedder, model string) {
	texts := append([]string{}, queries...)
	for _, lang := range []string{"en", "de", "fa"} {
		texts = append(texts, sentences[lang][:4]...)
	}
	texts = append(texts, chunks(3, 7)...)

	hv := must(e.Embed(ctx, texts))
	ov := must(ollamaEmbed(ctx, model, texts))
	if len(ov) != len(hv) {
		fmt.Fprintf(os.Stderr, "ollama returned %d vectors for %d texts\n", len(ov), len(texts))
		os.Exit(1)
	}
	if len(ov[0]) != len(hv[0]) {
		fmt.Printf("widths differ: hugot %d, ollama %d\n", len(hv[0]), len(ov[0]))
		return
	}
	var cos []float64
	for i := range texts {
		c := cosine(hv[i], ov[i])
		cos = append(cos, c)
		fmt.Printf("  %.5f  %s\n", c, short(texts[i], 60))
	}
	fmt.Printf("hugot vs ollama cosine over %d texts: %s\n", len(texts), summarize(cos))
}

func ollamaEmbed(ctx context.Context, model string, texts []string) ([][]float32, error) {
	body, _ := json.Marshal(map[string]any{"model": model, "input": texts})
	req, err := http.NewRequestWithContext(ctx, "POST", "http://localhost:11434/api/embed", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		var b bytes.Buffer
		b.ReadFrom(resp.Body)
		return nil, fmt.Errorf("ollama: %s: %s", resp.Status, b.String())
	}
	var out struct {
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Embeddings, nil
}
