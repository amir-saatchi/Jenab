package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/provider/fake"
	"github.com/amir-saatchi/jenab/internal/skill"
	"github.com/amir-saatchi/jenab/internal/tool"
)

func skillSet(t *testing.T, ss ...skill.Skill) *skill.Set {
	t.Helper()
	s, err := skill.New(ss...)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

var (
	sqlSkill   = skill.Skill{Name: "sql", Description: "SQL. Load to write SQL.", Body: "Use named parameters.", Files: map[string]string{"recipes.md": "Recipe one."}}
	viewsSkill = skill.Skill{Name: "views", Description: "Views. Load before save_thing.", LoadWith: []string{"save_thing"}, Body: "Views have columns."}
	motherOnly = skill.Skill{Name: "delegation", Description: "Delegation.", Body: "Delegate.", Mother: true}
)

// saveThing is a tool with skills in load_with.
func saveThing() tool.Tool {
	return tool.Func(tool.Spec{Name: "save_thing", Description: "Save a thing.", Schema: json.RawMessage(`{"type": "object"}`)},
		func(context.Context, *tool.Env, struct{}) (tool.Result, error) {
			return tool.Result{Text: "saved"}, nil
		})
}

// resultOf is the text of a call's result.
func resultOf(t *testing.T, ms []chat.Message, call string) string {
	t.Helper()
	for _, m := range ms {
		for _, p := range m.Parts {
			if p.ToolResult != nil && p.ToolResult.CallID == call {
				return p.ToolResult.Text
			}
		}
	}
	t.Fatalf("no result for %s", call)
	return ""
}

// chips are the chat's skill_loaded notices.
func chips(ms []chat.Message) []string {
	var out []string
	for _, m := range ms {
		for _, p := range m.Parts {
			if p.Notice != nil && p.Notice.Kind == chat.NoticeSkillLoaded {
				out = append(out, p.Notice.Text)
			}
		}
	}
	return out
}

func (h *harness) chat(c id.Chat) chat.Chat {
	h.t.Helper()
	var ch chat.Chat
	h.open(func(p *project.Project) {
		var err error
		if ch, err = p.Chats.Chat(context.Background(), c); err != nil {
			h.t.Fatal(err)
		}
	})
	return ch
}

func (h *harness) setSkills(c id.Chat, skills ...string) {
	h.t.Helper()
	h.open(func(p *project.Project) {
		if _, err := p.Chats.SetSkills(context.Background(), c, skills); err != nil {
			h.t.Fatal(err)
		}
	})
}

func TestLoadSkill(t *testing.T) {
	h := newHarness(t, t.TempDir(), "", testSettings(), loadSkill())
	defer h.stop()
	h.o.d.Skills = skillSet(t, sqlSkill, viewsSkill, motherOnly)
	c := h.newChat(chat.Chat{Title: "Prices", Role: "a data analyst"})
	load := func(call, name, file string) fake.Reply {
		return fake.ToolCall(call, "load_skill", map[string]string{"name": name, "file": file})
	}
	h.fp.Push(load("c1", "sql", ""), load("c2", "sql", ""), load("c3", "nope", ""), load("c4", "delegation", ""),
		load("c5", "sql", "recipes.md"), load("c6", "sql", "more.md"), load("c7", "views", "a.md"), fake.Text("done"))
	h.turn(c.ID, "write SQL")
	ms := h.messages(c.ID)
	for call, want := range map[string]string{
		"c1": "Skill loaded: sql\n\nUse named parameters.\n\nExtra files, read with load_skill(\"sql\", file): recipes.md",
		"c2": "The skill sql is already loaded in this chat.",
		"c3": `there is no skill named "nope"; the skills are: sql, views`,
		"c4": `there is no skill named "delegation"; the skills are: sql, views`,
		"c5": "Recipe one.",
		"c6": `the skill sql has no file "more.md"; its files are: recipes.md`,
		"c7": "the skill views has no extra files",
	} {
		if got := resultOf(t, ms, call); got != want {
			t.Errorf("%s: %q, want %q", call, got, want)
		}
	}
	if got := h.chat(c.ID).Skills; !slices.Equal(got, []string{"sql"}) {
		t.Errorf("chats.skills = %q", got)
	}
	if got := chips(ms); !slices.Equal(got, []string{"Skill loaded: sql"}) {
		t.Errorf("chips %q", got)
	}
	validHistory(t, ms)

	// Block 1: the role, then the skill list; no skill text before a cut.
	sys := h.fp.Calls()[0].System[0].Text
	list := "## Skills\n"
	if i, j := strings.Index(sys, "a data analyst"), strings.Index(sys, list); i < 0 || j < i ||
		!strings.HasSuffix(sys, "\n- sql: SQL. Load to write SQL.\n- views: Views. Load before save_thing.") ||
		strings.Contains(sys, "delegation") || strings.Contains(sys, "Skill loaded") {
		t.Errorf("block 1:\n%s", sys)
	}

	// Mother lists her own skill.
	h.fp.Push(fake.Text("hi"))
	h.turn(h.mother().ID, "hello")
	if sys := h.fp.Calls()[len(h.fp.Calls())-1].System[0].Text; !strings.Contains(sys, "\n- delegation: Delegation.") {
		t.Errorf("Mother's block 1:\n%s", sys)
	}
}

// A loaded skill moves into block 1 at the next cut, after the role and
// before the skill list; until then block 1 stays the same.
func TestSkillMovesIntoBlock1AtTheCut(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, t.TempDir(), "", testSettings(), loadSkill())
		defer h.stop()
		h.o.d.Skills = skillSet(t, sqlSkill, viewsSkill)
		c := h.newChat(chat.Chat{Title: "Prices", Role: "a data analyst"})
		h.fp.Push(fake.ToolCall("c1", "load_skill", map[string]string{"name": "sql"}), fake.Text("loaded"), fake.Text("two"), fake.Text("three"))
		h.turn(c.ID, "one")
		h.turn(c.ID, "two")
		calls := h.fp.Calls()
		if jsonOf(t, calls[2].System) != jsonOf(t, calls[0].System) {
			t.Errorf("block 1 changed between cuts:\n%s", calls[2].System[0].Text)
		}
		time.Sleep(cacheIdle + time.Second)
		h.turn(c.ID, "three")
		sys := h.fp.Calls()[3].System[0].Text
		role, body, list := strings.Index(sys, "a data analyst"), strings.Index(sys, "\n\nSkill loaded: sql\n\nUse named parameters."), strings.Index(sys, "## Skills")
		if role < 0 || body < role || list < body {
			t.Errorf("after the cut block 1 is:\n%s", sys)
		}
	})
}

// load_with loads the skill with the tool's first call in the chat, but
// not a skill the chat has already loaded.
func TestLoadWith(t *testing.T) {
	h := newHarness(t, t.TempDir(), "", testSettings(), loadSkill(), saveThing())
	defer h.stop()
	sql := sqlSkill
	sql.LoadWith = []string{"save_thing"}
	h.o.d.Skills = skillSet(t, sql, viewsSkill)
	c := h.newChat(chat.Chat{Title: "Prices"})
	h.fp.Push(fake.ToolCall("c1", "load_skill", map[string]string{"name": "sql"}), fake.ToolCall("c2", "save_thing", map[string]string{}),
		fake.ToolCall("c3", "save_thing", map[string]string{}), fake.Text("done"))
	h.turn(c.ID, "save it")
	ms := h.messages(c.ID)
	if got := resultOf(t, ms, "c2"); got != "saved\n\nSkill loaded: views\n\nViews have columns." {
		t.Errorf("first call: %q", got)
	}
	if got := resultOf(t, ms, "c3"); got != "saved" {
		t.Errorf("second call: %q", got)
	}
	if got := h.chat(c.ID).Skills; !slices.Equal(got, []string{"sql", "views"}) {
		t.Errorf("chats.skills = %q", got)
	}
	if got := chips(ms); !slices.Equal(got, []string{"Skill loaded: sql", "Skill loaded: views"}) {
		t.Errorf("chips %q", got)
	}
	validHistory(t, ms)
}

// A chat holds at most 6 skills or 10,000 tokens of them; load_with skips
// a skill over the limit without an error.
func TestSkillLimit(t *testing.T) {
	var ss []skill.Skill
	for _, n := range "abcdefg" {
		ss = append(ss, skill.Skill{Name: string(n), Description: "Small.", Body: "Small skill."})
	}
	for _, n := range []string{"big1", "big2", "big3", "big4"} {
		ss = append(ss, skill.Skill{Name: n, Description: "Big.", Body: strings.Repeat("abcd", 2900)}) // 2,900 tokens
	}
	ss = append(ss, skill.Skill{Name: "views", Description: "Views.", LoadWith: []string{"save_thing"}, Body: "Views have columns."})
	h := newHarness(t, t.TempDir(), "", testSettings(), loadSkill(), saveThing())
	defer h.stop()
	h.o.d.Skills = skillSet(t, ss...)

	// A removed skill doesn't count.
	c := h.newChat(chat.Chat{Title: "Many"})
	h.setSkills(c.ID, "gone", "a", "b", "c", "d", "e")
	h.fp.Push(fake.ToolCall("c1", "load_skill", map[string]string{"name": "f"}), fake.ToolCall("c2", "load_skill", map[string]string{"name": "g"}),
		fake.ToolCall("c3", "save_thing", map[string]string{}), fake.Text("done"))
	h.turn(c.ID, "load")
	ms := h.messages(c.ID)
	if got := resultOf(t, ms, "c1"); !strings.HasPrefix(got, "Skill loaded: f") {
		t.Errorf("sixth skill: %q", got)
	}
	if got, want := resultOf(t, ms, "c2"), "can't load g: a chat holds at most 6 skills or 10000 tokens of them, and this chat has loaded a, b, c, d, e, f"; got != want {
		t.Errorf("seventh skill: %q", got)
	}
	if got := resultOf(t, ms, "c3"); got != "saved" {
		t.Errorf("load_with over the limit: %q", got)
	}
	if got := h.chat(c.ID).Skills; !slices.Equal(got, []string{"gone", "a", "b", "c", "d", "e", "f"}) {
		t.Errorf("chats.skills = %q", got)
	}

	// Tokens: three big skills fit, a fourth doesn't.
	c2 := h.newChat(chat.Chat{Title: "Big"})
	h.setSkills(c2.ID, "big1", "big2")
	// Block 1 has two big skills, so the replies report a large input.
	h.fp.Push(read(fake.ToolCall("d1", "load_skill", map[string]string{"name": "big3"}), 9000),
		read(fake.ToolCall("d2", "load_skill", map[string]string{"name": "big4"}), 12000), read(fake.Text("done"), 12000))
	h.turn(c2.ID, "load")
	ms = h.messages(c2.ID)
	if got := resultOf(t, ms, "d1"); !strings.HasPrefix(got, "Skill loaded: big3") || len(got) < 4*2900 {
		t.Errorf("third big skill: %d bytes", len(got))
	}
	if got := resultOf(t, ms, "d2"); !strings.HasPrefix(got, "can't load big4") || !strings.HasSuffix(got, "loaded big1, big2, big3") {
		t.Errorf("fourth big skill: %q", got)
	}
}

// A result that carries a skill from load_with isn't trimmed in its turn,
// so the skill stays readable until it moves into block 1.
func TestLoadWithResultIsNotTrimmed(t *testing.T) {
	set := testSettings()
	set.LLM.Providers["p"].Models[0].Context = 7600 // trims past 3,800 tokens
	h := newHarness(t, t.TempDir(), "", set, big())
	defer h.stop()
	h.o.d.Skills = skillSet(t, skill.Skill{Name: "data", Description: "Data.", LoadWith: []string{"big"}, Body: "Read refs in parts."})
	c := h.newChat(chat.Chat{Title: "Prices"})
	for i := range 4 {
		h.fp.Push(read(fake.ToolCall(fmt.Sprint("c", i), "big", map[string]int{"n": 20000}), 10000))
	}
	h.fp.Push(read(fake.Text("done"), 10000))
	h.turn(c.ID, "read four")
	last := h.fp.Calls()[4]
	var stubs []string
	for _, m := range last.Messages {
		for _, p := range m.Parts {
			if r := p.ToolResult; r != nil && !strings.Contains(r.Text, "\n") {
				stubs = append(stubs, r.CallID)
			}
			if r := p.ToolResult; r != nil && r.CallID == "c0" && !strings.HasSuffix(r.Text, "Skill loaded: data\n\nRead refs in parts.") {
				t.Errorf("c0: %q", r.Text)
			}
		}
	}
	if !slices.Equal(stubs, []string{"c1"}) {
		t.Errorf("stubs %q, want c1 only", stubs)
	}
}

func TestLoadSkillWithoutSkill(t *testing.T) {
	_, err := tool.Run(context.Background(), loadSkill(), tool.Call{ID: "c1", Args: json.RawMessage(`{"name": "sql"}`), Env: &tool.Env{}})
	if err == nil || err.Error() != "skills can't be loaded here" {
		t.Errorf("err = %v", err)
	}
}

// Loading a skill writes the chat's skills and the chip; the frontend
// sees every sequence number, and each part has its turn.
func TestSkillEventsHaveNoGap(t *testing.T) {
	h := newHarness(t, t.TempDir(), "", testSettings(), loadSkill())
	defer h.stop()
	h.o.d.Skills = skillSet(t, sqlSkill)
	c := h.newChat(chat.Chat{Title: "Prices"})
	h.fp.Push(fake.ToolCall("c1", "load_skill", map[string]string{"name": "sql"}), fake.Text("done"))
	h.turn(c.ID, "write SQL")
	h.pub.mu.Lock()
	parts := slices.Clone(h.pub.parts)
	h.pub.mu.Unlock()
	for i, p := range parts {
		if p.Turn != 1 {
			t.Errorf("part %d: turn %d", i, p.Turn)
		}
		if i > 0 && p.Seq > parts[i-1].Seq+1 {
			t.Errorf("part %d: seq %d after %d", i, p.Seq, parts[i-1].Seq)
		}
	}
	if s := parts[len(parts)-1].Seq; s != h.o.Live(h.pid, c.ID).Seq {
		t.Errorf("last part seq %d, the chat's %d", s, h.o.Live(h.pid, c.ID).Seq)
	}
}

// A turn continued by Retry still keeps the result that carries a skill
// from load_with whole.
func TestLoadWithResultIsNotTrimmedAfterRetry(t *testing.T) {
	set := testSettings()
	set.LLM.Providers["p"].Models[0].Context = 7600 // trims past 3,800 tokens
	h := newHarness(t, t.TempDir(), "", set, big())
	defer h.stop()
	h.o.d.Skills = skillSet(t, skill.Skill{Name: "data", Description: "Data.", LoadWith: []string{"big"}, Body: "Read refs in parts."})
	c := h.newChat(chat.Chat{Title: "Prices"})
	for i := range 4 {
		h.fp.Push(read(fake.ToolCall(fmt.Sprint("c", i), "big", map[string]int{"n": 20000}), 10000))
	}
	h.fp.Push(fake.Fail(&provider.Error{Kind: provider.BadRequest, Provider: "p", Status: 400, Message: "bad"}))
	h.turn(c.ID, "read four")
	h.fp.Push(read(fake.Text("done"), 10000))
	if err := h.o.Retry(context.Background(), h.pid, c.ID); err != nil {
		t.Fatal(err)
	}
	h.wait()
	calls := h.fp.Calls()
	for _, m := range calls[len(calls)-1].Messages {
		for _, p := range m.Parts {
			if r := p.ToolResult; r != nil && r.CallID == "c0" && !strings.HasSuffix(r.Text, "Skill loaded: data\n\nRead refs in parts.") {
				t.Errorf("c0 after Retry: %q", r.Text)
			}
		}
	}
}
