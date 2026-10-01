// Eino: one Stream call per ChatModel component.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/cloudwego/eino-ext/components/model/claude"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

func main() {
	ctx := context.Background()
	base := os.Args[1]
	cm, _ := claude.NewChatModel(ctx, &claude.Config{APIKey: "x", BaseURL: &base, Model: "claude-sonnet-4-5", MaxTokens: 100})
	om, _ := openai.NewChatModel(ctx, &openai.ChatModelConfig{APIKey: "x", BaseURL: base + "/v1", Model: "gpt-4o"})
	for _, m := range []model.ToolCallingChatModel{cm, om} {
		sr, err := m.Stream(ctx, []*schema.Message{schema.UserMessage("hi")})
		if err != nil {
			fmt.Println(err)
			continue
		}
		for {
			c, err := sr.Recv()
			if err != nil {
				break
			}
			fmt.Print(c.Content)
		}
	}
}
