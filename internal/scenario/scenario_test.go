package scenario

import (
	"context"
	"path/filepath"
	"testing"
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
