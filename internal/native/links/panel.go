// Package links is the Links tab of the right sidebar on MyGo (SPEC 4.6,
// 5.12, P3-11): a project's saved links, the newest first, with a search
// over titles, hosts and tags, and the user's add, edit, copy, open and
// delete. The links are read through the services (state.Links).
package links

import (
	"strings"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/native/state"
)

// Screen shows the Links tab.
type Screen struct {
	st *state.Stores
	// drawn is the project whose tab this frame drew; "" when none.
	drawn id.Project
}

// New makes the screen on the stores.
func New(st *state.Stores) *Screen { return &Screen{st: st} }

// Panel is project p's Links tab: a search and an Add button over the
// list. A row opens its link in the system, when SafeLink allows it; its
// menu copies, edits or deletes it.
func (s *Screen) Panel(c *kit.Context, p id.Project) {
	st := s.st
	s.drawn = p
	v := st.Links.List(p)
	q := st.Links.Query[p]
	kit.Row(c).MinHeight(48).Shrink(0).AlignItems(kit.Center).Gap(4).Padding(0, 8, 0, 12).Children(func() {
		in := kit.TextInput(c, &q, "Search links").Placeholder("Search by title, site or tag").Grow(1).Shrink(1).MinWidth(0)
		if in.Changed() {
			st.Links.Query[p] = q
		}
		if kit.Tooltip(kit.IconButton(c, kit.IconPlus, "Add link", kit.Ghost).Size(28, 28), "Add link").Clicked() {
			st.Links.StartAdd(p)
		}
	})
	kit.Scroll(c).Grow(1).Shrink(1).MinHeight(0).Children(func() {
		kit.Column(c).Padding(0, 8, 8, 8).AlignItems(kit.Stretch).Children(func() {
			shown := state.FilterLinks(v.Links, q)
			switch {
			case v.Err != "":
				note(c, "Links can't be shown", v.Err)
				if kit.Button(c, "Try again", kit.Outline).AlignSelf(kit.Center).Clicked() {
					st.Links.Changed(p)
				}
			case !v.Loaded:
				kit.Spinner(c, "Loading the links").AlignSelf(kit.Center).Margin(16)
			case len(v.Links) == 0:
				note(c, "No links yet", "Links you add and links the agent saves for this project show here.")
			case len(shown) == 0:
				note(c, "No links match", "Search looks in the titles, the sites and the tags.")
			default:
				s.list(c, p, shown)
			}
		})
	})
	s.editor(c, p)
}

// EndFrame ends a frame of the window: the dialog closes when its tab
// wasn't drawn, as another tab, the sidebar hidden or another project
// took its place. The shell calls it after each View.
func (s *Screen) EndFrame() {
	if e := s.st.Links.Editor; e != nil && e.Project != s.drawn {
		s.st.Links.Detach()
	}
	s.drawn = ""
}

// list is the links as rows of a tree, so the arrows move between them
// and each has its menu.
func (s *Screen) list(c *kit.Context, p id.Project, links []app.Link) {
	kit.Tree(c, "Links", func(rows kit.TreeRows) {
		for _, lk := range links {
			safe := kit.SafeLink(lk.URL)
			face := kit.SavedLinkFace{Title: lk.Title, Host: state.LinkHost(lk.URL), Note: lk.Note, Tags: lk.Tags,
				Saved: "Saved by " + state.WhoText(lk.CreatedBy) + " · " + kit.MediumDate(c, lk.CreatedAt), Blocked: !safe}
			r := rows.Item(lk.ID, state.LinkTitle(lk), nil, false, func() { kit.SavedLink(c, face) },
				func(m *kit.Menu) { s.menu(c, m, p, lk, safe) })
			r.Tooltip(lk.URL)
			if r.Clicked() {
				s.open(c, lk)
			}
		}
	})
}

// open opens a link in the system; one SafeLink refuses says it doesn't.
func (s *Screen) open(c *kit.Context, lk app.Link) {
	if !kit.OpenLink(c, lk.URL) {
		s.st.Toasts = append(s.st.Toasts, state.Toast{Text: "This link doesn't open from the app.",
			Detail: "Only http, https and mailto links open. Copy it to use it elsewhere."})
	}
}

// menu is a link's actions: Open for a safe link, Copy link, Edit and
// Delete.
func (s *Screen) menu(c *kit.Context, m *kit.Menu, p id.Project, lk app.Link, safe bool) {
	if safe && m.Item("Open").Chosen() {
		s.open(c, lk)
	}
	if m.Item("Copy link").Chosen() {
		kit.CopyText(c, lk.URL)
	}
	if m.Item("Edit").Chosen() {
		s.st.Links.StartEdit(p, lk)
	}
	m.Separator()
	if m.Item("Delete").Chosen() {
		s.st.DeleteLink(p, lk)
	}
}

// editor is the dialog that adds or edits a link of p.
func (s *Screen) editor(c *kit.Context, p id.Project) {
	ls := s.st.Links
	e := ls.Editor
	if e == nil || e.Project != p {
		return
	}
	open := true
	title := "Add link"
	if e.ID != "" {
		title = "Edit link"
	}
	kit.Dialog(c, kit.Locked(&open, e.Busy), title, func() {
		kit.Field(c, "URL", "", e.Problem, func() {
			in := kit.TextInput(c, &e.URL, "URL").Placeholder("https://").Direction(kit.LeftToRight).AutoFocus()
			if in.Changed() {
				e.Problem = ""
			}
			if in.Submitted() {
				ls.Save()
			}
		})
		kit.Field(c, "Title", "", "", func() {
			if kit.TextInput(c, &e.Title, "Title").Submitted() {
				ls.Save()
			}
		})
		kit.Field(c, "Note", "", "", func() {
			kit.TextArea(c, &e.Note, "Note").Lines(2, 6)
		})
		kit.Field(c, "Tags", "Separate them with commas.", "", func() {
			if kit.TextInput(c, &e.Tags, "Tags").Submitted() {
				ls.Save()
			}
		})
		kit.DialogFooter(c, func() {
			if kit.Button(c, "Cancel", kit.Outline).Clicked() {
				ls.CloseEditor()
			}
			save := kit.ActionButton(c, nil, "Save", kit.Default, e.Busy)
			if kit.Disable(save, e.Busy || strings.TrimSpace(e.URL) == "").Clicked() {
				ls.Save()
			}
		})
	})
	if !open {
		ls.CloseEditor()
	}
}

// note is what the tab shows in place of the list.
func note(c *kit.Context, title, text string) {
	kit.Column(c).Gap(4).Padding(16).AlignItems(kit.Center).Children(func() {
		kit.Text(c, title).FontSize(14).FontWeight(500).TextAlign(kit.Center)
		kit.Text(c, text).FontSize(13).TextColor(kit.TokensOf(c).MutedForeground).TextAlign(kit.Center)
	})
}
