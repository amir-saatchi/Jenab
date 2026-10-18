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
)

// scriptSet has settings in the set and in a scenario, the new checks, and
// a second message during a turn and between turns.
var scriptSet = map[string]string{
	"_set.yaml": `title: Prices
settings: {llm: {turn_max_requests: 5}}
skills: skills
tools:
  - name: web_search
    description: Search the web.
    schema: {type: object, properties: {query: {type: string}}, required: [query]}
  - name: slow
    description: A slow job.
    schema: {type: object, properties: {}}
    delay: 10s
`,
	"skills/sql-queries/SKILL.md": "---\nname: sql-queries\ndescription: SQL for the query tool.\n---\nOnly one SELECT.\n",
	"capped.yaml": `id: capped
context: main
settings: {llm: {turn_max_requests: 2}}
messages: [{text: Search twice.}]
asserts:
  - {type: max_requests, test: x, n: 2}
  - {type: no_turn_failed, test: x}
  - {type: cache_read_share, min: 0.5}
  - {type: skill_loaded, test: x, skills: [sql-queries]}
`,
	"free.yaml": `id: free
context: main
messages: [{text: Search twice.}]
asserts:
  - {type: max_requests, test: x, n: 2}
  - {type: cache_read_share, test: x, min: 0.5}
`,
	"during.yaml": `id: during
context: main
messages:
  - text: Start the slow job.
  - {text: Call it nightly., at: 2s}
asserts:
  - {type: after_message, test: x, when: during, match: '(?i)nightly'}
  - {type: after_message, test: y, when: between}
`,
	"between.yaml": `id: between
context: main
messages:
  - text: Hi.
  - {text: Call it nightly., at: 100s} # 1 s at scale 0.01, well after the first turn
asserts:
  - {type: after_message, test: x, when: between}
  - {type: after_message, test: y, when: during}
`,
}

const script = `scenarios:
  capped:
    - calls: [{name: load_skill, args: {name: sql-queries}}]
      usage: {input: 100, cache_write: 900}
    - calls: [{name: web_search, args: {query: a}}]
      usage: {input: 100, cache_read: 900}
    - text: Found it.
  free:
    - calls: [{name: web_search, args: {query: a}}]
    - calls: [{name: web_search, args: {query: b}}]
    - text: Found it.
  during:
    - calls: [{name: slow}]
    - text: Started; it is called nightly.
    - text: Done.
  between:
    - text: Hello.
    - text: Done; called nightly.
`

func TestScriptedRun(t *testing.T) {
	s, err := Load(writeSet(t, scriptSet))
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "_plays.yaml")
	if err := os.WriteFile(p, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	sc, err := ReadScript(p)
	if err != nil {
		t.Fatal(err)
	}
	if sc.Name != "plays" {
		t.Errorf("name %q", sc.Name)
	}
	rep, err := RunSet(context.Background(), s, Options{
		Models: ScriptedModels(sc), Script: sc, Scale: 0.01, Timeout: time.Minute, Temp: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Models[0] != "scripted/plays" {
		t.Errorf("models %v", rep.Models)
	}
	want := map[string][]string{
		"capped":  {Pass, Pass, Fail, Pass}, // 900 of 2000 read from the cache
		"free":    {Fail, Skip},
		"during":  {Pass, Fail},
		"between": {Pass, Fail},
	}
	for _, r := range rep.Runs {
		if r.Error != "" {
			t.Fatalf("%s: %s\n%s", r.Scenario, r.Error, r.Transcript)
		}
		for i, a := range r.Asserts {
			if a.Status != want[r.Scenario][i] {
				t.Errorf("%s %d %s: %s (%s), want %s", r.Scenario, i, a.Type, a.Status, a.Why, want[r.Scenario][i])
			}
		}
		switch r.Scenario {
		case "capped":
			if r.Requests != 2 || r.Input != 200 || r.CacheRead != 900 || r.CacheWrite != 900 || r.Prompt != 2000 {
				t.Errorf("capped: %+v", r)
			}
			if r.Asserts[2].Test != "info" || !strings.Contains(r.Asserts[2].Why, "45 %") {
				t.Errorf("cache_read_share: %+v", r.Asserts[2])
			}
			if !strings.Contains(r.Transcript, "| 2 | 100 | 900 | 0 |") {
				t.Errorf("transcript lacks the cache columns:\n%s", r.Transcript)
			}
		case "free":
			if r.Requests != 3 || !strings.Contains(r.Asserts[0].Why, "3 requests") {
				t.Errorf("free: %+v", r)
			}
		case "between":
			if !strings.Contains(r.Asserts[1].Why, "arrived between turns, not during a turn") {
				t.Errorf("between: %q", r.Asserts[1].Why)
			}
		}
	}
	if got := rep.Summary[0].CacheRead; got != 0.45 {
		t.Errorf("cache share %v", got)
	}
	if md := rep.Markdown(nil); !strings.Contains(md, "| Cache read |") || !strings.Contains(md, " 45 % |") {
		t.Errorf("Markdown:\n%s", md)
	}
}

// The skill-loading set passes with its offline script, without keys.
func TestOfflineScript(t *testing.T) {
	dir := filepath.Join("..", "..", "scenarios", "skill-loading")
	s, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	sc, err := ReadScript(filepath.Join(dir, "_offline.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for id := range sc.Scenarios {
		if !slices.ContainsFunc(s.Scenarios, func(x *Scenario) bool { return x.ID == id }) {
			t.Errorf("the script has %s, which the set doesn't", id)
		}
	}
	rep, err := RunSet(context.Background(), s, Options{
		Models: ScriptedModels(sc), Script: sc, Scale: 0.01, Timeout: time.Minute, Temp: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rep.Runs {
		if !r.Passed() {
			t.Errorf("%s: %s %+v", r.Scenario, r.Error, r.Asserts)
		}
	}
	if sum := rep.Summary[0]; sum.Runs != len(s.Scenarios) || sum.Passed != sum.Runs {
		t.Errorf("summary %+v", sum)
	}
}

func TestReadScriptErrors(t *testing.T) {
	for name, body := range map[string]string{
		"empty":         "scenarios: {}\n",
		"unknown field": "scenarios: {a: [{text: hi}]}\nmodel: x\n",
		"empty reply":   "scenarios: {a: [{usage: {input: 1}}]}\n",
		"bad tool":      "scenarios: {a: [{calls: [{name: web search}]}]}\n",
	} {
		t.Run(name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "s.yaml")
			if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := ReadScript(p); err == nil {
				t.Error("read")
			} else {
				t.Log(err)
			}
		})
	}
}

// A set in another language gives its own false_running text.
func TestFalseRunningFromSet(t *testing.T) {
	s, err := Load(writeSet(t, map[string]string{
		"_set.yaml": "title: P\nfalse_running: ['در حال اجرا', 'ادامه دارد']\n",
		"a.yaml":    "id: a\ncontext: main\nmessages: [{text: سلام}]\nasserts: [{type: no_false_running, test: x}]\n",
	}))
	if err != nil {
		t.Fatal(err)
	}
	sc := s.Scenarios[0]
	for text, want := range map[string]string{
		"گزارش آماده است.":          Pass,
		"کار در حال اجرا است.":      Fail,
		"The job is still running.": Pass, // the set's text replaces the English one
	} {
		rec := &record{sc: sc, events: []event{{kind: evText, turn: 1, text: text, final: true}}}
		if st, why, _ := rec.check(sc.Asserts[0]); st != want {
			t.Errorf("%q: %s (%s), want %s", text, st, why, want)
		}
	}
}

func TestNoTurnFailed(t *testing.T) {
	sc := &Scenario{set: &Set{}}
	a := Assert{Type: "no_turn_failed", Test: "x"}
	rec := &record{sc: sc, events: []event{{kind: evNotice, turn: 1, notice: chat.NoticeTurnFailed, text: "p: too_large"}}}
	if st, why, _ := rec.check(a); st != Fail || !strings.Contains(why, "turn 1 failed: p: too_large") {
		t.Errorf("%s (%s)", st, why)
	}
}

// With 3 or more reps, the report shows the least and the most of a rep's
// pass share.
func TestSpread(t *testing.T) {
	run := func(rep int, st string) Run {
		return Run{Model: "m", Scenario: "a", Rep: rep, Asserts: []Result{{Type: "tool_called", Test: "x", Status: st}}}
	}
	rep := &Report{Models: []string{"m"}, Reps: 3, Runs: []Run{run(1, Pass), run(2, Fail), run(3, Pass)}}
	rep.summarize()
	sp := rep.Summary[0].Spread
	if sp[spreadRuns] != (Range{0, 1}) || sp["tool_called"] != (Range{0, 1}) {
		t.Errorf("spread %v", sp)
	}
	md := rep.Markdown(nil)
	for _, w := range []string{"| m | 2/3 (0–100 %) |", "| tool_called | 2/3 (0–100 %) |"} {
		if !strings.Contains(md, w) {
			t.Errorf("Markdown lacks %q:\n%s", w, md)
		}
	}
	rep.Reps, rep.Runs = 2, rep.Runs[:2]
	rep.summarize()
	if rep.Summary[0].Spread != nil {
		t.Errorf("spread with 2 reps: %v", rep.Summary[0].Spread)
	}
}

// A baseline is any report's JSON file.
func TestReadReport(t *testing.T) {
	rep := &Report{Models: []string{"m"}, Reps: 1, Runs: []Run{{Model: "m", Scenario: "a", Rep: 1, Asserts: []Result{{Type: "tool_called", Test: "x", Status: Fail}}}}}
	rep.summarize()
	md, err := rep.Write(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	base, err := ReadReport(strings.TrimSuffix(md, ".md") + ".json")
	if err != nil {
		t.Fatal(err)
	}
	rep.Runs[0].Asserts[0].Status = Pass
	rep.summarize()
	if text := rep.Markdown(base); !strings.Contains(text, "tool_called 0/1 → 1/1") {
		t.Errorf("Markdown:\n%s", text)
	}
	if _, err := ReadReport(filepath.Join(t.TempDir(), "none.json")); err == nil {
		t.Error("read a missing report")
	}
}
