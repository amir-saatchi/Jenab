// Genkit: one streaming Generate per provider plugin.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/anthropic"
	"github.com/firebase/genkit/go/plugins/compat_oai"
)

func main() {
	ctx := context.Background()
	g := genkit.Init(ctx, genkit.WithPlugins(&anthropic.Anthropic{APIKey: "x", BaseURL: os.Args[1]},
		&compat_oai.OpenAICompatible{Provider: "local", APIKey: "x", BaseURL: os.Args[1] + "/v1"}))
	for _, m := range []string{"anthropic/claude-sonnet-4-5", "local/gpt-4o"} {
		r, err := genkit.Generate(ctx, g, ai.WithModelName(m), ai.WithPrompt("hi"), ai.WithReturnToolRequests(true),
			ai.WithStreaming(func(ctx context.Context, c *ai.ModelResponseChunk) error { fmt.Print(c.Text()); return nil }))
		fmt.Println(r, err)
	}
}
