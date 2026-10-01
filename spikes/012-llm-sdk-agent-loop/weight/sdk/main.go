// Official SDKs: one streaming call per provider.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/anthropics/anthropic-sdk-go"
	aoption "github.com/anthropics/anthropic-sdk-go/option"
	"github.com/openai/openai-go/v3"
	ooption "github.com/openai/openai-go/v3/option"
)

func main() {
	ctx := context.Background()
	ac := anthropic.NewClient(aoption.WithBaseURL(os.Args[1]), aoption.WithAPIKey("x"))
	as := ac.Messages.NewStreaming(ctx, anthropic.MessageNewParams{Model: "claude-sonnet-4-5", MaxTokens: 100,
		Messages: []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock("hi"))}})
	var msg anthropic.Message
	for as.Next() {
		_ = msg.Accumulate(as.Current())
	}
	fmt.Println(as.Err())
	oc := openai.NewClient(ooption.WithBaseURL(os.Args[1]+"/v1"), ooption.WithAPIKey("x"))
	os_ := oc.Chat.Completions.NewStreaming(ctx, openai.ChatCompletionNewParams{Model: "gpt-4o",
		Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("hi")}})
	for os_.Next() {
		fmt.Print(os_.Current().Choices)
	}
	fmt.Println(os_.Err())
}
