package scenario

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// scriptedExamples runs a set from scenarios/ with its _offline.yaml and
// Options.Examples.
func scriptedExamples(t *testing.T, set string) *Report {
	t.Helper()
	dir := filepath.Join("..", "..", "scenarios", set)
	s, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	sc, err := ReadScript(filepath.Join(dir, "_offline.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	rep, err := RunSet(context.Background(), s, Options{
		Models: ScriptedModels(sc), Script: sc, Scale: 0.01, Timeout: time.Minute, Temp: t.TempDir(), Examples: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return rep
}

func readExamples(t *testing.T, text string) []Example {
	t.Helper()
	var out []Example
	sc := bufio.NewScanner(strings.NewReader(text))
	sc.Buffer(nil, 16<<20)
	for sc.Scan() {
		var ex Example
		if err := json.Unmarshal(sc.Bytes(), &ex); err != nil {
			t.Fatalf("%v: %s", err, sc.Text())
		}
		out = append(out, ex)
	}
	return out
}

// A chat's examples are its requests as the model saw them, with the
// answer last: the system prompt, the user's message, the calls and their
// results, and the tools.
func TestExamplesChat(t *testing.T) {
	rep := scriptedExamples(t, "data-tools")
	for _, r := range rep.Runs {
		if !r.Passed() {
			t.Fatalf("%s: %s %+v", r.Scenario, r.Error, r.Asserts)
		}
		exs := readExamples(t, r.Examples)
		if len(exs) != r.Requests {
			t.Fatalf("%s: %d examples for %d requests:\n%s", r.Scenario, len(exs), r.Requests, r.Examples)
		}
		for i, ex := range exs {
			m := ex.Meta
			if m.Agent != AgentChat || m.Request != i+1 || m.Turn != 1 {
				t.Errorf("%s %d: meta %+v", r.Scenario, i, m)
			}
			if ex.Messages[0].Role != "system" || ex.Messages[1].Role != "user" || len(ex.Tools) == 0 {
				t.Errorf("%s %d: starts %+v with %d tools", r.Scenario, i, ex.Messages[:2], len(ex.Tools))
			}
			last := ex.Messages[len(ex.Messages)-1]
			if last.Role != "assistant" {
				t.Errorf("%s %d: ends with %s", r.Scenario, i, last.Role)
			}
			if i < len(exs)-1 && (len(last.ToolCalls) == 0 || m.Stop != "tool_use") {
				t.Errorf("%s %d: a request before the last should end with calls: %+v", r.Scenario, i, last)
			}
			if i == len(exs)-1 && (last.Content == "" || m.Stop != "end") {
				t.Errorf("%s %d: the last should end with the answer: %+v", r.Scenario, i, last)
			}
			if i > 0 {
				// The request after a call has the call and its result.
				prev := exs[i-1].Messages[len(exs[i-1].Messages)-1]
				got := ex.Messages[len(ex.Messages)-3]
				if got.Role != "assistant" || len(got.ToolCalls) != len(prev.ToolCalls) || ex.Messages[len(ex.Messages)-2].Role != "tool" {
					t.Errorf("%s %d: no call and result before the answer: %+v", r.Scenario, i, ex.Messages)
				}
				if tool := ex.Messages[len(ex.Messages)-2]; tool.ToolName != prev.ToolCalls[len(prev.ToolCalls)-1].Function.Name {
					t.Errorf("%s %d: tool_name %q", r.Scenario, i, tool.ToolName)
				}
			}
		}
	}
}

// A subagent's requests are examples of their own, named after it.
func TestExamplesSubagent(t *testing.T) {
	rep := scriptedExamples(t, "schema-widen")
	var chat, sub int
	for _, r := range rep.Runs {
		if !r.Passed() {
			t.Fatalf("%s: %s %+v", r.Scenario, r.Error, r.Asserts)
		}
		for _, ex := range readExamples(t, r.Examples) {
			if ex.Meta.Agent == AgentChat {
				chat++
				continue
			}
			sub++
			if ex.Messages[0].Role != "system" || len(ex.Tools) == 0 || ex.Meta.Request != 0 {
				t.Errorf("subagent %s: %+v", ex.Meta.Agent, ex.Meta)
			}
		}
		if r.SubRequests == 0 || sub != r.SubRequests {
			t.Errorf("%s: %d subagent examples for %d requests", r.Scenario, sub, r.SubRequests)
		}
	}
	if chat == 0 {
		t.Error("no chat examples")
	}
}

// Export takes the passing runs of training sets, refuses a held-out set,
// and writes nothing when the scrub check finds a personal ID.
func TestExport(t *testing.T) {
	rep := scriptedExamples(t, "data-tools")
	dir := t.TempDir()
	rep.Split = "train"
	// One run fails: its examples stay out.
	rep.Runs[0].Asserts = append(rep.Runs[0].Asserts, Result{Type: "x", Test: "x", Status: Fail})
	if _, err := rep.Write(dir, nil); err != nil {
		t.Fatal(err)
	}
	reports, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	out := filepath.Join(dir, "out")
	st, err := Export(reports, ExportOptions{Out: out, Scrub: &Scrubber{rules: builtinScrub}})
	if err != nil {
		t.Fatal(err)
	}
	want := 0
	for _, r := range rep.Runs[1:] {
		want += r.Requests
	}
	if st.Runs != len(rep.Runs) || st.Passed != len(rep.Runs)-1 || st.Chat != want || st.Subagent != 0 {
		t.Errorf("stats %+v, want %d chat", st, want)
	}
	b, err := os.ReadFile(filepath.Join(out, "chat.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, ex := range readExamples(t, string(b)) {
		if ex.Meta.Scenario == rep.Runs[0].Scenario || ex.Meta.Set != rep.Set || ex.Meta.Split != "train" || ex.Meta.Model != "scripted/offline" || ex.Meta.Rep != 1 {
			t.Errorf("meta %+v", ex.Meta)
		}
	}

	// A held-out set is refused, unless its split is asked for.
	rep.Split = "heldout"
	if _, err := rep.Write(dir, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := Export(reports, ExportOptions{Out: out}); err == nil || !strings.Contains(err.Error(), "held-out") {
		t.Errorf("held out: %v", err)
	}
	if _, err := Export(reports, ExportOptions{Out: out, Splits: []string{"heldout"}}); err != nil {
		t.Errorf("held out, asked for: %v", err)
	}

	// A word from the examples, as if it were the user's name, stops the
	// export; the message names the rule, not the value.
	os.RemoveAll(out)
	s, err := NewScrubber(map[string]string{"the git user.name": "Ledger"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Export(reports, ExportOptions{Out: out, Splits: []string{"heldout"}, Scrub: s})
	if err == nil || !strings.Contains(err.Error(), "the git user.name") || strings.Contains(err.Error(), "Ledger") {
		t.Errorf("scrub: %v", err)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Errorf("the export wrote %s anyway", out)
	}
}

// Report runs without examples can't be exported.
func TestExportNeedsExamples(t *testing.T) {
	dir := t.TempDir()
	rep := &Report{Set: "s", Split: "train", Started: time.Now(), Models: []string{"m"}, Runs: []Run{{Model: "m", Scenario: "a", Rep: 1, Transcript: "x"}}}
	if _, err := rep.Write(dir, nil); err != nil {
		t.Fatal(err)
	}
	reports, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	if _, err := Export(reports, ExportOptions{Out: filepath.Join(dir, "out")}); err == nil || !strings.Contains(err.Error(), "-examples") {
		t.Errorf("got %v", err)
	}
}

func TestScrubber(t *testing.T) {
	s, err := NewScrubber(map[string]string{"name": "Jane Roe", "short": "ab"}, []string{`ACME-\d+`})
	if err != nil {
		t.Fatal(err)
	}
	for text, want := range map[string]string{
		"Total: 1,234.50 USD over 12 rows.":           "",
		"write to me at jane.roe@example.org":         "an e-mail address",
		`saved in C:\Users\jroe\Documents\a.csv`:      "a home folder",
		`saved in C:\\Users\\jroe\\a.csv`:             "a home folder",
		"open /home/jroe/data.db":                     "a home folder",
		"key sk-ant-REDACTEDREDACTEDREDACTED123":      "an API key",
		"AIza" + strings.Repeat("x", 35):              "an API key",
		"signed, jane roe.":                           "name",
		"the table has ab and abc":                    "",
		"order ACME-42":                               "the deny pattern ACME-\\d+",
		"the price of a bus/home/garden kit is 12.00": "",
	} {
		if got := s.Find(text); got != want {
			t.Errorf("%q: %q, want %q", text, got, want)
		}
	}
	if _, err := NewScrubber(nil, []string{"("}); err == nil {
		t.Error("a bad regex passed")
	}
}
