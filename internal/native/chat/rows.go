package chat

import (
	"slices"
	"strings"

	jc "github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/native/chat/logic"
	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/native/state"
)

// row builds rows[i] of the view: a user message, the agent's work, or a
// note from the app (RowView).
func (s *Screen) row(c *kit.Context, v *view, t *state.Thread, i int) {
	r := v.rows[i]
	running := t.State != jc.StateIdle
	switch r.Kind {
	case logic.RowUser:
		joining := slices.Contains(t.Joining, r.Message.ID)
		rewind := ""
		if turn, ok := v.starts[r.Message.ID]; ok && !running {
			rewind = "edit"
			if last := v.msgs[len(v.msgs)-1].Turn; turn < last {
				rewind = "both"
			}
		}
		s.userRow(c, v, t, r.Message, joining, rewind)
	case logic.RowNotice:
		n := r.Part.Notice
		if n == nil {
			return
		}
		failed := v.notice != nil && v.notice.Kind == jc.NoticeTurnFailed && !running &&
			v.notice.Message.ID == r.Message.ID && r.Part.Notice == lastPart(v.notice.Message).Notice
		if failed && s.noProvider() {
			s.connectCard(c, t)
			return
		}
		if failed {
			s.failedCard(c, t, n.Text)
			return
		}
		if n.Kind == jc.NoticeMemoryRefreshed && len(n.Edits) > 0 {
			s.refreshChip(c, t, string(r.Message.ID)+":refresh", n)
			return
		}
		noticeLine(c, *n, kit.Clock(c, r.Message.CreatedAt.Local()))
	case logic.RowAgent:
		last := i == lastRowIndex(v, t)
		stream := ""
		if last && t.Stream != "" && !slices.ContainsFunc(v.msgs, func(m jc.Message) bool { return m.ID == t.Stream }) {
			stream = string(t.Stream)
		}
		s.agentRow(c, v, t, r, stream, running && last)
	}
}

func lastPart(m jc.Message) jc.Part {
	if len(m.Parts) == 0 {
		return jc.Part{}
	}
	return m.Parts[len(m.Parts)-1]
}

// lastRowIndex is the index in rows of the last row shown before the
// messages that join the turn.
func lastRowIndex(v *view, t *state.Thread) int {
	joining := make([]string, len(t.Joining))
	for i, m := range t.Joining {
		joining[i] = string(m)
	}
	shown, _ := logic.SplitJoining(v.rows, joining)
	return len(shown) - 1
}

// noticeLine is a note from the app between messages.
func noticeLine(c *kit.Context, n jc.Notice, at string) {
	warn := n.Kind == jc.NoticeAnswerCut || n.Kind == jc.NoticeEarlyStop || n.Kind == jc.NoticeTurnFailed
	text := logic.NoticeText(n.Text)
	if at != "" {
		text += " · " + at
	}
	kit.Marker(c, text, warn)
}

// userRow is a message of the user's: its quote, its text in a bubble,
// and Reply with the rewind actions under it.
func (s *Screen) userRow(c *kit.Context, v *view, t *state.Thread, m jc.Message, joining bool, rewind string) {
	text := state.MessageText(&m)
	kit.Message(c, "You said", true, func() {
		for i, p := range m.Parts {
			switch {
			case p.Image != nil:
				kit.Muted(c, "Image: "+imageName(*p.Image)).Key(i)
			case p.Quote != nil:
				if kit.Quote(c, p.Quote.Text) {
					s.goTo(v, p.Quote.Message)
				}
			case p.Text != nil:
				s.bubble(c, p.Text.Text, joining).Key(i)
			}
		}
		if joining {
			kit.Footer(c, func() { kit.Text(c, "Joins at the next step") })
			return
		}
		if text == "" {
			return
		}
		kit.Footer(c, func() {
			if kit.SmallButton(c, kit.IconReply, "Reply", kit.Ghost).Clicked() {
				s.reply(t.Chat.ID, &m, text)
			}
			if rewind != "" {
				s.rewindActions(c, t, m, rewind == "both")
			}
		})
	})
}

func imageName(im jc.Image) string {
	if im.Alt != "" {
		return im.Alt
	}
	return im.Ref
}

// reply quotes a whole message in the composer (TASK-019).
func (s *Screen) reply(ch id.Chat, m *jc.Message, text string) {
	s.st.Quotes.Set(ch, state.QuoteOf(m, text))
	if v := s.views[ch]; v != nil {
		v.focus = true
	}
}

// goTo scrolls to the row holding message m.
func (s *Screen) goTo(v *view, m id.Message) {
	for i, it := range v.items {
		if it.kind != itemRow {
			continue
		}
		r := v.rows[it.row]
		if (r.Kind != logic.RowAgent && r.Message.ID == m) ||
			slices.ContainsFunc(r.Messages, func(x jc.Message) bool { return x.ID == m }) {
			v.list.HoldPlace()
			v.list.ScrollTo(i, kit.Center)
			return
		}
	}
}

// agentRow is the agent's work between two user messages: its parts, the
// answer streaming at its end, and the turn's footer on the row that ends
// it.
func (s *Screen) agentRow(c *kit.Context, v *view, t *state.Thread, r logic.Row, stream string, running bool) {
	busy := t.State != jc.StateIdle && hasCards(r.Messages)
	var waiting *jc.Waiting
	if hasCards(r.Messages) {
		waiting = t.Waiting
	}
	var answer *jc.Message
	for i := len(r.Messages) - 1; i >= 0; i-- {
		if r.Messages[i].Role == jc.RoleAssistant {
			answer = &r.Messages[i]
			break
		}
	}
	kit.Message(c, "Jenab said", false, func() {
		for _, m := range r.Messages {
			for i, p := range m.Parts {
				kit.Column(c).Key(string(m.ID) + ":" + itoa(i)).AlignItems(kit.Stretch).Children(func() {
					s.part(c, v, t, m, i, p, running, busy, waiting)
				})
			}
		}
		if stream != "" {
			s.stream(c, t)
		}
		if !r.Last || running || answer == nil {
			return
		}
		s.footer(c, t, r, *answer)
	})
}

// footer is the turn's footer: Copy, Reply, the time, the model, Undo
// turn.
func (s *Screen) footer(c *kit.Context, t *state.Thread, r logic.Row, answer jc.Message) {
	var quoted *jc.Message
	for i := len(r.Messages) - 1; i >= 0; i-- {
		if m := &r.Messages[i]; m.Role == jc.RoleAssistant && state.MessageText(m) != "" {
			quoted = m
			break
		}
	}
	kit.Footer(c, func() {
		copied := s.flag("copied:" + string(answer.ID))
		label := "Copy"
		if *copied {
			label = "Copied"
		}
		if kit.SmallButton(c, kit.IconCopy, label, kit.Ghost).Clicked() {
			c.WriteClipboard(logic.CopyText(r.Messages))
			*copied = true
		}
		if quoted != nil && kit.SmallButton(c, kit.IconReply, "Reply", kit.Ghost).Clicked() {
			s.reply(t.Chat.ID, quoted, state.MessageText(quoted))
		}
		kit.Text(c, kit.Clock(c, answer.CreatedAt.Local())).PaddingStart(4)
		if answer.Model != "" {
			kit.Text(c, "· "+logic.ModelName(s.st.Settings.Models, answer.Model))
		}
		kit.Spacer(c)
		if logic.ToolTurn(s.views[t.Chat.ID].msgs, answer.Turn) {
			s.undoTurn(c, t, answer.Turn, 0, "")
		}
		if started := s.st.Settings.Started; started != nil && started.DevTools &&
			kit.SmallButton(c, nil, "Inspect", kit.Ghost).Clicked() {
			s.st.Nav.Go(state.Place{View: state.ViewInspector, Project: t.Project, Chat: t.Chat.ID, Turn: answer.Turn})
		}
	})
}

func hasCards(ms []jc.Message) bool {
	return slices.ContainsFunc(ms, func(m jc.Message) bool {
		return slices.ContainsFunc(m.Parts, func(p jc.Part) bool { return p.Kind == jc.PartApproval || p.Kind == jc.PartQuestion })
	})
}

// part is one part of the agent's work (PartView).
func (s *Screen) part(c *kit.Context, v *view, t *state.Thread, m jc.Message, i int, p jc.Part, running, busy bool, w *jc.Waiting) {
	key := string(m.ID) + ":" + itoa(i)
	switch p.Kind {
	case jc.PartText:
		if p.Text != nil {
			s.md.View(c, state.MarkLinks(p.Text.Text))
			if p.Text.Stopped {
				kit.Muted(c, "Stopped")
			}
		}
	case jc.PartThinking:
		if p.Thinking != nil {
			s.thinking(c, key, p.Thinking.Text, false)
		}
	case jc.PartToolCall:
		if p.ToolCall != nil {
			var res *jc.ToolResult
			if r, ok := v.results[p.ToolCall.ID]; ok {
				res = &r
			}
			s.toolChip(c, key, *p.ToolCall, res, running)
		}
	case jc.PartImage:
		if p.Image != nil {
			kit.Muted(c, "Image: "+imageName(*p.Image))
		}
	case jc.PartNotice:
		s.noticePart(c, t, key, p.Notice)
	case jc.PartApproval, jc.PartQuestion:
		live := (w != nil && w.Message == m.ID && w.Index == i) || busy
		if p.Approval != nil {
			s.approvalCard(c, t, m.ID, i, *p.Approval, live)
		}
		if p.Question != nil {
			s.questionForm(c, t, m.ID, i, *p.Question, live)
		}
	}
}

// noticePart is a notice inside the agent's work: a skill's chip, a
// schema change, a memory edit or a desktop save with its undo, or a
// line.
func (s *Screen) noticePart(c *kit.Context, t *state.Thread, key string, n *jc.Notice) {
	if n == nil || n.Kind == jc.NoticeSkillText {
		return
	}
	switch n.Kind {
	case jc.NoticeSkillLoaded:
		kit.Record(c, kit.IconSparkles, kit.Neutral, func() { kit.Text(c, n.Text) })
	case jc.NoticeMemoryUpdated:
		s.memoryChip(c, t, key, n)
	case jc.NoticeDesktopSaved:
		s.desktopChip(c, t, key, n)
	case jc.NoticeMigration:
		kit.Record(c, kit.IconPackage, kit.Neutral, func() { kit.Text(c, n.Text) })
		if n.Details != "" {
			kit.Disclosure(c, "View steps", s.flag(key), false, func() { kit.Text(c, "View steps") }, func() {
				kit.Panel(c, func() { kit.Pre(c, n.Details) })
			})
		}
		if n.Note != "" {
			kit.Muted(c, n.Note)
		}
	default:
		noticeLine(c, *n, "")
	}
}

// thinking is a thinking part, folded.
func (s *Screen) thinking(c *kit.Context, key, text string, streaming bool) {
	label := "Thought"
	switch {
	case streaming:
		label = "Thinking"
	case text == "":
		label = "Thinking hidden by the provider"
	}
	kit.Disclosure(c, label, s.flag(key), false, func() {
		kit.Icon(c, kit.IconBrain).Size(12, 12)
		kit.Text(c, label)
	}, func() {
		if text != "" {
			kit.Detail(c, func() { kit.Text(c, text).PreserveSpaces() })
		}
	})
}

// toolChip is a tool call with its result: a line, unfolded to the
// arguments and the output. A desktop step shows no texts (SPEC 8.10).
func (s *Screen) toolChip(c *kit.Context, key string, call jc.ToolCall, res *jc.ToolResult, running bool) {
	chip := logic.ToolChip(call, res, running)
	icon := func() {
		switch chip.State {
		case logic.ChipDone:
			kit.Icon(c, kit.IconCheck).Size(12, 12).TextColor(kit.TokensOf(c).Tone(kit.Green).Text)
		case logic.ChipRunning:
			kit.SmallSpinner(c, "Running")
		default:
			kit.Icon(c, kit.IconX).Size(12, 12).TextColor(kit.TokensOf(c).Tone(kit.Red).Text)
		}
	}
	label := call.Name + " " + chip.Detail
	if chip.Desktop {
		kit.Record(c, nil, kit.Neutral, func() {
			icon()
			kit.Mono(c, call.Name)
			kit.Muted(c, chip.Detail)
		})
		return
	}
	kit.Disclosure(c, label, s.flag(key), true, func() {
		icon()
		kit.Mono(c, call.Name).TextColor(kit.TokensOf(c).Foreground)
		kit.Text(c, chip.Detail).SingleLine().Shrink(1)
	}, func() {
		kit.Panel(c, func() {
			detail(c, "input", logic.ArgsText(call.Args))
			if res != nil {
				name := "output"
				if res.IsError {
					name = "error"
				}
				detail(c, name, res.Text)
				if res.Ref != "" {
					s.storedAs(c, res.Ref)
				}
			}
		})
	})
}

func detail(c *kit.Context, label, text string) {
	kit.Row(c).Gap(12).AlignItems(kit.Start).Children(func() {
		kit.Mono(c, label).Width(56).Shrink(0).TextColor(kit.TokensOf(c).MutedForeground)
		kit.Pre(c, text).Grow(1).Shrink(1)
	})
}

// stream is the answer still streaming: its finished parts, then the part
// streaming, in markdown.Stream.
func (s *Screen) stream(c *kit.Context, t *state.Thread) {
	st := s.st.Threads.Stream(t.Chat.ID)
	if st == nil || st.Message != t.Stream {
		kit.Spinner(c, "Working")
		return
	}
	for i, p := range st.Parts {
		key := "stream:" + string(st.Message) + ":" + itoa(i)
		kit.Column(c).Key(key).AlignItems(kit.Stretch).Children(func() {
			switch {
			case p.Text != nil:
				s.md.View(c, state.MarkLinks(p.Text.Text))
			case p.Thinking != nil:
				s.thinking(c, key, p.Thinking.Text, false)
			case p.ToolCall != nil:
				s.toolChip(c, key, *p.ToolCall, nil, true)
			}
		})
	}
	key := "stream:" + string(st.Message) + ":" + itoa(st.Index)
	kit.Column(c).Key(key).AlignItems(kit.Stretch).Children(func() {
		switch {
		case st.Kind == jc.PartThinking:
			s.thinking(c, key, st.Text, true)
		case strings.TrimSpace(st.Text) != "":
			s.md.Stream(c, state.MarkLinks(st.Text))
		default:
			kit.Spinner(c, "Working")
		}
	})
}
