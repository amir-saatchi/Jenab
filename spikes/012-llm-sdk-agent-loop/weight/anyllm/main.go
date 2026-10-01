// any-llm-go: one CompletionStream per provider.
package main

import (
	"context"
	"fmt"
	"os"

	anyllm "github.com/mozilla-ai/any-llm-go"
	"github.com/mozilla-ai/any-llm-go/providers"
	"github.com/mozilla-ai/any-llm-go/providers/anthropic"
	"github.com/mozilla-ai/any-llm-go/providers/openai"
)

func main() {
	ctx := context.Background()
	ap, _ := anthropic.New(anyllm.WithAPIKey("x"), anyllm.WithBaseURL(os.Args[1]))
	op, _ := openai.NewCompatible(openai.CompatibleConfig{Name: "local", DefaultBaseURL: os.Args[1] + "/v1", DefaultAPIKey: "x"})
	for _, p := range []providers.Provider{ap, op} {
		chunks, errs := p.CompletionStream(ctx, providers.CompletionParams{Model: "m", Messages: []providers.Message{{Role: "user", Content: "hi"}}})
		for c := range chunks {
			fmt.Print(c.Choices)
		}
		fmt.Println(<-errs)
	}
}
