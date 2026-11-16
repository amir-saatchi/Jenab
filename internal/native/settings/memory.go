package settings

import (
	"fmt"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/native/settings/logic"
	"github.com/amir-saatchi/jenab/internal/native/state"
)

// MemoryEditor is a memory in its settings (SPEC 3.3, memory.tsx): user
// memory in Settings, project memory in the project's settings. Each
// section is edited on its own; empty text deletes it. When a chat changed
// the section since it was opened, the save is refused with the section
// as it is now, to merge by hand. A chat chip's [edit] opens its section
// (Stores.OpenMemory).
type MemoryEditor struct {
	// Scope is "user" or "project"; Project is the project of a project
	// memory, whose chips alone open its sections.
	Scope   string
	Project id.Project
	// Load reads the memory and Save saves a section, each giving its
	// result to then on the UI thread, as state.Pages' do.
	Load  func(then func(app.MemoryView, error))
	Save  func(e app.MemoryEdit, then func(app.MemorySaved, error))
	Error func(error)

	m     logic.Memory
	focus string // a section a chip asked to open, for the next read
	seq   int
}

// Reload reads the memory again, as the page opens.
func (e *MemoryEditor) Reload() {
	e.seq++
	seq, focus := e.seq, e.focus
	e.focus = ""
	e.Load(func(v app.MemoryView, err error) {
		if seq != e.seq {
			return
		}
		if err != nil {
			e.Error(err)
			return
		}
		e.m.Loaded(v, focus)
	})
}

// takeFocus opens the section a chip asked for in this memory, and clears
// the ask, so a later visit opens no section.
func (e *MemoryEditor) takeFocus(st *state.Stores) {
	f := st.Shell.MemoryFocus
	if f == nil {
		return
	}
	section, ok := logic.FocusSection(f.Target, f.Project, e.Scope, e.Project)
	if !ok {
		return
	}
	st.Shell.MemoryFocus = nil
	e.focus = section
	e.Reload()
}

func (e *MemoryEditor) submit(d logic.Draft, content string) {
	if e.m.Busy {
		return
	}
	e.m.Busy = true
	e.Save(d.Edit(content), func(r app.MemorySaved, err error) {
		e.m.Busy = false
		if err != nil {
			e.Error(err)
			return
		}
		e.m.Saved(d, content, r)
	})
}

// View builds the editor; st gives the chips' asks.
func (e *MemoryEditor) View(c *kit.Context, st *state.Stores) {
	e.takeFocus(st)
	k := kit.TokensOf(c)
	m := &e.m
	kit.Column(c).Gap(12).AlignItems(kit.Stretch).Children(func() {
		if v := m.View; v != nil {
			line, f := logic.Used(*v)
			kit.Row(c).Gap(12).AlignItems(kit.Center).Children(func() {
				kit.Meter(c, f, "Memory used").Width(160)
				kit.Muted(c, line)
			})
		}
		if m.Empty() {
			kit.Text(c, "Nothing yet. Chats save here what you ask them to remember.").FontSize(14).TextColor(k.MutedForeground)
		}
		for _, s := range m.Sections() {
			box(c, "Section "+s.Section, func() {
				kit.Row(c).Gap(8).AlignItems(kit.Center).Children(func() {
					kit.Text(c, s.Section).FontSize(14).FontWeight(500)
					kit.Muted(c, fmt.Sprintf("revision %d", s.Revision))
					if !m.Editing(s.Section) {
						kit.Spacer(c)
						b := kit.Disable(kit.SmallButton(c, nil, "Edit", kit.Ghost).Label("Edit "+s.Section), m.Draft != nil)
						if b.Clicked() && m.Draft == nil {
							m.Edit(s)
						}
					}
				})
				if m.Editing(s.Section) {
					e.editor(c, m.Draft)
				} else {
					kit.Text(c, s.Content).FontSize(14)
				}
			})
		}
		if d := m.Draft; d != nil && (d.New || m.Orphan()) {
			box(c, "New section", func() {
				if m.Orphan() {
					kit.Text(c, d.Section).FontSize(14).FontWeight(500)
				}
				e.editor(c, d)
			})
		} else {
			b := kit.Disable(kit.SmallButton(c, kit.IconPlus, "Add section", kit.Outline).AlignSelf(kit.Start), m.View == nil || m.Draft != nil)
			if b.Clicked() && m.View != nil && m.Draft == nil {
				m.Add()
			}
		}
	})
}

// box is a section's bordered box.
func box(c *kit.Context, label string, fn func()) kit.Element {
	return kit.Column(c).Gap(8).Padding(12).Radius(kit.Radius).Border(1, kit.TokensOf(c).Border).AlignItems(kit.Stretch).
		Label(label).Children(fn)
}

func (e *MemoryEditor) editor(c *kit.Context, d *logic.Draft) {
	m := &e.m
	kit.Column(c).Gap(12).AlignItems(kit.Stretch).Children(func() {
		if d.New {
			field(c, "Section name", func() {
				in := kit.TextInput(c, &d.Section, "Section name").Placeholder("such as preferences")
				if len([]rune(d.Section)) > 40 {
					d.Section = string([]rune(d.Section)[:40])
				}
				if d.Section == "" && !in.Focused() {
					in.Focus()
				}
			})
		}
		if cf := m.Conflict; cf != nil {
			text := logic.ConflictText(*cf)
			if cf.Content != "" {
				text = cf.Content + "\n\n" + text
			}
			kit.Alert(c, kit.Amber, logic.ConflictTitle(*cf), text)
		}
		field(c, "Text", func() {
			kit.TextArea(c, &d.Content, "Text").Lines(4, 16)
		})
		kit.Row(c).Gap(8).AlignItems(kit.Center).Children(func() {
			if kit.Disable(kit.SmallButton(c, nil, "Save", kit.Default), !m.CanSave()).Clicked() && m.CanSave() {
				e.submit(*d, d.Content)
			}
			if cf := m.Conflict; cf != nil {
				if kit.Disable(kit.SmallButton(c, nil, logic.ResolveLabel(*cf), kit.Outline), m.Busy).Clicked() && !m.Busy {
					m.Resolve()
				}
			}
			if kit.Disable(kit.SmallButton(c, nil, "Cancel", kit.Ghost), m.Busy).Clicked() && !m.Busy {
				m.Close()
			}
			if !d.New && !m.Orphan() {
				kit.Spacer(c)
				b := kit.Disable(kit.SmallButton(c, nil, "Delete section", kit.Ghost).TextColor(kit.TokensOf(c).Destructive), m.Busy)
				if b.Clicked() && !m.Busy {
					e.submit(*d, "")
				}
			}
		})
	})
}

// field is a labeled field: its label over what fn builds.
func field(c *kit.Context, label string, fn func()) {
	kit.Column(c).Gap(6).AlignItems(kit.Stretch).Children(func() {
		kit.Text(c, label).FontSize(14).FontWeight(500)
		fn()
	})
}

// memoryPage is Settings › Memory: user memory, for all projects.
func (s *Screen) memoryPage(c *kit.Context) {
	kit.SettingsSection(c, "Memory", "What every chat in every project knows about you: your goals, preferences and decisions. Keep values that change, such as prices, in tables.", func() {
		MemoryScope(c, s.st, OpenProject(s.st))
		s.memory.View(c, s.st)
	})
}

// MemoryScope says whose memory Settings › Memory is (SPEC 3.3): every
// project's. The open project's own memory is in its settings, linked.
func MemoryScope(c *kit.Context, st *state.Stores, p *app.ProjectItem) {
	k := kit.TokensOf(c)
	muted := func(s string) { kit.Text(c, s).FontSize(14).TextColor(k.MutedForeground) }
	if p == nil {
		muted(logic.ScopeLine + " " + logic.ScopeNoProject)
		return
	}
	kit.Row(c).Gap(4).AlignItems(kit.Baseline).Wrap().Children(func() {
		muted(logic.ScopeLine)
		kit.Text(c, p.Name).FontSize(14).TextColor(k.MutedForeground)
		muted(logic.ScopeHasOwn)
		if kit.Link(c, logic.ScopeLinkLabel).Clicked() {
			OpenProjectSettings(st, p.ID, "Memory")
		}
		muted(logic.ScopeLinkFinish)
	})
}
