package main

import (
	"encoding/json"
	"fmt"
	"os"

	"burrow/spikes/semanticrecall/builtin"
)

// dumpTokens writes the Go tokenizer's ids for the agreement texts, to
// compare with the reference tokenizer (ortcheck.py -tokens).
func dumpTokens(e *builtin.Embedder, path string) {
	texts := agreementTexts()
	ids := make([][]int, len(texts))
	for i, t := range texts {
		ids[i] = e.TokenIDs(t)
	}
	b := must(json.Marshal(map[string]any{"texts": texts, "ids": ids}))
	if err := os.WriteFile(path, b, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("wrote token ids for %d texts to %s\n", len(texts), path)
}
