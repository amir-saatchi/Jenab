// GoAI: one StreamText per provider.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/zendev-sh/goai"
	"github.com/zendev-sh/goai/provider"
	"github.com/zendev-sh/goai/provider/anthropic"
	"github.com/zendev-sh/goai/provider/compat"
)

func main() {
	ctx := context.Background()
	for _, m := range []provider.LanguageModel{
		anthropic.Chat("claude-sonnet-4-5", anthropic.WithAPIKey("x"), anthropic.WithBaseURL(os.Args[1])),
		compat.Chat("gpt-4o", compat.WithAPIKey("x"), compat.WithBaseURL(os.Args[1]+"/v1")),
	} {
		ts, err := goai.StreamText(ctx, m, goai.WithPrompt("hi"))
		if err != nil {
			fmt.Println(err)
			continue
		}
		for c := range ts.Stream() {
			fmt.Print(c.Text)
		}
		fmt.Println(ts.Err())
	}
}
