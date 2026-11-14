package scenario

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/agent"
	"github.com/amir-saatchi/jenab/internal/config"
)

// harnessSet is a set of three days with a fake page each, for P2-13's
// harnesses and day clock.
var harnessSet = map[string]string{
	"_set.yaml": `title: Prices
today: 2026-10-05
tools:
  - {name: fetch_page, kind: page}
rules:
  fetch_page:
    - {match: '2026-10-05', file: pages/a.md}
    - {match: '2026-10-06', result: "# Tuesday\n\nBTC 63,105"}
`,
	"pages/a.md": "# Monday\r\n\r\nBTC 62,418\r\n",
	"b.yaml": `id: B
context: main
messages:
  - {day: 2026-10-05, text: Monday's page.}
  - {text: And again.}
  - {day: 2026-10-06, text: Tuesday's page.}
asserts:
  - {type: tool_called, tool: fetch_page, turn: "1", result_match: 'ref: cache/pages/markets.example/'}
  - {type: tool_called, tool: update_memory, harness: jenab, test: info}
  - {type: answer_contains, harness: plain, match: 'ok'}
`,
	"_script.yaml": `scenarios:
  B:
    - calls: [{name: fetch_page, args: {url: "https://markets.example/daily/2026-10-05"}}]
    - text: "62,418"
    - text: ok
    - calls: [{name: fetch_page, args: {url: "https://markets.example/daily/2026-10-06"}}]
    - text: "63,105"
`,
}

func TestHarnesses(t *testing.T) {
	dir := writeSet(t, harnessSet)
	s, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	script, err := ReadScript(dir + "/_script.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var deps []*agent.Deps
	rep, err := RunSet(context.Background(), s, Options{
		Models: ScriptedModels(script), Script: script, Harnesses: []string{HarnessJenab, HarnessPlain}, Temp: t.TempDir(),
		Deps: func(d *agent.Deps) { deps = append(deps, d) },
		Settings: func(c *config.Settings) {
			c.Context.HistoryMaxTurns = 8 // no cut by turns
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Runs) != 2 || rep.Runs[0].Harness != HarnessJenab || rep.Runs[1].Harness != HarnessPlain {
		t.Fatalf("runs: %+v", rep.Runs)
	}
	if got := rep.lanes(); !slices.Equal(got, []string{"scripted/script · jenab", "scripted/script · plain"}) {
		t.Fatalf("lanes %q", got)
	}
	if !deps[1].Plain || deps[1].Card != nil || deps[1].Skills != nil || deps[0].Plain {
		t.Errorf("deps: jenab plain %v, plain %+v", deps[0].Plain, deps[1])
	}
	for _, r := range rep.Runs {
		if !r.Passed() {
			t.Errorf("%s: %+v", r.Harness, r.Asserts)
		}
		if len(r.TurnStats) != 3 || r.TurnStats[2].Turn != 3 || r.TurnStats[0].Estimate == 0 || len(r.TurnStats[0].Blocks) == 0 {
			t.Errorf("%s: turn stats %+v", r.Harness, r.TurnStats)
		}
		skipped := 0
		for _, a := range r.Asserts {
			if a.Status == Skip {
				skipped++
			}
		}
		if skipped != 1 {
			t.Errorf("%s: %d checks skipped, want the other harness's one: %+v", r.Harness, skipped, r.Asserts)
		}
	}
	// The plain harness grows: each turn's first request is larger than
	// the last. Jenab grows too: a new day alone doesn't cut, it comes as
	// a note (TASK-047).
	p, j := rep.Runs[1].TurnStats, rep.Runs[0].TurnStats
	if !(p[0].Estimate < p[1].Estimate && p[1].Estimate < p[2].Estimate) {
		t.Errorf("plain doesn't grow: %+v", p)
	}
	if !(j[1].Estimate < j[2].Estimate) {
		t.Errorf("jenab cut on the new day: %+v", j)
	}
	md := rep.Markdown(nil)
	for _, want := range []string{"## Side by side", "### Per turn", "scripted/script · plain"} {
		if !strings.Contains(md, want) {
			t.Errorf("the report has no %q", want)
		}
	}
}

func TestPlainTools(t *testing.T) {
	s, err := Load(writeSet(t, harnessSet))
	if err != nil {
		t.Fatal(err)
	}
	r := &run{sc: s.Scenarios[0], harness: HarnessPlain}
	var names []string
	for _, tl := range r.tools().For("main") {
		names = append(names, tl.Spec().Name)
	}
	slices.Sort(names)
	if want := []string{"ask_user", "fetch_page", "read_ref", "search_ref"}; !slices.Equal(names, want) {
		t.Errorf("plain tools %q, want %q", names, want)
	}
}

func TestLoadDayAndRuleFile(t *testing.T) {
	s, err := Load(writeSet(t, harnessSet))
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Rules["fetch_page"][0].Result; got != "# Monday\n\nBTC 62,418\n" {
		t.Errorf("rule file %q", got)
	}
	if d := s.Scenarios[0].Messages[2].day; d.Day() != 6 {
		t.Errorf("day %v", d)
	}
	for _, bad := range []map[string]string{
		{"_set.yaml": "title: T\n", "a.yaml": "id: A\ncontext: main\nmessages: [{text: hi, day: 2026-13-01}]\n"},
		{"_set.yaml": "title: T\ntools: [{name: fetch_page, kind: page}]\nrules: {fetch_page: [{file: nope.md}]}\n", "a.yaml": "id: A\ncontext: main\nmessages: [{text: hi}]\n"},
		{"_set.yaml": "title: T\ntools: [{name: web, kind: page, description: d}]\n", "a.yaml": "id: A\ncontext: main\nmessages: [{text: hi}]\n"},
		{"_set.yaml": "title: T\n", "a.yaml": "id: A\ncontext: main\nmessages: [{text: hi}]\nasserts: [{type: no_turn_failed, harness: other}]\n"},
	} {
		if _, err := Load(writeSet(t, bad)); err == nil {
			t.Errorf("loaded %v", bad)
		}
	}
}
