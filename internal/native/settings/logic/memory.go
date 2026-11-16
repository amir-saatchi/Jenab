package logic

import (
	"slices"
	"strings"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/store"
)

// Draft is a memory section being edited: its name ("" until a new one is
// named), the text and the revision the edit started from.
type Draft struct {
	Section  string
	Content  string
	Revision int
	New      bool
}

// Memory is a memory in its settings (SPEC 3.3), as memory.tsx's
// MemoryEditor keeps it: user memory in Settings, project memory in the
// project's settings. Each section is edited on its own; empty text
// deletes it. When a chat changed the section since it was opened, the
// save is refused with the section as it is now (Conflict), to merge by
// hand.
type Memory struct {
	View     *app.MemoryView
	Draft    *Draft
	Conflict *store.MemorySection
	Busy     bool
}

// Loaded takes the memory as read. focus is a section a chat's chip asked
// to open ("" for none); a section already being edited keeps its
// unsaved text.
func (m *Memory) Loaded(v app.MemoryView, focus string) {
	m.View = &v
	if focus == "" || m.Draft != nil {
		return
	}
	if s, ok := m.section(focus); ok {
		m.Edit(s)
	}
}

func (m *Memory) section(name string) (store.MemorySection, bool) {
	if m.View == nil {
		return store.MemorySection{}, false
	}
	i := slices.IndexFunc(m.View.Sections, func(s store.MemorySection) bool { return s.Section == name })
	if i < 0 {
		return store.MemorySection{}, false
	}
	return m.View.Sections[i], true
}

// Sections are the live sections.
func (m *Memory) Sections() []store.MemorySection {
	if m.View == nil {
		return nil
	}
	return m.View.Sections
}

// Edit opens a section for editing.
func (m *Memory) Edit(s store.MemorySection) {
	m.Draft = &Draft{Section: s.Section, Content: s.Content, Revision: s.Revision}
}

// Add opens a new section to name and write.
func (m *Memory) Add() { m.Draft = &Draft{New: true} }

// Close drops the draft and the conflict.
func (m *Memory) Close() { m.Draft, m.Conflict = nil, nil }

// Editing tells whether the section is open in the editor in its place.
func (m *Memory) Editing(section string) bool {
	return m.Draft != nil && !m.Draft.New && m.Draft.Section == section
}

// Orphan tells whether the section being edited was deleted elsewhere
// meanwhile: its editor stays, below the others, so the user can save it
// again or let it go.
func (m *Memory) Orphan() bool {
	if m.Draft == nil || m.Draft.New {
		return false
	}
	_, ok := m.section(m.Draft.Section)
	return !ok
}

// Empty tells whether to say there is nothing yet: the memory is read,
// has no section, and no new or orphan section is being written.
func (m *Memory) Empty() bool {
	return m.View != nil && len(m.View.Sections) == 0 && !(m.Draft != nil && (m.Draft.New || m.Orphan()))
}

// CanSave tells whether Save may run: a named section, and no save under
// way.
func (m *Memory) CanSave() bool {
	return m.Draft != nil && !m.Busy && strings.TrimSpace(m.Draft.Section) != ""
}

// Edit is the save of the draft with content, "" to delete the section.
func (d Draft) Edit(content string) app.MemoryEdit {
	return app.MemoryEdit{Section: d.Section, Content: content, Revision: d.Revision}
}

// Saved takes a save's result for the draft saved with content: a
// conflict keeps the draft at the section's revision now, with the
// section shown to merge; otherwise the editor closes.
func (m *Memory) Saved(d Draft, content string, r app.MemorySaved) {
	m.View = &r.Memory
	if r.Conflict != nil {
		c := *r.Conflict
		m.Conflict = &c
		d.Content, d.Revision = content, c.Revision
		m.Draft = &d
		return
	}
	m.Close()
}

// Resolve is "Use theirs" while the section changed elsewhere, taking its
// text into the draft, and "Let it go" while it was deleted, closing the
// editor.
func (m *Memory) Resolve() {
	c := m.Conflict
	m.Conflict = nil
	if c == nil {
		return
	}
	if c.Content == "" {
		m.Close()
		return
	}
	if m.Draft != nil {
		m.Draft.Content = c.Content
	}
}

// ConflictTitle and ConflictText say what happened to a section that
// changed, or was deleted, since it was opened.
func ConflictTitle(c store.MemorySection) string {
	if c.Content != "" {
		return "This section changed since you opened it"
	}
	return "This section was deleted since you opened it"
}

func ConflictText(c store.MemorySection) string {
	if c.Content != "" {
		return "Merge it into your text and save again, or use it as it is."
	}
	return "Save to keep your text, or let it go."
}

// ResolveLabel is the button that takes the other side of a conflict.
func ResolveLabel(c store.MemorySection) string {
	if c.Content != "" {
		return "Use theirs"
	}
	return "Let it go"
}

// Used is the memory's size line: "About 1,234 of 4,000 tokens", and how
// full it is, from 0 to 1.
func Used(v app.MemoryView) (string, float64) {
	f := 0.0
	if v.Cap > 0 {
		f = min(1, float64(v.Tokens)/float64(v.Cap))
	}
	return "About " + Thousands(v.Tokens) + " of " + Thousands(v.Cap) + " tokens", f
}

// FocusSection is the section a chip's [edit] asks to open in a memory of
// scope ("user" or "project"), for project memory only the project's own:
// target is memory:<scope>/<section>.
func FocusSection(target string, asked id.Project, scope string, project id.Project) (string, bool) {
	prefix := "memory:" + scope + "/"
	if !strings.HasPrefix(target, prefix) || (scope == "project" && asked != project) {
		return "", false
	}
	return target[len(prefix):], true
}

// The words around the user memory's scope line (SPEC 3.3, TASK-050):
// with a project open, its name and a link to its memory go between them.
const (
	ScopeLine       = "This memory is for every project."
	ScopeNoProject  = "Each project has its own memory too, in its Project settings."
	ScopeHasOwn     = "has its own memory too, in"
	ScopeLinkLabel  = "Project settings › Memory"
	ScopeLinkFinish = "."
)
