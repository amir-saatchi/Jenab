package main

import (
	"encoding/json"
	"os"
)

// dumpTexts writes the bench chunks and queries as JSON, so the ONNX Runtime
// stand-in (ortbench.py) embeds exactly the same text.
func dumpTexts(path string, pages int) error {
	b, err := json.Marshal(map[string][]string{"chunks": chunks(pages, 33), "queries": queries})
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}
