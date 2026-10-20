package scenario

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// groundSet has the number check in the set, a scenario that overrides
// it and one that turns it off (SPIKE-032).
var groundSet = map[string]string{
	"_set.yaml": `title: Closes
grounding: {mode: same_turn, k: 2}
tools:
  - name: web_search
    description: Search the web.
    schema: {type: object, properties: {query: {type: string}}, required: [query]}
rules:
  web_search:
    - result: 'ETH close on 2026-09-28: 2,561.35 USD.'
`,
	"made.yaml": `id: made
context: main
messages: [{text: "ETH closes this week?"}]
asserts:
  - {type: grounding_fired, test: x}
  - {type: grounding_quiet, test: y}
`,
	"honest.yaml": `id: honest
context: main
grounding: {mode: mark}
messages: [{text: "ETH close yesterday?"}]
asserts:
  - {type: grounding_quiet, test: x}
`,
	"off.yaml": `id: off
context: main
grounding: {mode: "off"}
messages: [{text: "ETH closes this week?"}]
asserts:
  - {type: grounding_fired, test: x}
`,
}

const groundScript = `scenarios:
  made:
    - calls: [{name: web_search, args: {query: eth close}}]
    - text: 'Closes: 09-26 2,470.10, 09-27 2,530.55, 09-28 2,561.35.'
      usage: {input: 3000, output: 20}
    - text: 'I only found 2,561.35 for 09-28.'
      usage: {input: 4000, output: 10}
  honest:
    - calls: [{name: web_search, args: {query: eth close}}]
    - text: It closed at 2,561.35 USD (about 2.56k).
  off:
    - calls: [{name: web_search, args: {query: eth close}}]
    - text: 'Closes: 09-26 2,470.10, 09-27 2,530.55.'
`

func TestGroundingRun(t *testing.T) {
	s, err := Load(writeSet(t, groundSet))
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "_plays.yaml")
	if err := os.WriteFile(p, []byte(groundScript), 0o644); err != nil {
		t.Fatal(err)
	}
	sc, err := ReadScript(p)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := RunSet(context.Background(), s, Options{
		Models: ScriptedModels(sc), Script: sc, Scale: 0.01, Timeout: time.Minute, Temp: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"made":   {Pass, Fail},
		"honest": {Pass},
		"off":    {Skip},
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
		case "made":
			if r.GroundMode != "same_turn" || r.GroundK != 2 || len(r.Grounding) != 2 {
				t.Fatalf("made: %s %d %+v", r.GroundMode, r.GroundK, r.Grounding)
			}
			g, after := r.Grounding[0], r.Grounding[1]
			if !g.Fired || !g.Told || g.Request != 2 || strings.Join(g.Unsourced, " ") != "2,470.10 2,530.55" {
				t.Errorf("first check %+v", g)
			}
			if !after.After || after.Fired || after.Request != 3 || len(after.Unsourced) != 0 {
				t.Errorf("check after the notice %+v", after)
			}
			if x := r.GroundExtra; x == nil || x.Requests != 1 || x.Prompt != 4000 || x.Output != 10 {
				t.Errorf("extra %+v", x)
			}
			if r.groundCell() != "2*→0" {
				t.Errorf("cell %q", r.groundCell())
			}
			for _, s := range []string{"## Number check", "Mode same_turn, K 2.", "request 2 (fired, told): 2 of 3 numbers without a source: 2,470.10, 2,530.55", "[app check: 2 numbers"} {
				if !strings.Contains(r.Transcript, s) {
					t.Errorf("transcript lacks %q:\n%s", s, r.Transcript)
				}
			}
		case "honest":
			if r.GroundMode != "mark" || r.GroundK != 3 || len(r.Grounding) != 1 || r.Grounding[0].Fired || r.GroundExtra != nil {
				t.Errorf("honest: %s %d %+v %+v", r.GroundMode, r.GroundK, r.Grounding, r.GroundExtra)
			}
		case "off":
			if r.GroundMode != "" || r.Grounding != nil || strings.Contains(r.Transcript, "Number check") {
				t.Errorf("off: %s %+v", r.GroundMode, r.Grounding)
			}
		}
	}
	if md := rep.Markdown(nil); !strings.Contains(md, "## Number check") || !strings.Contains(md, "| made | 2*→0 |") {
		t.Errorf("Markdown:\n%s", md)
	}
}

// The runner's grounding wins over the set's and the scenario's, and the
// mark stays the user's: the model's requests don't have it.
func TestGroundingOption(t *testing.T) {
	s, err := Load(writeSet(t, groundSet))
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "_plays.yaml")
	if err := os.WriteFile(p, []byte(groundScript), 0o644); err != nil {
		t.Fatal(err)
	}
	sc, err := ReadScript(p)
	if err != nil {
		t.Fatal(err)
	}
	g, err := ParseGrounding("mark", 0)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := RunSet(context.Background(), s, Options{
		Models: ScriptedModels(sc), Script: sc, Scale: 0.01, Timeout: time.Minute, Temp: t.TempDir(),
		Only: []string{"off"}, Grounding: g,
	})
	if err != nil {
		t.Fatal(err)
	}
	r := rep.Runs[0]
	if r.Error != "" || r.GroundMode != "mark" || r.GroundK != 3 || len(r.Grounding) != 1 {
		t.Fatalf("%s %s %d %+v", r.Error, r.GroundMode, r.GroundK, r.Grounding)
	}
	if c := r.Grounding[0]; c.Fired || c.Told || len(c.Unsourced) != 2 {
		t.Errorf("2 of K 3 fired: %+v", c)
	}
	if r.Asserts[0].Status != Fail || r.GroundExtra != nil {
		t.Errorf("%+v %+v", r.Asserts[0], r.GroundExtra)
	}
}

func TestParseGrounding(t *testing.T) {
	for _, c := range []struct {
		mode string
		k    int
		want string
	}{{"mark", 0, "mark"}, {"same_turn", 5, "same_turn"}, {"next_turn", 8, "next_turn"}, {"", 0, ""}, {"off", 3, ""}} {
		g, err := ParseGrounding(c.mode, c.k)
		if err != nil || g.Mode != c.want || g.K != c.k {
			t.Errorf("%s %d: %+v %v", c.mode, c.k, g, err)
		}
	}
	for _, c := range []struct {
		mode string
		k    int
	}{{"loud", 0}, {"mark", -1}} {
		if _, err := ParseGrounding(c.mode, c.k); err == nil {
			t.Errorf("%s %d: no error", c.mode, c.k)
		}
	}
	if got := (*GroundingSpec)(nil).agent(); got.Mode != "" {
		t.Errorf("nil spec %+v", got)
	}
}
