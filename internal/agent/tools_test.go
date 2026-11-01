package agent

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/provider/fake"
	"github.com/amir-saatchi/jenab/internal/store"
	"github.com/amir-saatchi/jenab/internal/tool"
)

// waiting is a tool that waits for w.
func waiting(name string, w tool.When) tool.Tool {
	return tool.Func(tool.Spec{Name: name, Description: "Waits.", Schema: json.RawMessage(`{"type": "object"}`), When: w},
		func(context.Context, *tool.Env, struct{}) (tool.Result, error) {
			return tool.Result{Text: name + " ran"}, nil
		})
}

// A tool that waits for a table joins at the migration that makes one, in
// the same turn; one that waits for history or another chat joins at the
// cut that has them. A tool left out still runs when called.
func TestToolsWhenNeeded(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		set := testSettings()
		set.Context.HistoryMinTurns = 1
		h := newHarness(t, t.TempDir(), "", set, append(cardTools(),
			waiting("rows", tool.WhenTables), waiting("past", tool.WhenHistory), waiting("chats", tool.WhenChats))...)
		defer h.stop()
		c := h.newChat(chat.Chat{Title: "Prices"})
		h.fp.Push(fake.ToolCall("c1", "past", struct{}{}), fake.ToolCall("c2", "migrate", struct{}{}), fake.Text("done"))
		h.turn(c.ID, "make a table")
		calls := h.fp.Calls()
		want := [][]string{{"add", "migrate"}, {"add", "migrate"}, {"add", "migrate", "rows"}}
		for i, req := range calls {
			if got := toolNames(req); !slices.Equal(got, want[i]) {
				t.Errorf("request %d has %q, want %q", i+1, got, want[i])
			}
		}
		if tx := texts(h.messages(c.ID)); !slices.Contains(tx, "tool: result past ran") {
			t.Errorf("the left-out tool didn't run: %q", tx)
		}

		// A cut with turn 1 before the window, and another chat.
		h.newChat(chat.Chat{Title: "Other"})
		h.fp.Push(fake.Text("two"), fake.Text("three"))
		h.turn(c.ID, "two")
		if got := toolNames(h.fp.Calls()[3]); !slices.Equal(got, []string{"add", "migrate", "rows"}) {
			t.Errorf("without a cut: %q", got)
		}
		time.Sleep(cacheIdle + time.Second)
		h.turn(c.ID, "three")
		if got := toolNames(h.fp.Calls()[4]); !slices.Equal(got, []string{"add", "chats", "migrate", "past", "rows"}) {
			t.Errorf("after the cut: %q", got)
		}
	})
}

// A bucket tool joins at the cut after the bucket has an object outside
// cache/: cached pages don't count.
func TestBucketToolsWhenNeeded(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, t.TempDir(), "", testSettings(), waiting("objects", tool.WhenBucket))
		defer h.stop()
		c := h.newChat(chat.Chat{Title: "Files"})
		put := func(key string) {
			h.open(func(p *project.Project) {
				if _, err := p.DB.PutObject(t.Context(), limit.Interactive, id.SourceUser, key, strings.NewReader("text"), store.PutOptions{Source: "upload"}); err != nil {
					t.Fatal(err)
				}
			})
		}
		put("cache/pages/example.test/a.txt")
		for i, want := range []bool{false, true} {
			h.fp.Push(fake.Text("ok"))
			h.turn(c.ID, "hi")
			calls := h.fp.Calls()
			if got := slices.Contains(toolNames(calls[len(calls)-1]), "objects"); got != want {
				t.Errorf("turn %d: objects offered %v, want %v", i+1, got, want)
			}
			put("notes/a.txt")
			time.Sleep(cacheIdle + time.Second)
		}
	})
}
