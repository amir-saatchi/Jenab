package main

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"regexp"
	"strings"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/provider/fake"
)

// dryModel is a rule-based stand-in for a model, for -dry: it calls the
// spike's tools the way a model might, so every path of the harness runs
// without a key or any quota. Its answers mean nothing.
type dryModel struct{}

func (dryModel) Models(context.Context) ([]provider.ModelInfo, error) { return nil, nil }

func (dryModel) Stream(ctx context.Context, req provider.Request) iter.Seq2[provider.Event, error] {
	return fake.New(dryReply(req)).Stream(ctx, req)
}

var reRevision = regexp.MustCompile(`expected_revision (\d+)`)
var reTopic = regexp.MustCompile(`(?m)^(t\d+) · ([^·]+) · `)
var reSubject = regexp.MustCompile(`(?m)^(s\d+) · ([^·]+) · `)

func dryReply(req provider.Request) fake.Reply {
	var sys strings.Builder
	for _, b := range req.System {
		sys.WriteString(b.Text + "\n")
	}
	usage := chat.Usage{Input: provider.EstimateTokens(req), Output: 20}
	has := map[string]bool{}
	for _, t := range req.Tools {
		has[t.Name] = true
	}
	ms := req.Messages
	// A notice the app adds mid-turn (memory changed) isn't the user's.
	last := ms[len(ms)-1]
	if len(ms) > 1 && last.Role == chat.RoleUser && !hasText(last) {
		last = ms[len(ms)-2]
	}
	var user string
	for i := len(ms) - 1; i >= 0; i-- {
		if ms[i].Role == chat.RoleUser {
			for _, p := range ms[i].Parts {
				if p.Text != nil {
					user = p.Text.Text
				}
			}
			if user != "" {
				break
			}
		}
	}
	text := func(s string) fake.Reply {
		return fake.Reply{Events: []provider.Event{
			{Kind: provider.EventPart, Part: &chat.Part{Kind: chat.PartText, Text: &chat.Text{Text: s}}},
			fake.Done(provider.StopEnd, usage),
		}}
	}
	calls := func(cs ...[2]any) fake.Reply {
		var evs []provider.Event
		for i, c := range cs {
			b, _ := json.Marshal(c[1])
			evs = append(evs, provider.Event{Kind: provider.EventPart, Part: &chat.Part{Kind: chat.PartToolCall,
				ToolCall: &chat.ToolCall{ID: fmt.Sprintf("dry%d_%d", len(ms), i), Name: c[0].(string), Args: b}}})
		}
		return fake.Reply{Events: append(evs, fake.Done(provider.StopToolUse, usage))}
	}
	if req.ToolChoice != "" {
		return text("Done (dry run).")
	}
	// After tool results: retry a memory conflict, else answer.
	if hasResult(last) {
		for _, p := range last.Parts {
			if r := p.ToolResult; r != nil && r.IsError && strings.Contains(r.Text, "changed since") {
				rev := 0
				if m := reRevision.FindStringSubmatch(r.Text); m != nil {
					fmt.Sscan(m[1], &rev)
				}
				return calls([2]any{"update_memory", map[string]any{"scope": "project", "section": "conventions", "content": "merged: " + clip(user, 80), "expected_revision": rev}})
			}
			if r := p.ToolResult; r != nil && strings.Contains(r.Text, "The user accepted") {
				return text("Moving this to the topic.")
			}
		}
		return text("Done (dry run): " + clip(user, 60))
	}
	lower := strings.ToLower(user)
	topicsIdx := reTopic.FindAllStringSubmatch(sys.String(), -1)
	inMain := strings.Contains(sys.String(), "Topics of this chat")
	switch {
	case strings.Contains(lower, "one thing"):
		if has["merge_topics"] && len(topicsIdx) >= 2 {
			return calls([2]any{"merge_topics", map[string]any{"ids": []string{topicsIdx[len(topicsIdx)-2][1], topicsIdx[len(topicsIdx)-1][1]}, "title": "Merged"}})
		}
		if subs := reSubject.FindAllStringSubmatch(sys.String(), -1); has["merge_subjects"] && len(subs) >= 2 {
			return calls([2]any{"merge_subjects", map[string]any{"ids": []string{subs[0][1], subs[1][1]}, "subject": "Merged"}})
		}
		return text("Noted.")
	case strings.HasSuffix(strings.TrimSpace(lower), "?"):
		return calls([2]any{"search_history", map[string]any{"query": firstWord(lower), "scope": "project"}})
	}
	if has["propose_topic"] && inMain {
		for _, t := range topicsIdx {
			if strings.Contains(lower, strings.ToLower(strings.Fields(t[2])[0])) {
				return calls([2]any{"propose_topic", map[string]any{"topic_id": t[1]}})
			}
		}
		return calls([2]any{"propose_topic", map[string]any{"title": strings.Title(firstWord(lower)) + " work"}})
	}
	var cs [][2]any
	if strings.Contains(lower, "remember") {
		cs = append(cs, [2]any{"update_memory", map[string]any{"scope": "project", "section": "conventions", "content": clip(user, 120), "expected_revision": 0}})
	} else {
		cs = append(cs, [2]any{"create_table", map[string]any{"name": firstWord(lower) + fmt.Sprint(len(ms)), "columns": []map[string]string{{"name": "id", "type": "INTEGER"}}, "primary_key": []string{"id"}}})
	}
	open := []string{}
	if strings.Contains(lower, "wait") {
		open = []string{"the pipeline waits on the user"}
	}
	switch {
	case has["update_notes"]:
		cs = append(cs, [2]any{"update_notes", map[string]any{"note": clip(user, 60), "open": open}})
	case has["update_subject"]:
		args := map[string]any{"subject": strings.Title(firstWord(lower)), "note": clip(user, 60), "open": open}
		if subs := reSubject.FindAllStringSubmatch(sys.String(), -1); len(subs) > 0 && strings.Contains(lower, strings.ToLower(strings.TrimSpace(subs[0][2]))) {
			args["id"] = subs[0][1]
		}
		cs = append(cs, [2]any{"update_subject", args})
	}
	return calls(cs...)
}

func hasText(m chat.Message) bool {
	for _, p := range m.Parts {
		if p.Text != nil {
			return true
		}
	}
	return false
}

func hasResult(m chat.Message) bool {
	for _, p := range m.Parts {
		if p.ToolResult != nil {
			return true
		}
	}
	return false
}

// firstWord is the message's first longer word, as the dry model's name
// for things.
func firstWord(s string) string {
	for _, w := range strings.FieldsFunc(s, func(r rune) bool { return !(r >= 'a' && r <= 'z') }) {
		if len(w) > 4 {
			return w
		}
	}
	return "thing"
}
