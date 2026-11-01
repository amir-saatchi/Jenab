package chatexport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/bidi"

	"github.com/amir-saatchi/jenab/internal/bucket"
	"github.com/amir-saatchi/jenab/internal/chat"
)

// ArgsMax and LineMax are how many characters of a tool call's arguments
// and of a notice, approval or question line the Markdown keeps.
const (
	ArgsMax = 80
	LineMax = 200
)

// hidden are the notices the chat doesn't show (SPEC 8.9, 8.8); the
// Markdown leaves them out, like thinking.
var hidden = map[chat.NoticeKind]bool{chat.NoticeSkillText: true, chat.NoticeCardOutside: true}

// md writes the Markdown export. Text blocks and list lines are kept
// apart by blank lines, as Markdown needs.
type md struct {
	s      Source
	b      bytes.Buffer // the message being written; flushed after each one
	inList bool
	wrote  bool // anything written yet, in this message or before
	// speaker is the heading the turn shows now: "user", or
	// "assistant:<model>"; a new one starts a new heading.
	speaker string
}

func writeMarkdown(ctx context.Context, w io.Writer, s Source) (int, error) {
	m := &md{s: s}
	m.header()
	if _, err := w.Write(m.b.Bytes()); err != nil {
		return 0, err
	}
	n := 0
	err := eachBatch(ctx, s, func(ms []chat.Message) error {
		results := map[string]*chat.ToolResult{}
		for _, msg := range ms {
			for _, p := range msg.Parts {
				if p.Kind == chat.PartToolResult {
					results[p.ToolResult.CallID] = p.ToolResult
				}
			}
		}
		turn := -1
		for _, msg := range ms {
			if msg.Turn != turn {
				turn, m.speaker = msg.Turn, ""
			}
			m.b.Reset()
			m.message(msg, results)
			// The parts were redacted before they were cut; this catches
			// anything assembled around them.
			if _, err := io.WriteString(w, s.Redact(m.b.String())); err != nil {
				return err
			}
			n++
		}
		return nil
	})
	return n, err
}

func (m *md) header() {
	title := m.s.Redact(m.s.Chat.Title)
	if title == "" {
		title = "New chat"
	}
	m.block("# " + oneLine(title))
	model := m.s.Model
	if model == "" {
		model = m.s.Chat.Model
	}
	m.line("Project: " + m.inline(m.s.ProjectName))
	m.line("Model: " + m.inline(model))
	m.line("Created: " + m.s.Chat.CreatedAt.In(m.s.Loc).Format("2006-01-02 15:04 -07:00"))
	m.line("Exported: " + m.s.Now.In(m.s.Loc).Format("2006-01-02 15:04 -07:00"))
	m.raw("---")
}

func (m *md) message(msg chat.Message, results map[string]*chat.ToolResult) {
	switch msg.Role {
	case chat.RoleUser:
		m.heading("user", "You · "+msg.CreatedAt.In(m.s.Loc).Format("2006-01-02 15:04"))
	case chat.RoleAssistant:
		h := "Assistant"
		if msg.Model != "" {
			h += " · " + msg.Model
		}
		m.heading("assistant:"+msg.Model, h)
	}
	// Tool messages carry results and chips: they stay under the heading
	// the turn has.
	for _, p := range msg.Parts {
		m.part(p, results)
	}
}

func (m *md) heading(speaker, text string) {
	if m.speaker == speaker {
		return
	}
	m.speaker = speaker
	m.raw("### " + text)
}

func (m *md) part(p chat.Part, results map[string]*chat.ToolResult) {
	switch p.Kind {
	case chat.PartText:
		t := strings.TrimSpace(m.s.Redact(p.Text.Text))
		if p.Text.Stopped {
			t += "\n\n_(stopped)_"
		}
		if t != "" {
			m.block(t)
		}
	case chat.PartToolCall:
		m.line(m.toolLine(p.ToolCall, results[p.ToolCall.ID]))
	case chat.PartToolResult:
		// Shown on its call's line.
	case chat.PartImage:
		alt := oneLine(m.s.Redact(p.Image.Alt))
		alt = strings.NewReplacer("[", `\[`, "]", `\]`).Replace(alt)
		m.block("![" + alt + "](" + bucket.Link(m.s.Project, p.Image.Ref) + ")")
	case chat.PartNotice:
		if !hidden[p.Notice.Kind] {
			m.line("Notice: " + m.inline(p.Notice.Text))
		}
	case chat.PartApproval:
		m.line("Approval: " + m.inline(p.Approval.Ask) + " → " + m.approvalAnswer(p.Approval))
	case chat.PartQuestion:
		m.question(p.Question)
	}
	// Thinking is left out.
}

// toolLine is a tool call as one line: its name, its arguments cut short
// and its result's size, error or link.
func (m *md) toolLine(c *chat.ToolCall, r *chat.ToolResult) string {
	line := "Tool " + code(c.Name)
	args := c.Invalid
	if args == "" {
		var b bytes.Buffer
		if json.Compact(&b, c.Args) == nil {
			args = b.String()
		} else {
			args = string(c.Args)
		}
	}
	if args = oneLine(m.s.Redact(args)); args != "" && args != "{}" {
		line += " " + code(cut(args, ArgsMax))
	}
	switch {
	case r == nil:
		line += " → no result"
	case r.IsError:
		line += " → error: " + m.inline(firstLine(r.Text))
	case r.Ref != "":
		line += " → " + bucket.Link(m.s.Project, r.Ref)
	default:
		line += " → " + size(len(r.Text))
	}
	return line
}

func (m *md) approvalAnswer(a *chat.Approval) string {
	var out string
	switch {
	case a.Answer != "":
		o, _ := a.Option(a.Answer)
		out = m.inline(o.Label)
		if a.By != "" {
			out += " (" + string(a.By) + ")"
		}
	case a.Stopped:
		out = "stopped"
	default:
		out = "no answer yet"
	}
	if a.Note != "" {
		out += ": " + m.inline(a.Note)
	}
	return out
}

func (m *md) question(q *chat.Question) {
	for _, it := range q.Questions {
		ans := "no answer"
		if got := q.Answers[it.Header]; len(got) > 0 {
			ans = m.inline(strings.Join(got, ", "))
		} else if q.Stopped {
			ans = "stopped"
		}
		m.line("Question: " + m.inline(it.Question) + " → " + ans)
	}
	if q.Note != "" {
		m.line("Answered with a message: " + m.inline(q.Note))
	}
}

// inline is text for a short line: redacted, on one line, cut to LineMax,
// and isolated in <bdi> when it runs right to left, so its words and
// punctuation keep their order inside the English line (SPEC 5.8).
func (m *md) inline(s string) string {
	s = cut(oneLine(m.s.Redact(s)), LineMax)
	if rtl(s) {
		return "<bdi>" + s + "</bdi>"
	}
	return s
}

// block writes a paragraph or more. Text that starts right to left is
// wrapped in <div dir="rtl">, with blank lines so Markdown inside it still
// renders (SPEC 5.8). The text itself is unchanged.
func (m *md) block(s string) {
	if rtl(s) {
		s = "<div dir=\"rtl\">\n\n" + s + "\n\n</div>"
	}
	m.raw(s)
}

// raw writes a block as it is.
func (m *md) raw(s string) {
	if m.wrote {
		m.b.WriteString("\n")
	}
	m.inList, m.wrote = false, true
	m.b.WriteString(s)
	m.b.WriteString("\n")
}

// line writes a list line.
func (m *md) line(s string) {
	if !m.inList && m.wrote {
		m.b.WriteString("\n")
	}
	m.inList, m.wrote = true, true
	m.b.WriteString("- " + s + "\n")
}

// rtl tells whether the first strong character of s runs right to left,
// as dir="auto" decides.
func rtl(s string) bool {
	for _, r := range s {
		p, _ := bidi.LookupRune(r)
		switch p.Class() {
		case bidi.L:
			return false
		case bidi.R, bidi.AL:
			return true
		}
	}
	return false
}

// code is s as an inline code span, with a fence longer than any run of
// backticks in it.
func code(s string) string {
	longest, run := 0, 0
	for _, r := range s {
		if r == '`' {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}
	fence := strings.Repeat("`", longest+1)
	if strings.HasPrefix(s, "`") || strings.HasSuffix(s, "`") {
		s = " " + s + " "
	}
	return fence + s + fence
}

// cut keeps the first n characters of s, adding … when it cuts.
func cut(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	i := 0
	for j := range s {
		if i == n {
			return strings.TrimRightFunc(s[:j], unicode.IsSpace) + "…"
		}
		i++
	}
	return s
}

// oneLine joins s's lines and runs of spaces with single spaces.
func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// size is a byte count as the UI shows it.
func size(n int) string {
	switch {
	case n < 1000:
		return fmt.Sprintf("%d B", n)
	case n < 1000*1000:
		return fmt.Sprintf("%.1f KB", float64(n)/1000)
	default:
		return fmt.Sprintf("%.1f MB", float64(n)/1000/1000)
	}
}
