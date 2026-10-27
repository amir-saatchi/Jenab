package tool

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/store"
)

func notesArgsJSON(content string, rev int) string {
	return fmt.Sprintf(`{"content": %q, "expected_revision": %d}`, content, rev)
}

// update_session_notes saves, refuses a stale revision with the notes as
// they are now, empties them with empty content and keeps to the cap.
func TestUpdateSessionNotes(t *testing.T) {
	ctx := context.Background()
	env := testEnv(t)
	if got := mustText(t, env, "update_session_notes", notesArgsJSON("Plan:\n1. import\n2. chart", 0)); got != "Saved the session notes, revision 1." {
		t.Fatalf("save: %q", got)
	}
	contains(t, toolError(t, env, "update_session_notes", notesArgsJSON("Plan: other", 0)),
		"changed since you read them. They are at revision 1 now, with this content:\nPlan:\n1. import\n2. chart\n", "expected_revision 1")
	if got := mustText(t, env, "update_session_notes", notesArgsJSON("Plan:\n1. import\n2. chart", 1)); !strings.HasPrefix(got, "No change:") {
		t.Errorf("same text: %q", got)
	}

	// The cap counts the summary too.
	sum := strings.Repeat("s", 4*store.MaxSummaryTokens)
	if _, err := env.Project.Chats.SaveSummary(ctx, env.Chat, sum, 1); err != nil {
		t.Fatal(err)
	}
	over := strings.Repeat("abcd", store.MaxNotesTokens-store.MaxSummaryTokens+1)
	contains(t, toolError(t, env, "update_session_notes", notesArgsJSON(over, 2)), "2,501 tokens with the summary, over their cap of 2,500")

	if got := mustText(t, env, "update_session_notes", notesArgsJSON("", 2)); got != "Emptied the session notes, now at revision 3." {
		t.Errorf("empty: %q", got)
	}
	n, err := env.Project.Chats.Notes(ctx, env.Chat)
	if err != nil || n.Content != "" || n.Summary != sum || n.Revision != 3 {
		t.Errorf("after emptying: %+v, %v", n, err)
	}
	contains(t, toolError(t, env, "update_session_notes", notesArgsJSON("x", 1)), "are empty now, at revision 3")
}

// The summary section is Refresh memory's: a changed copy is refused, a
// copy as shown is dropped, and the notes as shown in block 5 come back
// as they were, with lines in brackets their own.
func TestUpdateSessionNotesSummary(t *testing.T) {
	ctx := context.Background()
	env := testEnv(t)
	if _, err := env.Project.Chats.SaveSummary(ctx, env.Chat, "Imported prices.\n[done]", 0); err != nil {
		t.Fatal(err)
	}
	for _, content := range []string{
		"[summary, written by Refresh memory]\nImported prices and charts.\n\n[notes]\nPlan: chart.",
		"[Summary]\nMy own summary.\n[notes]\nPlan: chart.",
		"Plan: chart.\n[summary]\nImported prices.",
	} {
		contains(t, toolError(t, env, "update_session_notes", notesArgsJSON(content, 1)), "the summary section is written by Refresh memory, not by you")
	}
	if n, _ := env.Project.Chats.Notes(ctx, env.Chat); n.Revision != 1 {
		t.Fatalf("a refused edit saved: revision %d", n.Revision)
	}

	notes := "Plan:\n[notes]\n[step 1] import\n[notes]? no, a line of mine"
	block := NotesBlock(chat.SessionNote{Content: notes, Summary: "Imported prices.\n[done]", Revision: 1})
	want := "Session notes of this chat, revision 1:\n\n[summary, written by Refresh memory]\nImported prices.\n\\[done]\n\n[notes]\nPlan:\n\\[notes]\n[step 1] import\n[notes]? no, a line of mine"
	if block != want {
		t.Fatalf("block:\n%s\nwant:\n%s", block, want)
	}
	// A model that copies the whole block back, with its first line
	// dropped and spaces at line ends, saves the notes only.
	copied := strings.ReplaceAll(strings.SplitN(block, "\n", 2)[1], "\n", "  \n")
	if got := mustText(t, env, "update_session_notes", notesArgsJSON(copied, 1)); got != "Saved the session notes, revision 2." {
		t.Fatalf("copied block: %q", got)
	}
	n, err := env.Project.Chats.Notes(ctx, env.Chat)
	if err != nil || n.Summary != "Imported prices.\n[done]" || strings.ReplaceAll(n.Content, "  \n", "\n") != notes {
		t.Fatalf("after the copy: %q / %q, %v", n.Content, n.Summary, err)
	}

	for _, c := range []struct {
		n    chat.SessionNote
		want string
	}{
		{chat.SessionNote{}, ""},
		{chat.SessionNote{Revision: 4}, "Session notes of this chat, revision 4: empty."},
		{chat.SessionNote{Content: "a", Revision: 2}, "Session notes of this chat, revision 2:\n\n[notes]\na"},
	} {
		if got := NotesBlock(c.n); got != c.want {
			t.Errorf("NotesBlock(%+v) = %q, want %q", c.n, got, c.want)
		}
	}
}

// splitNotes reads back what a model copies from block 5: the block's
// own lines go, also reworded or in Persian, and the stored summary is
// no edit wherever it is copied, spaces and blank lines aside. The
// model's own lines in brackets stay.
func TestSplitNotes(t *testing.T) {
	const sum = "Imported prices.\n[done]"
	const fa = "قیمت‌ها وارد شد.\n\nنمودار مانده است."
	for _, c := range []struct {
		name, stored, in, want string
		edits                  bool
	}{
		{"whole block", sum, NotesBlock(chat.SessionNote{Content: "Plan: chart.", Summary: sum, Revision: 3}), "Plan: chart.", false},
		{"first line twice", "", "Session notes of this chat, revision 2:\n\n[notes]\nPlan.\nSession notes of this chat, revision 2:", "Plan.", false},
		{"empty block", "", "Session notes of this chat, revision 4: empty.\nPlan: chart.", "Plan: chart.", false},
		{"reworded header", sum, "[Summary]\nImported prices.\n\\[done]\n[notes]\nPlan.", "Plan.", false},
		{"header without comma", sum, "[summary written by Refresh memory]\nImported prices.\n[done]\n\nPlan.", "Plan.", false},
		{"Persian header", fa, "[خلاصه، نوشتهٔ Refresh memory]\n" + fa + "\n\n[notes]\nبرنامه: نمودار.", "برنامه: نمودار.", false},
		{"Persian summary without header", fa, fa + "\n\nبرنامه: نمودار.", "برنامه: نمودار.", false},
		{"summary after notes text", fa, "برنامه: نمودار.\n\n" + fa, "برنامه: نمودار.\n\n" + fa, false},
		{"summary under an own header", sum, "Plan.\n\n[earlier]\nImported prices.\n[done]\n\nMore.", "Plan.\n\n[earlier]\nImported prices.\n[done]\n\nMore.", false},
		{"own line like a short summary", "Imported prices.", "Plan:\n- Imported prices.\nImported prices.\n- next: chart", "Plan:\n- Imported prices.\nImported prices.\n- next: chart", false},
		{"own bracket line before it", "Imported prices.", "[Done]\nImported prices.\n[Next]\nchart", "[Done]\nImported prices.\n[Next]\nchart", false},
		{"own summary heading", sum, "[Summary of the meeting]\nWe chose Calvino.", "[Summary of the meeting]\nWe chose Calvino.", false},
		{"own summary heading, none stored", "", "Plan.\n[Summary of the meeting]\nWe chose Calvino.", "Plan.\n[Summary of the meeting]\nWe chose Calvino.", false},
		{"summary with a quoted line", "Imported.\n\\[done]", NotesBlock(chat.SessionNote{Content: "Plan.", Summary: "Imported.\n\\[done]", Revision: 2}), "Plan.", false},
		{"collapsed blank line", fa, "[summary, written by Refresh memory]\nقیمت‌ها   وارد شد.\nنمودار مانده است.\n[notes]\nبرنامه.", "برنامه.", false},
		{"own bracket lines", sum, "[notes, todo]\n- chart\n[notes]\n- check", "[notes, todo]\n- chart\n[notes]\n- check", false},
		{"indent kept", "", "[notes]\n  - import\n  - chart", "  - import\n  - chart", false},
		{"changed summary", sum, "[summary, written by Refresh memory]\nImported prices and charts.\n[notes]\nPlan.", "Plan.", true},
		{"Persian summary changed", fa, "[خلاصه]\nقیمت‌ها وارد شد.\n[notes]\nبرنامه.", "برنامه.", true},
		{"summary heading, none stored", "", "[summary]\nMy own.", "[summary]\nMy own.", false},
	} {
		got, edits := splitNotes(c.in, c.stored)
		if got != c.want || edits != c.edits {
			t.Errorf("%s: got %q, %v; want %q, %v", c.name, got, edits, c.want, c.edits)
		}
	}
}

// A model that copies block 5 whole, first line included, saves the
// notes only, and again the same block is no change.
func TestUpdateSessionNotesCopiedBlock(t *testing.T) {
	ctx := context.Background()
	env := testEnv(t)
	if _, err := env.Project.Chats.SaveSummary(ctx, env.Chat, "Imported prices.", 0); err != nil {
		t.Fatal(err)
	}
	mustText(t, env, "update_session_notes", notesArgsJSON("Plan: chart.", 1))
	n, err := env.Project.Chats.Notes(ctx, env.Chat)
	if err != nil {
		t.Fatal(err)
	}
	block := NotesBlock(n)
	if got := mustText(t, env, "update_session_notes", notesArgsJSON(block, 2)); !strings.HasPrefix(got, "No change:") {
		t.Errorf("same block: %q", got)
	}
	if got := mustText(t, env, "update_session_notes", notesArgsJSON(block+"\nNext: check.", 2)); got != "Saved the session notes, revision 3." {
		t.Fatalf("block and a line: %q", got)
	}
	if n, err := env.Project.Chats.Notes(ctx, env.Chat); err != nil || n.Content != "Plan: chart.\nNext: check." || n.Summary != "Imported prices." {
		t.Errorf("after: %+v, %v", n, err)
	}
}
