package logic

import (
	"fmt"
	"slices"
	"strings"

	"github.com/amir-saatchi/jenab/internal/chat"
)

// RowKind is what a row of the thread holds.
type RowKind string

const (
	RowUser   RowKind = "user"
	RowAgent  RowKind = "agent"
	RowNotice RowKind = "notice"
)

// Row is one item of the thread: a user message, the agent's work between
// two user messages, or a note from the app. Message is a user row's and a
// notice row's, with Part the notice; Messages and Last are an agent row's.
type Row struct {
	Kind     RowKind
	ID       string
	Message  chat.Message
	Part     chat.Part
	Messages []chat.Message
	Last     bool
}

// aside are the notices that don't say how a turn ended (agent.aside):
// those added to a turn after it ended, and hidden ones.
var aside = []chat.NoticeKind{
	chat.NoticeApprovalLevel,
	chat.NoticeTurnUndone,
	chat.NoticeMemoryRefreshed,
	chat.NoticeCardOutside,
	chat.NoticeRewound,
}

func isAsideNotice(p chat.Part) bool {
	return p.Notice != nil && slices.Contains(aside, p.Notice.Kind)
}

// hidden tells whether a message holds only notices nobody reads: history
// cards' mark of outside data, for the app (agent.cardMark), and a
// rewind's notice, for the model; the rewind's mark stands for it.
func hidden(m chat.Message) bool {
	return isNotice(m) && !slices.ContainsFunc(m.Parts, func(p chat.Part) bool {
		return p.Notice == nil || (p.Notice.Kind != chat.NoticeCardOutside && p.Notice.Kind != chat.NoticeRewound)
	})
}

// isNotice tells whether a user message is a note from the app.
func isNotice(m chat.Message) bool {
	return m.Role == chat.RoleUser && !slices.ContainsFunc(m.Parts, func(p chat.Part) bool { return p.Kind != chat.PartNotice })
}

// RefreshCard tells whether a message is Refresh memory's card (SPEC
// 3.5). It is a user message, so the model never reads it, but it shows
// with the agent's work, as cards do; and like an aside notice it doesn't
// say how the turn ended (agent.refreshCard).
func RefreshCard(m chat.Message) bool {
	return len(m.Parts) > 0 && !slices.ContainsFunc(m.Parts, func(p chat.Part) bool {
		return p.Approval == nil || !strings.HasPrefix(p.Approval.Target, "refresh:")
	})
}

// isAside tells whether a message holds only notices added after a turn
// ended.
func isAside(m chat.Message) bool {
	return isNotice(m) && !slices.ContainsFunc(m.Parts, func(p chat.Part) bool { return !isAsideNotice(p) })
}

// Rows groups messages for the thread. Assistant and tool messages in a row
// are one agent item; Last marks the one that ends a turn, which gets the
// turn's footer. Turns of messages not yet read from the store are a guess
// until the turn ends. Notices added after a turn ended, such as an
// undo's, sit in the chat's last turn but don't take its footer away.
func Rows(messages []chat.Message) []Row {
	var out []Row
	var group []chat.Message
	i := 0
	closeGroup := func(next *chat.Message) {
		if next != nil && isAside(*next) {
			next = nil
			for j := i + 1; j < len(messages); j++ {
				if !isAside(messages[j]) {
					next = &messages[j]
					break
				}
			}
		}
		if group != nil {
			last := next == nil || next.Turn != group[len(group)-1].Turn
			out = append(out, Row{Kind: RowAgent, ID: string(group[0].ID), Messages: group, Last: last})
		}
		group = nil
	}
	for ; i < len(messages); i++ {
		m := messages[i]
		if hidden(m) {
			continue
		}
		if m.Role != chat.RoleUser || RefreshCard(m) {
			group = append(group, m)
			continue
		}
		if isNotice(m) {
			closeGroup(&messages[i])
			for n, p := range m.Parts {
				out = append(out, Row{Kind: RowNotice, ID: fmt.Sprintf("%s:%d", m.ID, n), Message: m, Part: p})
			}
			continue
		}
		closeGroup(&messages[i])
		out = append(out, Row{Kind: RowUser, ID: string(m.ID), Message: m})
	}
	closeGroup(nil)
	return out
}

// SplitJoining splits off the user messages at the end that join the
// running turn at its next step. They show last, below the answer
// streaming, and the agent's work before them is still the turn's last row.
func SplitJoining(rs []Row, joining []string) ([]Row, []Row) {
	i := len(rs)
	for i > 0 && rs[i-1].Kind == RowUser && slices.Contains(joining, rs[i-1].ID) {
		i--
	}
	return rs[:i], rs[i:]
}

// Results are a group's tool results by call ID.
func Results(messages []chat.Message) map[string]chat.ToolResult {
	out := map[string]chat.ToolResult{}
	for _, m := range messages {
		for _, p := range m.Parts {
			if p.ToolResult != nil {
				out[p.ToolResult.CallID] = *p.ToolResult
			}
		}
	}
	return out
}

// Notice is the newest notice of a chat, with its message.
type Notice struct {
	Message chat.Message
	Kind    chat.NoticeKind
	Text    string
}

// LastNotice is the newest notice of the chat, for the provider card: a
// turn that failed because no model is set up. As on the Go side, messages
// that only note a change of the approval level, an undo or a refresh, and
// a refresh's card, don't count. It is nil when the newest message that
// counts doesn't end with a notice.
func LastNotice(messages []chat.Message) *Notice {
	for k := len(messages) - 1; k >= 0; k-- {
		m := messages[k]
		if RefreshCard(m) || !slices.ContainsFunc(m.Parts, func(p chat.Part) bool { return !isAsideNotice(p) }) {
			continue
		}
		p := m.Parts[len(m.Parts)-1]
		if p.Notice == nil {
			return nil
		}
		return &Notice{Message: m, Kind: p.Notice.Kind, Text: p.Notice.Text}
	}
	return nil
}

// ToolTurn tells whether the turn called a tool, so it may have changed
// the project and has an *Undo turn*.
func ToolTurn(messages []chat.Message, turn int) bool {
	return slices.ContainsFunc(messages, func(m chat.Message) bool {
		return m.Turn == turn && slices.ContainsFunc(m.Parts, func(p chat.Part) bool { return p.Kind == chat.PartToolCall })
	})
}

// CopyText is an agent item's text, for *Copy*.
func CopyText(messages []chat.Message) string {
	var texts []string
	for _, m := range messages {
		if m.Role != chat.RoleAssistant {
			continue
		}
		for _, p := range m.Parts {
			if p.Text != nil && p.Text.Text != "" {
				texts = append(texts, p.Text.Text)
			}
		}
	}
	return strings.Join(texts, "\n\n")
}

// rowLines is a rough height of a row in lines of text.
func rowLines(r Row) int {
	ms := []chat.Message{r.Message}
	if r.Kind == RowAgent {
		ms = r.Messages
	}
	n := 1
	for _, m := range ms {
		for _, p := range m.Parts {
			if p.Text != nil && p.Text.Text != "" {
				t := p.Text.Text
				n += strings.Count(t, "\n") + 1 + utf16Len(t)/80
			} else {
				n++
			}
		}
	}
	return n
}

// FirstRows is how many rows, from the end, a chat renders when it opens:
// about two screens, so the opening frame stays short (N-02). It is
// FirstRowsIn with the React app's 80 lines and 24 rows.
func FirstRows(rs []Row) int {
	return FirstRowsIn(rs, 80, 24)
}

// FirstRowsIn is FirstRows with a budget of lines and a most of rows.
func FirstRowsIn(rs []Row, budget, most int) int {
	n := 0
	for used := 0; n < len(rs) && n < most && used < budget; n++ {
		used += rowLines(rs[len(rs)-1-n])
	}
	return max(n, min(2, len(rs)))
}
