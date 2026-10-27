package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/store"
)

// update_session_notes (SPEC 3.4, 8.1). The notes are block 5 of the
// chat's context (3.1); NotesBlock writes it, and the tool reads back
// what a model copies from it. The description gives no number for the
// cap: the number check counts tool descriptions as sources, and a 2,500
// there would source a made-up 2,470.10 (SPIKE-032); a refusal gives it.
// Subagents don't get the tool: it has the Memory effect, like
// update_memory.

const notesDescription = `Edit this chat's session notes. They are part of this chat's context, and only this chat's: the current task, the plan and how far it got, open questions, and decisions not yet confirmed. They stay when older turns leave the context, so keep them current as the work goes on.
- Where things go: would a new chat in this project need it? Then project memory (update_memory). Only this conversation? Then session notes.
- content replaces the whole notes, so keep what should stay. Empty content empties them.
- The summary section is written by Refresh memory, not by you: leave it out of content, and it stays as it is.
- expected_revision is the notes' revision as shown, 0 if there are none yet. If they changed since, the edit is refused with their current content: merge your edit into it and call again with its revision.
- Keep them short: they have a size cap, with the summary, and an edit over it is refused.`

// The section headers of block 5. A line of the notes that looks like one
// is quoted (store.QuoteMemory), so it can't pass for one.
const (
	summaryHeader = "[summary, written by Refresh memory]"
	notesHeader   = "[notes]"
)

type notesArgs struct {
	Content          string `json:"content"`
	ExpectedRevision int    `json:"expected_revision"`
}

// UpdateSessionNotes is the update_session_notes tool.
func UpdateSessionNotes() Tool {
	return Func(Spec{
		Name:        "update_session_notes",
		Description: notesDescription,
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"content": {"type": "string", "description": "the whole new text of the notes, without the summary section; empty empties them"},
				"expected_revision": {"type": "integer", "minimum": 0, "description": "the notes' revision as shown; 0 if there are none yet"}
			},
			"required": ["content", "expected_revision"],
			"additionalProperties": false
		}`),
		Effects: Memory,
	}, runUpdateNotes)
}

func runUpdateNotes(ctx context.Context, env *Env, a notesArgs) (Result, error) {
	cur, err := env.Project.Chats.Notes(ctx, env.Chat)
	if err != nil {
		return Result{}, err
	}
	content, editsSummary := splitNotes(a.Content, cur.Summary)
	if cur.Revision == a.ExpectedRevision {
		// A summary copied back as it is shown is no edit of it; any
		// other is refused, as only Refresh memory writes it (3.4).
		if editsSummary {
			return Result{}, &Error{Msg: "not saved: the summary section is written by Refresh memory, not by you. Leave it out of content, and put what you meant to add there in the notes instead."}
		}
		if content == cur.Content {
			return Result{Text: fmt.Sprintf("No change: the session notes already have this text (revision %d).", cur.Revision)}, nil
		}
	}
	n, err := env.Project.Chats.SaveNotes(ctx, chat.SessionNote{Chat: env.Chat, Content: content, Revision: a.ExpectedRevision})
	if err != nil {
		return Result{}, notesError(err)
	}
	if n.Content == "" {
		return Result{Text: fmt.Sprintf("Emptied the session notes, now at revision %d.", n.Revision)}, nil
	}
	return Result{Text: fmt.Sprintf("Saved the session notes, revision %d.", n.Revision)}, nil
}

// notesError turns what the model can fix into an *Error: a stale
// revision with the notes as they are now, the cap.
func notesError(err error) error {
	var ce *store.NotesConflictError
	var cap *store.NotesCapError
	switch {
	case errors.As(err, &ce):
		c := ce.Current
		if c.Content == "" {
			return &Error{Msg: fmt.Sprintf("the session notes changed since you read them, and are empty now, at revision %d. Call update_session_notes again with expected_revision %d.",
				c.Revision, c.Revision), Err: err}
		}
		return &Error{Msg: fmt.Sprintf("the session notes changed since you read them. They are at revision %d now, with this content:\n%s\nMerge your edit into it and call update_session_notes again with expected_revision %d.",
			c.Revision, store.QuoteMemory(c.Content), c.Revision), Err: err}
	case errors.As(err, &cap):
		return &Error{Msg: fmt.Sprintf("not saved: the session notes would be %s tokens with the summary, over their cap of %s. Make them shorter: keep what the work still needs, and drop what is done.",
			count(cap.Tokens), count(cap.Cap)), Err: err}
	case errors.Is(err, store.ErrReadOnly):
		return &Error{Msg: "the project is open read-only after damage, so nothing can be changed; tell the user", Err: err}
	}
	return err
}

// NotesBlock is block 5 of the context (3.1): the chat's session notes,
// with their revision, the summary first. A chat without notes has no
// block.
func NotesBlock(n chat.SessionNote) string {
	if n.Revision == 0 {
		return ""
	}
	if n.Content == "" && n.Summary == "" {
		return fmt.Sprintf("Session notes of this chat, revision %d: empty.", n.Revision)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Session notes of this chat, revision %d:", n.Revision)
	if n.Summary != "" {
		b.WriteString("\n\n" + summaryHeader + "\n" + store.QuoteMemory(n.Summary))
	}
	if n.Content != "" {
		b.WriteString("\n\n" + notesHeader + "\n" + store.QuoteMemory(n.Content))
	}
	return b.String()
}

// splitNotes reads content as a model writes it, perhaps copied from
// block 5, and returns the notes, unquoted. The block's own lines are
// dropped: its first line, its summary header (also reworded or in
// Persian), and the [notes] header where it starts the notes. Other lines
// in brackets are the model's own, such as "[notes, todo]" or "[Summary
// of the meeting]", and stay. A summary section is no edit when its text
// is the stored summary's, spaces and blank lines aside; lines after that
// go to the notes. Any other text there edits the summary: editsSummary.
// With no summary stored, the section is the model's own and stays. The
// stored summary copied without a header before any notes text is dropped
// too, so it isn't saved twice; a copy further on is the model's to keep.
func splitNotes(s, stored string) (notes string, editsSummary bool) {
	// The block shows the summary quoted; normLines unquotes it once.
	want := normLines(store.QuoteMemory(stored))
	var ns, ss []string
	var head string
	inSummary := false
	closeSummary := func() {
		if !inSummary {
			return
		}
		inSummary = false
		if end, ok := matchLines(ss, 0, want); ok {
			ns = append(ns, ss[end:]...)
		} else if len(want) == 0 {
			ns = append(append(ns, head), ss...)
		} else if hasText(ss) {
			editsSummary = true
		}
		ss = nil
	}
	for _, l := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		t := strings.TrimSpace(l)
		switch {
		case blockLine.MatchString(t):
			continue
		case isSummaryHeader(t):
			closeSummary()
			inSummary, head = true, l
			continue
		case t == notesHeader && (inSummary || !hasText(ns[leadingCopy(ns, want):])):
			closeSummary()
			continue
		}
		if inSummary {
			ss = append(ss, l)
		} else {
			ns = append(ns, l)
		}
	}
	closeSummary()
	return store.UnquoteMemory(trimBlank(ns[leadingCopy(ns, want):])), editsSummary
}

// blockLine is block 5's first line.
var blockLine = regexp.MustCompile(`^Session notes of this chat, revision \d+:( empty\.)?$`)

// summaryName is the name in block 5's summary header, as a model may
// reword or translate it: "summary", "summary, …", "summary written by …"
// or the same in Persian.
var summaryName = regexp.MustCompile(`^(summary|خلاصه)(\s*[,،].*|\s+(written by|نوشته).*)?$`)

// isSummaryHeader: t is "[…]", not quoted, with summaryName in it.
func isSummaryHeader(t string) bool {
	if !strings.HasPrefix(t, "[") || !strings.HasSuffix(t, "]") {
		return false
	}
	return summaryName.MatchString(strings.ToLower(strings.TrimSpace(strings.Trim(t, "[]"))))
}

// hasText: some line is not blank.
func hasText(lines []string) bool {
	return len(normLines(strings.Join(lines, "\n"))) > 0
}

// leadingCopy is where lines go on after a copy of want before any other
// text, or 0 if they don't start with one.
func leadingCopy(lines, want []string) int {
	i := 0
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}
	if end, ok := matchLines(lines, i, want); ok {
		return end
	}
	return 0
}

// normLines are the lines of s, unquoted, with their spaces collapsed and
// blank lines left out, for comparing texts as a model copies them.
func normLines(s string) []string {
	var out []string
	for _, l := range strings.Split(store.UnquoteMemory(s), "\n") {
		if f := strings.Fields(l); len(f) > 0 {
			out = append(out, strings.Join(f, " "))
		}
	}
	return out
}

// matchLines reports whether lines from i, blank ones aside, start with
// want, and where the match ends. An empty want matches nothing.
func matchLines(lines []string, i int, want []string) (end int, ok bool) {
	if len(want) == 0 {
		return 0, false
	}
	k := 0
	for ; i < len(lines) && k < len(want); i++ {
		l := normLines(lines[i])
		if len(l) == 0 {
			continue
		}
		if l[0] != want[k] {
			return 0, false
		}
		k++
	}
	return i, k == len(want)
}

// trimBlank joins lines without the blank ones at the start and the end.
// A line's own indent stays.
func trimBlank(lines []string) string {
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}
