package scenario

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/provider/fake"
)

// writeSet writes a set from file names and contents.
func writeSet(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

var testSet = map[string]string{
	"_set.yaml": `title: Prices
card: card.md
fixture: fixture.sql
skills: skills
tools:
  - name: query
    description: Read-only SQL on the project database.
    schema: {type: object, properties: {sql: {type: string}}, required: [sql]}
    kind: sql
    arg: sql
    delay: 1s
  - name: web_search
    description: Search the web.
    schema: {type: object, properties: {query: {type: string}}, required: [query]}
rules:
  web_search:
    - match: '(?i)gold'
      result: gold is 2641 USD
    - result: nothing found
`,
	"card.md":     "## Project card\nTables: prices (day, close).\n",
	"fixture.sql": "CREATE TABLE prices (day TEXT PRIMARY KEY, close REAL);\nINSERT INTO prices VALUES ('2026-09-27', 100.5), ('2026-09-28', 123.456);\n",
	"skills/sql-queries/SKILL.md": `---
name: sql-queries
description: SQL for the query tool.
---
Only one SELECT.
`,
	"close.yaml": `id: close
title: Last close
tests: ["sql"]
context: main
why: Load the SQL skill, query, give the close.
messages:
  - text: What was the last close?
asserts:
  - {type: text_before_tool, test: sql, tool: query}
  - {type: tool_called, test: sql, tool: query, args_match: '(?i)select'}
  - {type: skill_loaded_before, test: sql, skills: [sql-queries], tools: [query]}
  - {type: answer_contains, test: sql, match: '123\.46'}
  - {type: tool_not_called, test: sql, tool: web_search}
  - {type: max_turns, test: info, n: 1}
  - {type: max_prompt_tokens, test: info, n: 5}
`,
	"later.yaml": `id: later
title: Not yet
tests: ["bg"]
context: main
needs: [background]
messages:
  - text: Start the run.
asserts:
  - {type: background_started, test: bg, tools: [run]}
`,
}

func reply(text string, call *chat.ToolCall) fake.Reply {
	var ev []provider.Event
	stop := provider.StopEnd
	if text != "" {
		ev = append(ev, provider.Event{Kind: provider.EventPart, Part: &chat.Part{Kind: chat.PartText, Text: &chat.Text{Text: text}}})
	}
	if call != nil {
		ev = append(ev, provider.Event{Kind: provider.EventPart, Part: &chat.Part{Kind: chat.PartToolCall, ToolCall: call}})
		stop = provider.StopToolUse
	}
	return fake.Reply{Events: append(ev, fake.Done(stop, chat.Usage{Input: 100, CacheRead: 20, Output: 7}))}
}

func TestRunSet(t *testing.T) {
	s, err := Load(writeSet(t, testSet))
	if err != nil {
		t.Fatal(err)
	}
	fp := fake.New(
		reply("Let me look at the prices table.", &chat.ToolCall{ID: "c1", Name: "load_skill", Args: []byte(`{"name":"sql-queries"}`)}),
		reply("", &chat.ToolCall{ID: "c2", Name: "query", Args: []byte(`{"sql":"SELECT close FROM prices ORDER BY day DESC LIMIT 1"}`)}),
		reply("The last close was 123.46.", nil),
	)
	const key = "test-key-0123456789"
	models := Static([]ProviderSpec{{Name: "p", Kind: "openai_compatible", BaseURL: "https://example.test/v1/", Models: []config.ModelSettings{{ID: "m", Context: 32000}}}},
		map[string]string{"p": key}, "p/m")
	var progress []string
	rep, err := RunSet(context.Background(), s, Options{
		Models: models, Backends: map[provider.Kind]provider.Factory{provider.KindCompatible: fp.Factory()},
		Scale: 0.01, Timeout: time.Minute, Temp: t.TempDir(),
		Progress: func(r Run) { progress = append(progress, r.Scenario) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Runs) != 2 || len(progress) != 2 {
		t.Fatalf("runs %d, progress %v", len(rep.Runs), progress)
	}
	byID := map[string]Run{}
	for _, r := range rep.Runs {
		byID[r.Scenario] = r
	}
	r := byID["close"]
	if r.Error != "" {
		t.Fatalf("error: %s", r.Error)
	}
	for _, a := range r.Asserts {
		want := Pass
		if a.Type == "max_prompt_tokens" {
			want = Fail
		}
		if a.Status != want {
			t.Errorf("%s: %s (%s), want %s", a.Type, a.Status, a.Why, want)
		}
	}
	if !r.Passed() || r.Turns != 1 || r.Requests != 3 || r.Prompt != 360 || r.PeakPrompt != 120 || r.Output != 21 {
		t.Errorf("run: %+v", r)
	}
	if !strings.Contains(r.Transcript, "123.46") || !strings.Contains(r.Transcript, "sql-queries") {
		t.Errorf("transcript:\n%s", r.Transcript)
	}
	if byID["later"].Skipped != "needs background" {
		t.Errorf("later: %+v", byID["later"])
	}

	// The model saw the card, the skill list and the fake tools, and the
	// query ran on the fixture.
	calls := fp.Calls()
	if len(calls) != 3 {
		t.Fatalf("%d calls", len(calls))
	}
	var sys strings.Builder
	for _, b := range calls[0].System {
		sys.WriteString(b.Text)
	}
	if !strings.Contains(sys.String(), "Tables: prices") || !strings.Contains(sys.String(), "sql-queries") {
		t.Errorf("system prompt lacks the card or the skill list")
	}
	var names []string
	for _, tl := range calls[0].Tools {
		names = append(names, tl.Name)
	}
	for _, n := range []string{"query", "web_search", "load_skill"} {
		if !strings.Contains(" "+strings.Join(names, " ")+" ", " "+n+" ") {
			t.Errorf("tools %v lack %s", names, n)
		}
	}
	last := calls[2].Messages[len(calls[2].Messages)-1]
	var res string
	for _, p := range last.Parts {
		if p.ToolResult != nil {
			res += p.ToolResult.Text
		}
	}
	if !strings.Contains(res, "123.46") {
		t.Errorf("query result %q", res)
	}

	// The report: summary, Markdown, files and changes; no key anywhere.
	sum := rep.Summary[0]
	if sum.Runs != 1 || sum.Passed != 1 || sum.Skipped != 1 || sum.Checks["tool_called"] != (Rate{Pass: 1}) {
		t.Errorf("summary %+v", sum)
	}
	out := t.TempDir()
	md, err := rep.Write(out, nil)
	if err != nil {
		t.Fatal(err)
	}
	prev, err := Previous(out)
	if err != nil || prev == nil || len(prev.Runs) != 2 {
		t.Fatalf("previous: %v %v", prev, err)
	}
	prev.Runs[0].Asserts[0].Status = Fail
	prev.summarize()
	text := rep.Markdown(prev)
	for _, want := range []string{"| p/m | 1/1 |", "| close | 1/1 |", "skip (needs background)", "## Changes since", "text_before_tool 0/1 → 1/1", "max_prompt_tokens (info)"} {
		if !strings.Contains(text, want) {
			t.Errorf("Markdown lacks %q:\n%s", want, text)
		}
	}
	err = filepath.WalkDir(out, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if strings.Contains(string(b), key) {
			t.Errorf("%s has the key", p)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(strings.TrimSuffix(md, ".md"), "p_m", "close_r1.md")); err != nil {
		t.Error(err)
	}
}

func TestRunMother(t *testing.T) {
	s, err := Load(writeSet(t, map[string]string{
		"_set.yaml": `title: Prices
skills: skills
mother_skills: [delegation]
chats:
  - title: Reviewer
    role: Review pipelines.
    notes: Reviewed daily_prices.
tools:
  - name: create_chat
    description: Create a chat with a role and skills.
    schema:
      type: object
      properties: {title: {type: string}, role: {type: string}, message: {type: string}, skills: {type: array, items: {type: string}}}
      required: [title, role, message, skills]
    mother: true
    delay: 10s
rules:
  create_chat:
    - result: "started: chat c_new"
`,
		"skills/delegation/SKILL.md":  "---\nname: delegation\ndescription: How Mother hands work to chats.\n---\nUse create_chat.\n",
		"skills/sql-queries/SKILL.md": "---\nname: sql-queries\ndescription: SQL for the query tool.\n---\nOnly one SELECT.\n",
		"roles.yaml": `id: roles
context: mother
skills: [delegation]
messages:
  - text: Start a chat that answers SQL questions.
  - text: Also, call it SQL desk.
    at: 2s
asserts:
  - {type: created_skills, test: roles, tool: create_chat, skills: [sql-queries], allow: [config-guide]}
  - {type: created_skills, test: info, tool: create_chat, skills: [sql-queries]}
  - {type: after_message, test: roles, match: '(?i)sql desk'}
  - {type: only_skills, test: roles}
`,
	}))
	if err != nil {
		t.Fatal(err)
	}
	fp := fake.New(
		reply("I will start a chat for SQL questions.", &chat.ToolCall{ID: "c1", Name: "create_chat",
			Args: []byte(`{"title":"SQL","role":"Answer SQL questions.","message":"Hi","skills":["SQL-Queries","config-guide"]}`)}),
		reply("Done; I named it SQL desk.", nil),
	)
	models := Static([]ProviderSpec{{Name: "p", Kind: "openai_compatible", BaseURL: "https://example.test/v1/", Models: []config.ModelSettings{{ID: "m", Context: 32000}}}}, map[string]string{"p": "test-key-0123456789"}, "p/m")
	rep, err := RunSet(context.Background(), s, Options{
		Models: models, Backends: map[provider.Kind]provider.Factory{provider.KindCompatible: fp.Factory()},
		Scale: 0.01, Timeout: time.Minute, Temp: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	r := rep.Runs[0]
	if r.Error != "" {
		t.Fatalf("error: %s\n%s", r.Error, r.Transcript)
	}
	want := map[int]string{0: Pass, 1: Fail, 2: Pass, 3: Pass}
	for i, a := range r.Asserts {
		if a.Status != want[i] {
			t.Errorf("%d %s: %s (%s), want %s", i, a.Type, a.Status, a.Why, want[i])
		}
	}
	if !strings.Contains(r.Asserts[0].Why, "acceptable") || !strings.Contains(r.Asserts[2].Why, "during a turn") {
		t.Errorf("why: %q, %q", r.Asserts[0].Why, r.Asserts[2].Why)
	}
	if r.Turns != 1 || r.Requests != 2 {
		t.Errorf("turns %d, requests %d\n%s", r.Turns, r.Requests, r.Transcript)
	}
	calls := fp.Calls()
	var sys strings.Builder
	for _, b := range calls[0].System {
		sys.WriteString(b.Text)
	}
	for _, w := range []string{"Use create_chat.", "Reviewer", "sql-queries"} {
		if !strings.Contains(sys.String(), w) {
			t.Errorf("Mother's system prompt lacks %q", w)
		}
	}
	if !slices.ContainsFunc(calls[0].Tools, func(d provider.ToolDef) bool { return d.Name == "create_chat" }) {
		t.Error("Mother lacks create_chat")
	}
}

// A run that doesn't end in time is stopped and reported as broken.
func TestRunTimeout(t *testing.T) {
	s, err := Load(writeSet(t, map[string]string{
		"_set.yaml": "title: Prices\n",
		"a.yaml":    "id: a\ncontext: main\nmessages: [{text: hi}]\nasserts: [{type: answer_delivered, test: x}]\n",
	}))
	if err != nil {
		t.Fatal(err)
	}
	fp := fake.New(fake.Reply{Hang: true})
	models := Static([]ProviderSpec{{Name: "p", Kind: "openai_compatible", BaseURL: "https://example.test/v1/", Models: []config.ModelSettings{{ID: "m", Context: 32000}}}}, map[string]string{"p": "test-key-0123456789"}, "p/m")
	rep, err := RunSet(context.Background(), s, Options{
		Models: models, Backends: map[provider.Kind]provider.Factory{provider.KindCompatible: fp.Factory()},
		Timeout: 300 * time.Millisecond, Temp: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	r := rep.Runs[0]
	if !strings.Contains(r.Error, "took longer") || r.Passed() || rep.Summary[0].Errors != 1 {
		t.Errorf("run %+v", r)
	}
}

func TestStrList(t *testing.T) {
	for args, want := range map[string]string{
		`{"skills":["A"," b "]}`:      "a,b",
		`{"skills":"[\"A\", \"b\"]"}`: "a,b",
		`{"skills":"A, b,"}`:          "a,b",
		`{"skills":3}`:                "",
		`not json`:                    "",
	} {
		if got := strings.Join(strList(args, "skills"), ","); got != want {
			t.Errorf("%s: %q, want %q", args, got, want)
		}
	}
}

func TestPassed(t *testing.T) {
	ok := Run{Asserts: []Result{{Type: "a", Status: Pass}, {Type: "b", Test: "info", Status: Fail}}}
	for r, want := range map[*Run]bool{
		&ok: true,
		{Asserts: ok.Asserts, Error: "p: too_large"}:                              false,
		{Skipped: "needs background"}:                                             false,
		{Asserts: []Result{{Type: "a", Status: Fail}}}:                            false,
		{Asserts: []Result{{Type: "a", Status: Skip}, {Type: "b", Status: Pass}}}: true,
	} {
		if r.Passed() != want {
			t.Errorf("%+v: passed %v", *r, !want)
		}
	}
}
