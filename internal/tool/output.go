package tool

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/store"
)

// Previews, refs and stubs (SPEC 3.7). A large result is stored in the
// bucket and enters the context as a preview: a header line with its size
// and ref, the first part of the text, and a line saying how to read on.
// At a cut the preview becomes a stub: the header without the size.
//
//	[page "Laptop X review" — example.com — 8,000 tokens, showing first 1,500 — ref: cache/pages/example.com/ab12f9.txt]
//	…first part of the text…
//	[use read_ref(ref, offset) or search_ref(ref, query) for more; the next offset is 6012]

// tokens estimates s's tokens as 4 bytes a token, the rule the provider
// layer uses.
func tokens(s string) int { return (len(s) + 3) / 4 }

// cut returns the start of s, at most max bytes. It ends after a line
// break, else after a space, when one is in the last fifth; else at a
// character boundary.
func cut(s string, max int) string {
	if len(s) <= max {
		return s
	}
	i := max
	for i > 0 && !utf8.RuneStart(s[i]) {
		i--
	}
	if j := strings.LastIndexByte(s[:i], '\n'); j >= 0 && j+1 >= i*4/5 {
		return s[:j+1]
	}
	if j := strings.LastIndexByte(s[:i], ' '); j >= 0 && j+1 >= i*4/5 {
		return s[:j+1]
	}
	return s[:i]
}

// count writes n with thousands separators, as 8,000.
func count(n int) string {
	s := strconv.Itoa(n)
	if n < 0 {
		return "-" + count(-n)
	}
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// preview is text as it enters the context: whole under a header if it
// fits in maxTokens, else its start with the header and the footer.
func preview(label, ref, text string, maxTokens int) string {
	shown := cut(text, maxTokens*4)
	if len(shown) == len(text) {
		return fmt.Sprintf("[%s — %s tokens — ref: %s]\n%s", label, count(tokens(text)), ref, text)
	}
	return fmt.Sprintf("[%s — %s tokens, showing first %s — ref: %s]\n%s\n%s",
		label, count(tokens(text)), count(tokens(shown)), ref, strings.TrimRight(shown, "\n"), more(len(shown)))
}

func more(next int) string {
	return fmt.Sprintf("[use read_ref(ref, offset) or search_ref(ref, query) for more; the next offset is %d]", next)
}

// Stub is a stored result as it stays in the context after a cut: the
// header line without the size. A result without a ref is small and stays
// as it is.
func Stub(r chat.ToolResult) string {
	if r.Ref == "" {
		return r.Text
	}
	line, _, _ := strings.Cut(r.Text, "\n")
	tail := " — ref: " + r.Ref + "]"
	head, ok := strings.CutSuffix(line, tail)
	if !ok || !strings.HasPrefix(head, "[") {
		return "[ref: " + r.Ref + "]"
	}
	if i := strings.LastIndex(head, " — "); i >= 0 && strings.Contains(head[i:], " tokens") {
		head = head[:i]
	}
	return head + tail
}

// Output turns a call's outcome into its tool_result part. An *Error is an
// error result for the model; any other error is returned, as it is a
// system failure. A result larger than env.PreviewTokens is stored under
// cache/tool/<chat>/<message>-<n> and enters as a preview with its ref; n
// is the call's place in the message.
func Output(ctx context.Context, call Call, name string, n int, r Result, err error) (chat.ToolResult, error) {
	out := chat.ToolResult{CallID: call.ID}
	if err != nil {
		var te *Error
		if !errors.As(err, &te) {
			return out, err
		}
		out.Text, out.IsError = te.Msg, true
		return out, nil
	}
	env := call.Env
	if r.Ref != "" || tokens(r.Text) <= env.PreviewTokens {
		out.Text, out.Ref = r.Text, r.Ref
		return out, nil
	}
	key := fmt.Sprintf("cache/tool/%s/%s-%d", env.Chat, env.Message, n)
	_, err = env.Project.DB.PutObject(ctx, env.Priority, env.Source, key, strings.NewReader(r.Text), store.PutOptions{Source: "tool"})
	if err != nil {
		return out, fmt.Errorf("tool: storing the output of %s: %w", name, err)
	}
	out.Text, out.Ref = preview("output of "+name, key, r.Text, env.PreviewTokens), key
	return out, nil
}
