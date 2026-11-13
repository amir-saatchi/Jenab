package main

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/chat"
)

// Every tool the harness registers is in exactly one group, always on, or
// Mother's, and every grouped or always-on tool is registered.
func TestGroupsCoverRegistry(t *testing.T) {
	r := &run{w: newWorld(), j: job{Cond: condGroups}}
	var names []string
	for _, tl := range r.registry().For(chat.KindMother) {
		names = append(names, tl.Spec().Name)
	}
	for _, n := range names {
		in := 0
		for _, g := range groups {
			if slices.Contains(g.Tools, n) {
				in++
			}
		}
		if slices.Contains(alwaysOn, n) || slices.Contains(motherTools, n) {
			in++
		}
		if in != 1 {
			t.Errorf("%s is in %d places", n, in)
		}
	}
	for _, n := range append(groupTools(), alwaysOn...) {
		if !slices.Contains(names, n) {
			t.Errorf("%s isn't registered", n)
		}
		if groupOf(n) != "" && toolLines[n] == "" {
			t.Errorf("%s has no line for condition 3", n)
		}
	}
}

func TestToolsOf(t *testing.T) {
	ts := toolsOf([]string{"read_data", "bucket_put", "nonsense"})
	for _, n := range []string{"query", "describe_table", "bucket_put", "ask_user"} {
		if !slices.Contains(ts, n) {
			t.Errorf("toolsOf lacks %s: %v", n, ts)
		}
	}
	if slices.Contains(ts, "insert") || slices.Contains(ts, "nonsense") {
		t.Errorf("toolsOf has too much: %v", ts)
	}
	if got := groupsOf(ts); !slices.Equal(got, []string{"read_data", "files"}) {
		t.Errorf("groupsOf = %v", got)
	}
	m := motherToolList()
	for _, n := range []string{"insert", "save_pipeline", "web_search", "request_schema_change"} {
		if slices.Contains(m, n) {
			t.Errorf("Mother has %s", n)
		}
	}
}

func TestRefusal(t *testing.T) {
	msg := refusal(false)("insert")
	if !strings.Contains(msg, "Change data") || !strings.Contains(msg, refusedMark) || !strings.Contains(msg, "Tell the user") {
		t.Errorf("agent refusal: %s", msg)
	}
	if msg := refusal(true)("save_pipeline"); !strings.Contains(msg, "Views and pipelines") || !strings.Contains(msg, "send_to_chat") {
		t.Errorf("Mother's refusal: %s", msg)
	}
}

func TestCreateChatSchema(t *testing.T) {
	for _, single := range []bool{false, true} {
		var s struct {
			Properties struct {
				Tools struct {
					Items struct{ Enum []string }
				}
			}
		}
		if err := json.Unmarshal(createChatSchema(single), &s); err != nil {
			t.Fatal(err)
		}
		want := len(groups)
		if single {
			want = len(groupTools())
		}
		if got := len(s.Properties.Tools.Items.Enum); got != want {
			t.Errorf("single %v: %d names, want %d", single, got, want)
		}
		// The registry compiles the schema; it panics on a bad one.
		(&run{w: newWorld(), j: job{Cond: map[bool]int{false: condGroups, true: condSingle}[single]}}).registry()
	}
}

func TestScenarios(t *testing.T) {
	if len(scenarios) != 7 {
		t.Fatalf("%d scenarios", len(scenarios))
	}
	for _, sc := range scenarios {
		for _, g := range append(append(slices.Clone(sc.Need), sc.Forbid...), sc.KeyGroups...) {
			if _, ok := groupByName(g); !ok {
				t.Errorf("%s: unknown group %s", sc.ID, g)
			}
		}
		for _, g := range sc.Need {
			if !slices.Contains(sc.KeyGroups, g) {
				t.Errorf("%s: the key lacks needed group %s", sc.ID, g)
			}
		}
		if roleTokens(sc.KeyRole) > maxRoleTokens {
			t.Errorf("%s: the key's role is too long", sc.ID)
		}
	}
}
