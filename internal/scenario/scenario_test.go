package scenario

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/amir-saatchi/jenab/internal/tool"
)

// The repo's sets load, and their fixtures run.
func TestRepoSets(t *testing.T) {
	sets, err := filepath.Glob(filepath.Join("..", "..", "scenarios", "*", SetFile))
	if err != nil || len(sets) == 0 {
		t.Fatalf("no sets: %v", err)
	}
	for _, f := range sets {
		dir := filepath.Dir(f)
		t.Run(filepath.Base(dir), func(t *testing.T) {
			s, err := Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			if len(s.Scenarios) == 0 {
				t.Fatal("no scenarios")
			}
			if s.Fixture != "" {
				db, err := openFixture(context.Background(), s.Fixture)
				if err != nil {
					t.Fatal(err)
				}
				db.Close()
			}
			n := 0
			for _, sc := range s.Scenarios {
				if sc.skip() == "" {
					n++
				}
			}
			// A set on a project with data fakes tools for its tables,
			// pipelines and views, so block 1 names all three (TASK-014).
			var areas tool.Areas
			for _, tl := range s.Tools {
				areas |= tl.areas
			}
			if s.Fixture != "" && areas != tool.Tables|tool.Pipelines|tool.Views {
				t.Errorf("the fake tools' areas are %b, not all three", areas)
			}
			t.Logf("%d scenarios, %d run now, %d skills", len(s.Scenarios), n, len(s.Skills))
		})
	}
}

func TestLoadErrors(t *testing.T) {
	const tool = "tools: [{name: web_search, description: Search.}]\n"
	const sc = "id: a\ncontext: main\nmessages: [{text: hi}]\n"
	for name, files := range map[string]map[string]string{
		"no set file":              {"a.yaml": sc},
		"unknown set field":        {"_set.yaml": "title: P\ncolour: red\n", "a.yaml": sc},
		"tool without description": {"_set.yaml": "tools: [{name: web_search}]\n", "a.yaml": sc},
		"bad tool name":            {"_set.yaml": "tools: [{name: web search, description: x}]\n", "a.yaml": sc},
		"sql without fixture":      {"_set.yaml": "tools: [{name: q, description: x, kind: sql, arg: sql}]\n", "a.yaml": sc},
		"rule for no tool":         {"_set.yaml": tool + "rules: {fetch: [{result: x}]}\n", "a.yaml": sc},
		"bad rule regex":           {"_set.yaml": tool + "rules: {web_search: [{match: '(', result: x}]}\n", "a.yaml": sc},
		"no scenarios":             {"_set.yaml": tool},
		"duplicate id":             {"_set.yaml": tool, "a.yaml": sc, "b.yaml": sc},
		"bad context":              {"_set.yaml": tool, "a.yaml": "id: a\ncontext: side\nmessages: [{text: hi}]\n"},
		"role in mother":           {"_set.yaml": tool, "a.yaml": "id: a\ncontext: mother\nrole: x\nmessages: [{text: hi}]\n"},
		"unknown need":             {"_set.yaml": tool, "a.yaml": sc + "needs: [time travel]\n"},
		"no messages":              {"_set.yaml": tool, "a.yaml": "id: a\ncontext: main\n"},
		"first message timed":      {"_set.yaml": tool, "a.yaml": "id: a\ncontext: main\nmessages: [{text: hi, at: 1s}]\n"},
		"first message restarts":   {"_set.yaml": tool, "a.yaml": "id: a\ncontext: main\nmessages: [{text: hi, restart: true}]\n"},
		"timed restart":            {"_set.yaml": tool, "a.yaml": "id: a\ncontext: main\nmessages: [{text: hi}, {text: ho, at: 1s, restart: true}]\n"},
		"unknown assert":           {"_set.yaml": tool, "a.yaml": sc + "asserts: [{type: vibes, test: x}]\n"},
		"assert missing field":     {"_set.yaml": tool, "a.yaml": sc + "asserts: [{type: tool_called, test: x}]\n"},
		"assert extra field":       {"_set.yaml": tool, "a.yaml": sc + "asserts: [{type: answer_delivered, test: x, match_all: [a]}]\n"},
		"assert bad turn":          {"_set.yaml": tool, "a.yaml": sc + "asserts: [{type: answer_delivered, test: x, turn: middle}]\n"},
		"assert turn 0":            {"_set.yaml": tool, "a.yaml": sc + "asserts: [{type: answer_delivered, test: x, turn: \"0\"}]\n"},
		"delay for no tool":        {"_set.yaml": tool, "a.yaml": sc + "delays: {fetch: 1s}\n"},
		"bad duration":             {"_set.yaml": tool, "a.yaml": sc + "delays: {web_search: soon}\n"},
		"bad skill":                {"_set.yaml": "skills: skills\n" + tool, "a.yaml": sc, "skills/x/SKILL.md": "no front matter"},
		"unknown mother skill":     {"_set.yaml": "skills: skills\nmother_skills: [x]\n" + tool, "a.yaml": sc, "skills/y/SKILL.md": "---\nname: y\ndescription: Y.\n---\nBody.\n"},
		"typo in a skipped one":    {"_set.yaml": tool, "a.yaml": sc + "needs: [background]\nasserts: [{type: backgroud_started, test: x, tools: [run]}]\n"},
		"planned without its need": {"_set.yaml": tool, "a.yaml": sc + "asserts: [{type: background_started, test: x, tools: [run]}]\n"},
		"bad when":                 {"_set.yaml": tool, "a.yaml": sc + "asserts: [{type: after_message, test: x, when: later}]\n"},
		"when on another check":    {"_set.yaml": tool, "a.yaml": sc + "asserts: [{type: answer_delivered, test: x, when: during}]\n"},
		"share above 1":            {"_set.yaml": tool, "a.yaml": sc + "asserts: [{type: cache_read_share, min: 2}]\n"},
		"settings providers":       {"_set.yaml": tool, "a.yaml": sc + "settings: {llm: {providers: {p: {kind: openai}}}}\n"},
		"settings other section":   {"_set.yaml": tool, "a.yaml": sc + "settings: {ui: {theme: dark}}\n"},
		"settings unknown key":     {"_set.yaml": tool, "a.yaml": sc + "settings: {llm: {turn_max_request: 2}}\n"},
		"settings bad value":       {"_set.yaml": tool, "a.yaml": sc + "settings: {llm: {turn_max_requests: many}}\n"},
		"set settings models":      {"_set.yaml": tool + "settings: {llm: {models: {default: p/m}}}\n", "a.yaml": sc},
		"bad false_running":        {"_set.yaml": tool + "false_running: ['(']\n", "a.yaml": sc},
		"unknown area":             {"_set.yaml": "tools: [{name: q, description: x, areas: [charts]}]\n", "a.yaml": sc},
		"bad grounding mode":       {"_set.yaml": tool + "grounding: {mode: loud}\n", "a.yaml": sc},
		"negative grounding k":     {"_set.yaml": tool, "a.yaml": sc + "grounding: {mode: mark, k: -1}\n"},
		"unknown grounding field":  {"_set.yaml": tool, "a.yaml": sc + "grounding: {mode: mark, n: 3}\n"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Load(writeSet(t, files)); err == nil {
				t.Error("loaded")
			} else {
				t.Log(err)
			}
		})
	}
}

// A set that fakes a data tool gets none of the real ones, which would see
// the project's empty database; a set with data gets them all.
func TestDataTools(t *testing.T) {
	for _, set := range []string{"skill-loading", "data-tools"} {
		s, err := Load(filepath.Join("..", "..", "scenarios", set))
		if err != nil {
			t.Fatal(err)
		}
		reg := (&run{sc: s.Scenarios[0]}).tools()
		for _, n := range dataTools {
			_, ok := reg.Get(n)
			faked := s.tool(n) != nil
			switch {
			case set == "data-tools" && (!ok || faked):
				t.Errorf("%s: no real %s", set, n)
			case set == "skill-loading" && ok != faked:
				t.Errorf("%s: %s is there: %v, faked: %v", set, n, ok, faked)
			}
		}
	}
}
