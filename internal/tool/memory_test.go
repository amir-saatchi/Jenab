package tool

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/store"
)

// memoryEnv is testEnv with a registry, for user memory.
func memoryEnv(t *testing.T) *Env {
	t.Helper()
	env := testEnv(t)
	reg, err := store.OpenRegistry(context.Background(), filepath.Join(t.TempDir(), "registry.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reg.Close() })
	env.Project.Registry = reg
	return env
}

func memArgs(scope, section, content string, rev int) string {
	return fmt.Sprintf(`{"scope": %q, "section": %q, "content": %q, "expected_revision": %d}`, scope, section, content, rev)
}

// update_memory saves, refuses a stale revision with the section as it is
// now, deletes with empty content, keeps to the caps and checks names, in
// both scopes. Each save brings the chip.
func TestUpdateMemory(t *testing.T) {
	ctx := context.Background()
	for _, scope := range []string{"user", "project"} {
		t.Run(scope, func(t *testing.T) {
			env := memoryEnv(t)
			r, err := call(t, env, "update_memory", memArgs(scope, "Preferences", "Short answers.", 0))
			if err != nil || r.Text != fmt.Sprintf("Saved %s memory, section preferences, revision 1.", scope) {
				t.Fatalf("save: %+v, %v", r, err)
			}
			if len(r.Notices) != 1 || r.Notices[0].Kind != chat.NoticeMemoryUpdated || r.Notices[0].Text != "Memory updated: preferences" ||
				r.Notices[0].Target != "memory:"+scope+"/preferences" || r.Notices[0].Change == "" {
				t.Errorf("chip %+v", r.Notices)
			}

			contains(t, toolError(t, env, "update_memory", memArgs(scope, "preferences", "In Persian.", 0)),
				"section preferences, changed since you read it. It is at revision 1 now, with this content:\nShort answers.\n",
				"expected_revision 1")
			if got := mustText(t, env, "update_memory", memArgs(scope, "preferences", "Short answers.", 1)); !strings.HasPrefix(got, "No change:") {
				t.Errorf("same text: %q", got)
			}

			r, err = call(t, env, "update_memory", memArgs(scope, "preferences", "", 1))
			if err != nil || r.Text != fmt.Sprintf("Deleted %s memory, section preferences, now at revision 2.", scope) || len(r.Notices) != 1 ||
				r.Notices[0].Text != "Memory updated: preferences (deleted)" {
				t.Fatalf("delete: %+v, %v", r, err)
			}
			if got := mustText(t, env, "update_memory", memArgs(scope, "preferences", "", 2)); !strings.HasPrefix(got, "No change:") {
				t.Errorf("delete again: %q", got)
			}
			contains(t, toolError(t, env, "update_memory", memArgs(scope, "preferences", "x", 1)), "was deleted at revision 2", "expected_revision 2")

			cap := store.MemoryScope(scope).Cap()
			big := strings.Repeat("a", cap*4)
			contains(t, toolError(t, env, "update_memory", memArgs(scope, "goals", big, 0)), "over its cap of "+count(cap))
			contains(t, toolError(t, env, "update_memory", memArgs(scope, "a b", "x", 0)), "a section name is 1 to 40 letters")

			var all []store.MemorySection
			if scope == "user" {
				all, err = env.Project.Registry.UserMemory(ctx)
			} else {
				all, err = env.Project.DB.Memory(ctx)
			}
			if err != nil || len(all) != 1 || all[0].Content != "" || all[0].Revision != 2 || all[0].Source != env.Source {
				t.Errorf("memory %+v, %v", all, err)
			}
		})
	}
}

// Content shown to the model is quoted (store.QuoteMemory), and what the
// model writes back is unquoted, so a merge from the conflict's text keeps
// the section's lines as they were.
func TestUpdateMemoryQuoted(t *testing.T) {
	ctx := context.Background()
	env := memoryEnv(t)
	stored := "[todo]\n" + `\[done]` + "\nShort answers."
	if _, err := env.Project.DB.UpdateMemory(ctx, store.MemoryEdit{Section: "notes", Content: stored, Source: "user"}); err != nil {
		t.Fatal(err)
	}
	shown := `\[todo]` + "\n" + `\\[done]` + "\nShort answers."
	contains(t, toolError(t, env, "update_memory", memArgs("project", "notes", "In Persian.", 0)), "with this content:\n"+shown+"\n")
	mustText(t, env, "update_memory", memArgs("project", "notes", shown+"\nIn Persian.", 1))
	all, err := env.Project.DB.Memory(ctx)
	if err != nil || len(all) != 1 || all[0].Content != stored+"\nIn Persian." {
		t.Errorf("memory %+v, %v", all, err)
	}
}
