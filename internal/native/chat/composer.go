package chat

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/amir-saatchi/jenab/internal/app"
	jc "github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/native/chat/logic"
	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/native/state"
)

// maxUpload is the largest file the app stores (store.MaxUpload).
const maxUpload = 100 << 20

// attached is a file attached in the composer (TASK-021).
type attached struct {
	id    int
	name  string
	size  int64
	state string // uploading, done or error
	file  app.Uploaded
	err   string
}

// composer is a chat's composer between frames: its text is the stores'
// draft.
type composer struct {
	text   string
	loaded bool
	files  []*attached
	next   int
	focus  bool // put the focus in the text, as after Reply
}

// composerView is where the user writes (SPEC 8.3, composer.tsx): Enter
// sends, also during a turn, where the message joins at the next step;
// Shift+Enter makes a new line; Escape takes the quote off.
func (s *Screen) composerView(c *kit.Context, v *view, t *state.Thread, readOnly bool) {
	st := s.st
	ch, p := t.Chat.ID, t.Project
	cm := &v.composer
	if !cm.loaded {
		cm.text, cm.loaded, cm.focus = st.Threads.Drafts[ch], true, true
	}
	if fill, ok := st.Threads.Fills[ch]; ok {
		cm.text, cm.files, cm.focus = fill, nil, true
		st.Threads.Take(ch)
	}
	running := t.State != jc.StateIdle
	quote, quoted := st.Quotes[ch]
	uploading, done := false, false
	for _, f := range cm.files {
		uploading = uploading || f.state == "uploading"
		done = done || f.state == "done"
	}
	canSend := !readOnly && !uploading && (strings.TrimSpace(cm.text) != "" || done)
	send := func() bool {
		if !canSend {
			return false
		}
		text, kept := withLinks(cm.text, cm.files), cm.files
		before := cm.text
		var q *jc.Quote
		if quoted {
			qq := quote
			q = &qq
		}
		cm.text, cm.files = "", nil
		delete(st.Threads.Drafts, ch)
		st.Quotes.Set(ch, nil)
		st.Actions.Send(p, ch, text, q, func(m id.Message, err error) {
			if err != nil {
				cm.text, cm.files = before, kept
				st.Threads.Drafts[ch] = before
				st.Quotes.Set(ch, q)
				st.Error(err)
				return
			}
			if running {
				st.Threads.Joined(ch, m)
			}
		})
		return true
	}
	title := t.Chat.Title
	if title == "" {
		title = "the chat"
	}
	placeholder := "Message " + title + "…"
	switch {
	case readOnly:
		placeholder = "This project is open read-only."
	case running:
		placeholder = "Write anytime. Your message joins the running turn."
	}
	box := kit.Column(c).Width(768).MaxWidthPercent(100).AlignSelf(kit.Center).Padding(0, 24, 16, 24).AlignItems(kit.Stretch)
	// In a narrow chat, as one docked next to a file, the pickers make room
	// as composer.tsx's do under @max-sm/composer: the input group under
	// 384 px, its box less the padding.
	narrow := kit.Narrower(c, box, composerNarrow+48)
	box.Children(func() {
		kit.InputGroup(c, func() {
			if quoted {
				kit.Box(c).Padding(8, 8, 0, 8).Children(func() {
					who := "your message"
					if quote.Role == jc.RoleAssistant {
						who = "the answer"
					}
					if kit.QuoteChip(c, who, quote.Text) {
						st.Quotes.Set(ch, nil)
						cm.focus = true
					}
				})
			}
			if len(cm.files) > 0 {
				kit.Row(c).Gap(6).Padding(8, 8, 0, 8).Wrap().Children(func() {
					for _, f := range cm.files {
						kit.Column(c).Key(f.id).Children(func() {
							if kit.Attachment(c, fileIcon(f), f.name, describe(f), f.state == "error", f.state == "uploading") {
								cm.remove(f.id)
								cm.focus = true
							}
						})
					}
				})
			}
			area := kit.ComposerText(c, &cm.text, "Message").Placeholder(placeholder)
			if readOnly {
				kit.Disable(area, true)
			}
			if cm.focus {
				area.Focus()
				cm.focus = false
			}
			if area.Changed() {
				if cm.text != "" {
					st.Threads.Drafts[ch] = cm.text
				} else {
					delete(st.Threads.Drafts, ch)
				}
			}
			kit.ComposerKeys(area, func(text string) bool {
				cm.text = text
				return send()
			}, func() bool {
				if _, ok := st.Quotes[ch]; !ok {
					return false
				}
				st.Quotes.Set(ch, nil)
				return true
			})
			for _, path := range area.DroppedFiles() {
				s.attach(cm, p, path)
			}
			kit.Row(c).Gap(4).Padding(4, 8, 8, 8).AlignItems(kit.Center).Children(func() {
				if st.Actions.PickFiles != nil {
					b := kit.Tooltip(kit.IconButton(c, kit.IconPaperclip, "Attach files", kit.Ghost).Size(32, 32), "Attach files")
					if kit.Disable(b, readOnly).Clicked() {
						st.Actions.Pick(func(paths []string) {
							for _, path := range paths {
								s.attach(cm, p, path)
							}
							cm.focus = true
						})
					}
				}
				s.modelPicker(c, t, narrow)
				s.levelPicker(c, t, narrow)
				kit.Spacer(c)
				if running && strings.TrimSpace(cm.text) == "" && len(cm.files) == 0 {
					if kit.SmallButton(c, kit.IconStop, "Stop", kit.Default).Height(32).Clicked() {
						st.Actions.Stop(p, ch)
					}
				} else if kit.Disable(kit.IconButton(c, kit.IconSend, "Send", kit.Default).Size(32, 32), !canSend).Clicked() {
					cm.focus = send()
				}
			})
		})
	})
}

func (cm *composer) remove(n int) {
	for i, f := range cm.files {
		if f.id == n {
			cm.files = append(cm.files[:i:i], cm.files[i+1:]...)
			return
		}
	}
}

// attach stores the file at path; one that can't be stored stays, saying
// why, until it is removed.
func (s *Screen) attach(cm *composer, p id.Project, path string) {
	cm.next++
	f := &attached{id: cm.next, name: filepath.Base(path), state: "uploading"}
	cm.files = append(cm.files, f)
	if info, err := os.Stat(path); err == nil {
		f.size = info.Size()
	}
	if f.size > maxUpload {
		f.state, f.err = "error", "It is larger than 100 MB."
		return
	}
	s.st.Actions.Attach(p, path, func(up app.Uploaded, err error) {
		if err != nil {
			f.state, f.err = "error", err.Error()
			return
		}
		f.state, f.file = "done", up
	})
}

// withLinks is the text sent for a message with files: the user's words,
// then each file's link on its own line.
func withLinks(text string, files []*attached) string {
	var links []string
	for _, f := range files {
		if f.state == "done" {
			links = append(links, f.file.Link)
		}
	}
	var parts []string
	if t := strings.TrimSpace(text); t != "" {
		parts = append(parts, t)
	}
	if len(links) > 0 {
		parts = append(parts, strings.Join(links, "\n"))
	}
	return strings.Join(parts, "\n\n")
}

func fileIcon(f *attached) *kit.SVG {
	switch {
	case strings.HasPrefix(f.file.MIME, "image/"):
		return kit.IconImage
	case strings.HasPrefix(f.file.MIME, "text/"), f.file.MIME == "application/pdf":
		return kit.IconFileText
	}
	return kit.IconFile
}

// describe is the line under an attached file's name.
func describe(f *attached) string {
	switch f.state {
	case "uploading":
		return "Uploading…"
	case "error":
		return f.err
	}
	if f.file.Note != "" {
		return f.file.Note
	}
	return size(f.file.Size)
}

func size(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.0f KB", float64(n)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
}

// modelPicker sets the chat's model from those that are on, by provider.
func (s *Screen) modelPicker(c *kit.Context, t *state.Thread, narrow bool) {
	groups := s.st.Settings.Models
	name := logic.ModelName(groups, t.Chat.Model)
	maxLabel := float32(160)
	if narrow {
		maxLabel = 96
	}
	kit.MenuPicker(c, kit.IconSparkles, name, "Model: "+name, maxLabel, func(m *kit.Menu) {
		if len(groups) == 0 {
			m.Item("No model is on").Disabled(true)
			return
		}
		for gi, g := range groups {
			if gi > 0 {
				m.Separator()
			}
			m.Item(g.Provider).Disabled(true)
			for _, mod := range g.Models {
				it := m.Item(mod.Name).Radio(mod.Ref == t.Chat.Model || containsAlias(mod.Aliases, t.Chat.Model))
				if it.Chosen() {
					ref := mod.Ref
					p, ch := t.Project, t.Chat.ID
					s.st.Actions.SetModel(p, ch, ref, func(err error) {
						if err != nil {
							s.st.Error(err)
							return
						}
						info := t.Chat
						info.Model = ref
						s.st.Threads.SetChat(info)
					})
				}
			}
		}
	})
}

// levelPicker sets the project's approval level (SPEC 8.8), "Approvals:
// Standard", next to the model picker. A change applies from the next
// tool call, and the chat gets a notice of it.
func (s *Screen) levelPicker(c *kit.Context, t *state.Thread, narrow bool) {
	p, ch := t.Project, t.Chat.ID
	level := s.st.Projects.Opened[p].Level
	if level == "" {
		level = "standard"
	}
	name := "Approvals: " + logic.LevelName(level)
	// Narrow, the button is its icon, still named for what it shows.
	label := name
	if narrow {
		label = ""
	}
	kit.MenuPicker(c, kit.IconShieldCheck, label, name, 160, func(m *kit.Menu) {
		m.Item("Approvals for this project").Disabled(true)
		for _, l := range logic.Levels {
			if m.Item(l.Name).ID(l.ID).Subtitle(l.Hint).Radio(l.ID == level).Chosen() {
				id := l.ID
				s.st.Actions.SetLevel(p, ch, id, func(err error) {
					if err != nil {
						s.st.Error(err)
						return
					}
					s.st.Projects.SetLevel(p, id)
				})
			}
		}
	})
}

func containsAlias(as []string, m string) bool {
	for _, a := range as {
		if a == m {
			return true
		}
	}
	return false
}

// composerNarrow is the input group's width under which the pickers make
// room (Tailwind's @max-sm container query: 24rem).
const composerNarrow = 384
