package main

import (
	"fmt"

	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/native/state"
)

// shell is the window's placeholder for Part A: the projects in a
// sidebar, opening one opens its chat in the stores, and the chat shows
// its title and size. Part B puts the chat screen here.
type shell struct {
	stores   *state.Stores
	post     state.Post
	quit     func()
	selected string
}

// start loads what the window shows first, on the UI thread.
func (s *shell) start() {
	s.report(s.stores.Projects.Load())
	s.report(s.stores.Settings.Load())
}

func (s *shell) report(done <-chan error) {
	go func() {
		if err := <-done; err != nil {
			s.post(func() { s.stores.Error(err) })
		}
	}()
}

func (s *shell) view(c *kit.Context) {
	kit.Root(c, func() {
		for _, t := range s.stores.TakeToasts() {
			kind := kit.ToastInfo
			switch {
			case t.Error:
				kind = kit.ToastError
			case t.Warning:
				kind = kit.ToastWarning
			}
			kit.Toast(c, t.Text, kind, t.Sticky)
		}
		kit.Row(c).Grow(1).AlignItems(kit.Stretch).Children(func() {
			s.sidebar(c)
			kit.Column(c).Grow(1).Padding(24).Children(func() { s.main(c) })
		})
		kit.Toasts(c)
	})
}

func (s *shell) sidebar(c *kit.Context) {
	if p := s.stores.Nav.Current(); p != nil {
		s.selected = string(p.Project)
	}
	kit.Column(c).Width(260).AlignItems(kit.Stretch).Children(func() {
		changed := kit.Sidebar(c, &s.selected, func() {
			kit.SidebarSection(c, "Projects", nil, func() {
				for _, p := range s.stores.Projects.List {
					kit.SidebarItem(c, string(p.ID), kit.IconInbox, p.Name)
				}
			})
		}).Grow(1).Changed()
		if changed {
			s.stores.OpenProject(id.Project(s.selected), "")
		}
		kit.Row(c).Padding(8).Children(func() {
			if kit.Button(c, "Quit", kit.Ghost).Clicked() {
				s.quit()
			}
		})
	})
}

func (s *shell) main(c *kit.Context) {
	p := s.stores.Nav.Current()
	if !s.stores.Projects.Loaded {
		kit.Spinner(c, "Loading the projects")
		return
	}
	if p == nil || p.View != state.ViewChat {
		kit.Empty(c, kit.IconChat, "Choose a project", "Its chat opens here. The chat screen comes in SPIKE-048 Part B.", nil)
		return
	}
	t := s.stores.Threads.Threads[p.Chat]
	if t == nil {
		kit.Spinner(c, "Opening the chat")
		return
	}
	title := t.Chat.Title
	if title == "" {
		title = "Untitled chat"
	}
	kit.Empty(c, kit.IconChat, title, fmt.Sprintf("%d messages, %s", len(t.Messages), t.State), nil)
}
