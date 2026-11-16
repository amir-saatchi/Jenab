package logic

// Ported from the rows tests of the React app's state/thread.test.ts. Two of
// them build their messages with the thread store (withPart); here the
// messages are written as the store leaves them.

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
)

func msg(mid string, turn int, role chat.Role, texts ...string) chat.Message {
	m := chat.Message{ID: id.Message(mid), Chat: "c1", Turn: turn, Role: role, CreatedAt: day, Parts: []chat.Part{}}
	for _, t := range texts {
		m.Parts = append(m.Parts, chat.Part{Kind: chat.PartText, Text: &chat.Text{Text: t}})
	}
	return m
}

func withParts(m chat.Message, ps ...chat.Part) chat.Message {
	m.Parts = ps
	return m
}

func noticePart(k chat.NoticeKind, text string) chat.Part {
	return chat.Part{Kind: chat.PartNotice, Notice: &chat.Notice{Kind: k, Text: text}}
}

func rowKinds(rs []Row) []string {
	out := []string{}
	for _, r := range rs {
		out = append(out, string(r.Kind))
	}
	return out
}

// lastsOrNil is each row's Last, or "-" for a row that isn't the agent's.
func lastsOrNil(rs []Row) []string {
	out := []string{}
	for _, r := range rs {
		if r.Kind == RowAgent {
			out = append(out, fmt.Sprint(r.Last))
		} else {
			out = append(out, "-")
		}
	}
	return out
}

func rowIDs(rs []Row) []string {
	out := []string{}
	for _, r := range rs {
		out = append(out, r.ID)
	}
	return out
}

func eq(t *testing.T, what string, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s = %v, want %v", what, got, want)
	}
}

func TestRowsGroupTheAgentsWorkAndMarkTurnEnds(t *testing.T) {
	call := withParts(msg("a1", 1, chat.RoleAssistant), chat.Part{Kind: chat.PartToolCall, ToolCall: &chat.ToolCall{ID: "k1", Name: "query", Args: []byte("{}")}})
	tool := withParts(msg("t1", 1, chat.RoleTool), chat.Part{Kind: chat.PartToolResult, ToolResult: &chat.ToolResult{CallID: "k1", Text: "1 row"}})
	notice := withParts(msg("n1", 2, chat.RoleUser), noticePart(chat.NoticeTaskFinished, "done"))
	ms := []chat.Message{msg("u1", 1, chat.RoleUser, "go"), call, tool, msg("a2", 1, chat.RoleAssistant, "Done."), notice, msg("a3", 2, chat.RoleAssistant, "News")}
	rs := Rows(ms)
	eq(t, "kinds", rowKinds(rs), []string{"user", "agent", "notice", "agent"})
	eq(t, "lasts", lastsOrNil(rs), []string{"-", "true", "-", "true"})
	eq(t, "result", Results(rs[1].Messages)["k1"].Text, "1 row")
	eq(t, "copy", CopyText(rs[1].Messages), "Done.")
	if n := LastNotice(ms); n != nil {
		t.Errorf("LastNotice = %v, want nil", n)
	}
	if n := LastNotice(ms[:5]); n == nil || n.Kind != chat.NoticeTaskFinished {
		t.Errorf("LastNotice(first 5) = %v", n)
	}
}

func TestAnUndoNoticeKeepsTheTurnsFooter(t *testing.T) {
	undone := withParts(msg("n1", 1, chat.RoleUser), noticePart(chat.NoticeTurnUndone, "[turn 1 was undone]"))
	level := withParts(msg("n2", 1, chat.RoleUser), noticePart(chat.NoticeApprovalLevel, "[level]"))
	lasts := func(ms ...chat.Message) []bool {
		out := []bool{}
		for _, r := range Rows(ms) {
			if r.Kind == RowAgent {
				out = append(out, r.Last)
			}
		}
		return out
	}
	u1, a1 := msg("u1", 1, chat.RoleUser, "go"), msg("a1", 1, chat.RoleAssistant, "Done.")
	eq(t, "lasts", lasts(u1, a1, undone, level), []bool{true})
	eq(t, "kinds", rowKinds(Rows([]chat.Message{u1, a1, undone, level})), []string{"user", "agent", "notice", "notice"})
	// A message that joins the turn after the notice still takes the footer.
	eq(t, "lasts joined", lasts(u1, a1, undone, msg("u2", 1, chat.RoleUser, "more"), msg("a2", 1, chat.RoleAssistant, "Ok.")), []bool{false, true})
}

func TestHistoryCardsMarkIsNotShownAndKeepsTheTurnsFooter(t *testing.T) {
	mark := withParts(msg("n1", 1, chat.RoleUser), noticePart(chat.NoticeCardOutside, "[mark]"))
	ms := []chat.Message{msg("u1", 1, chat.RoleUser, "go"), mark, msg("a1", 1, chat.RoleAssistant, "Done.")}
	rs := Rows(ms)
	eq(t, "kinds", rowKinds(rs), []string{"user", "agent"})
	eq(t, "lasts", lastsOrNil(rs), []string{"-", "true"})
	mark2 := mark
	mark2.ID = "n2"
	eq(t, "lasts with a second mark", lastsOrNil(Rows(append(ms[:3:3], mark2))), []string{"-", "true"})
	if n := LastNotice(append(ms[:3:3], mark)); n != nil {
		t.Errorf("LastNotice = %v, want nil", n)
	}
}

func TestAChatOpensWithAboutTwoScreensOfRows(t *testing.T) {
	long := strings.Join(numbered(30, "line %d"), "\n")
	var ms, even []chat.Message
	for n := range 40 {
		text := "hi"
		if n%2 == 1 {
			text = long
		}
		m := msg(fmt.Sprintf("m%d", n), n+1, chat.RoleUser, text)
		ms = append(ms, m)
		if n%2 == 0 {
			even = append(even, m)
		}
	}
	eq(t, "all", FirstRows(Rows(ms)), 5)
	eq(t, "short ones", FirstRows(Rows(even)), 20)
	eq(t, "short ones, budget 1000", FirstRowsIn(Rows(even), 1000, 24), 20)
	eq(t, "one long", FirstRows(Rows([]chat.Message{msg("a", 1, chat.RoleUser, long+long+long)})), 1)
	eq(t, "none", FirstRows(nil), 0)
}

// The messages are as the thread store sorts them: a message sent during
// a turn after the answer and results made before it.
func TestAMessageSentDuringATurnShowsLast(t *testing.T) {
	call := withParts(msg("01A1", 1, chat.RoleAssistant), chat.Part{Kind: chat.PartToolCall, ToolCall: &chat.ToolCall{ID: "k1", Name: "query", Args: []byte("{}")}})
	result := withParts(msg("01A2", 1, chat.RoleTool), chat.Part{Kind: chat.PartToolResult, ToolResult: &chat.ToolResult{CallID: "k1", Text: "1 row"}})
	ms := []chat.Message{msg("00U1", 1, chat.RoleUser, "go"), call, result, msg("01U2", 1, chat.RoleUser, "also this")}
	rs := Rows(ms)
	eq(t, "kinds", rowKinds(rs), []string{"user", "agent", "user"})
	eq(t, "result", Results(rs[1].Messages)["k1"].Text, "1 row")
	// The joining message shows last; the agent's work stays the turn's last row.
	shown, joined := SplitJoining(rs, []string{"01U2"})
	eq(t, "shown", rowIDs(shown), []string{"00U1", "01A1"})
	eq(t, "joined", rowIDs(joined), []string{"01U2"})
	// Once the next try takes it, it is an ordinary message.
	_, joined = SplitJoining(rs, nil)
	eq(t, "joined none", len(joined), 0)
}

// Sent while idle, a message is the next turn's, so the last turn keeps
// its footer.
func TestANewTurnKeepsTheLastTurnsFooter(t *testing.T) {
	ms := []chat.Message{msg("01U3", 3, chat.RoleUser, "third"), msg("01A3", 3, chat.RoleAssistant, "Done."), msg("01U4", 4, chat.RoleUser, "fourth")}
	eq(t, "lasts", lastsOrNil(Rows(ms)), []string{"-", "true", "-"})
}

func TestAnApprovalLevelNoticeDoesntHideTheFailedTurn(t *testing.T) {
	notice := func(mid string, k chat.NoticeKind, text string) chat.Message {
		return withParts(msg(mid, 1, chat.RoleUser), noticePart(k, text))
	}
	failed := notice("01N1", chat.NoticeTurnFailed, "no model")
	level := notice("01N2", chat.NoticeApprovalLevel, "[level]")
	ms := []chat.Message{msg("01U1", 1, chat.RoleUser, "go"), failed, level}
	if n := LastNotice(ms); n == nil || n.Kind != chat.NoticeTurnFailed || n.Message.ID != failed.ID {
		t.Errorf("LastNotice = %v, want the failed turn's", n)
	}
	if n := LastNotice(append(ms[:3:3], msg("01U2", 2, chat.RoleUser, "again"))); n != nil {
		t.Errorf("LastNotice after a message = %v, want nil", n)
	}
	if n := LastNotice([]chat.Message{level}); n != nil {
		t.Errorf("LastNotice(level) = %v, want nil", n)
	}
}

func TestARefreshsCardShowsWithTheAgentsWork(t *testing.T) {
	notice := func(mid string, k chat.NoticeKind, text string) chat.Message {
		return withParts(msg(mid, 2, chat.RoleUser), noticePart(k, text))
	}
	card := withParts(msg("01C1", 2, chat.RoleUser), chat.Part{Kind: chat.PartApproval, Approval: &chat.Approval{ID: "a1", Kind: "memory", Target: "refresh:c1", Ask: "Save?"}})
	u1, a1 := msg("01U1", 1, chat.RoleUser, "go"), msg("01A1", 1, chat.RoleAssistant, "Done.")
	failed := notice("01N1", chat.NoticeTurnFailed, "no model")
	chip := notice("01N2", chat.NoticeMemoryRefreshed, "Memory refreshed: 1 added")
	ms := []chat.Message{u1, a1, msg("01U2", 2, chat.RoleUser, "again"), failed, card, chip}
	eq(t, "kinds", rowKinds(Rows(ms)), []string{"user", "agent", "user", "notice", "agent", "notice"})
	if n := LastNotice(ms); n == nil || n.Message.ID != failed.ID {
		t.Errorf("LastNotice = %v, want the failed turn's", n)
	}
	// After an answer, the card joins the answer's item, which keeps its footer.
	card1, chip1 := card, chip
	card1.Turn, chip1.Turn = 1, 1
	var got [][2]any
	for _, r := range Rows([]chat.Message{u1, a1, card1, chip1}) {
		if r.Kind == RowAgent {
			got = append(got, [2]any{len(r.Messages), r.Last})
		}
	}
	eq(t, "agent rows", got, [][2]any{{2, true}})
}

func TestToolTurn(t *testing.T) {
	call := withParts(msg("a1", 2, chat.RoleAssistant), chat.Part{Kind: chat.PartToolCall, ToolCall: &chat.ToolCall{ID: "k1", Name: "query"}})
	ms := []chat.Message{msg("u1", 1, chat.RoleUser, "hi"), msg("a0", 1, chat.RoleAssistant, "hello"), call}
	eq(t, "turn 1", ToolTurn(ms, 1), false)
	eq(t, "turn 2", ToolTurn(ms, 2), true)
}
