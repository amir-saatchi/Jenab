package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"burrow/spikes/semanticrecall/builtin"
)

// agreementTexts are short and long texts in the three languages, for
// checking hugot's vectors against ONNX Runtime's (ortcheck.py).
func agreementTexts() []string {
	texts := append([]string{}, queries...)
	for _, lang := range []string{"en", "de", "fa"} {
		texts = append(texts, sentences[lang][:5]...)
	}
	texts = append(texts, faQueries[:5]...)
	return append(texts, chunks(3, 7)...)
}

// dumpVectors writes the texts and hugot's vectors for them as JSON.
func dumpVectors(ctx context.Context, e *builtin.Embedder, path string) {
	texts := agreementTexts()
	v := must(e.Embed(ctx, texts))
	b := must(json.Marshal(map[string]any{"texts": texts, "vectors": v}))
	if err := os.WriteFile(path, b, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("wrote %d vectors to %s\n", len(v), path)
}
