// Package files is the bucket's screens on MyGo (SPEC 5.12): the Files
// tab of the right sidebar and a file opened in the main area, as
// the React app's shell/files-panel and file-view. The files are read
// through the services (state.Files), not the React app's /objects route.
package files

import (
	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/native/state"
)

// Screen shows the bucket's screens. It keeps what a file's view needs
// between frames: its Markdown.
type Screen struct {
	st *state.Stores
	md map[state.FileRef]*kit.Markdown
}

// New makes the screens on the stores.
func New(st *state.Stores) *Screen {
	return &Screen{st: st, md: map[state.FileRef]*kit.Markdown{}}
}

// Panel is the Files tab (files-panel.tsx): one folder of project p's
// bucket with a path bar back to the top. A folder opens in place; a file
// opens in the main area, next to the open chat. It loads 100 entries at a
// time as the list scrolls. cache/, the app's own folder, is listed apart
// as *App cache*.
func (s *Screen) Panel(c *kit.Context, p id.Project) {
	st := s.st
	sh := st.Shell
	prefix := sh.Folder[p]
	v := st.Files.Folder(p, prefix, sh.Rev[p])
	kit.Row(c).MinHeight(48).Shrink(0).AlignItems(kit.Center).Gap(4).Padding(0, 8, 0, 12).Children(func() {
		crumbs := state.Crumbs(prefix)
		labels := make([]string, len(crumbs))
		for i, cr := range crumbs {
			labels[i] = cr.Label
		}
		chosen := -1
		if kit.PathBar(c, labels, &chosen).Changed() && chosen >= 0 {
			sh.SetFolder(p, crumbs[chosen].Prefix)
		}
		kit.Spacer(c)
		if kit.Tooltip(kit.IconButton(c, kit.IconRefresh, "Refresh", kit.Ghost).Size(28, 28), "Refresh").Clicked() {
			st.Files.Reload(p)
		}
	})
	kit.Scroll(c).Grow(1).Shrink(1).MinHeight(0).Children(func() {
		kit.Column(c).Padding(0, 8, 8, 8).AlignItems(kit.Stretch).Children(func() {
			switch f := v.Folder; {
			case v.Err != "":
				note(c, "Files can't be shown", v.Err)
			case f == nil:
				kit.Spinner(c, "Loading the files").AlignSelf(kit.Center).Margin(16)
			case len(f.Folders)+len(f.Objects) == 0 && prefix == "":
				note(c, "No files yet", "Files you upload and files the agent saves show here.")
			case len(f.Folders)+len(f.Objects) == 0:
				note(c, "This folder is empty", "Its files were deleted or moved.")
			default:
				s.entries(c, p, f, v.Loading)
			}
		})
	})
	if prefix == "" && v.HasCache {
		kit.Column(c).Shrink(0).Padding(8).AlignItems(kit.Stretch).Children(func() {
			kit.Group(c, "App")
			row := kit.NavItem(c, "App cache", false, func() {
				kit.Icon(c, kit.IconDatabase).Size(16, 16).Shrink(0)
				kit.Lines(c, "App cache", "Fetched pages and tool outputs, kept 30 days", kit.Neutral)
			})
			if row.Clicked() {
				sh.SetFolder(p, state.Cache)
			}
		})
	}
}

// entries are a folder's folders and files, and a row that loads more as
// it comes near the view.
func (s *Screen) entries(c *kit.Context, p id.Project, f *state.Folder, loading bool) {
	st := s.st
	var chat id.Chat
	var active string
	if pl := st.Nav.Current(); pl != nil && pl.View == state.ViewChat && pl.Project == p {
		chat = pl.Chat
		if d, ok := st.Shell.Open[state.ChatRef{Project: p, Chat: chat}]; ok && d.Kind == "file" {
			active = d.Key
		}
	}
	kit.Tree(c, "Files", func(rows kit.TreeRows) {
		for _, folder := range f.Folders {
			name := state.NameOf(folder)
			r := rows.Item(folder, name, nil, false, func() {
				kit.Icon(c, kit.IconFolder).Size(16, 16).Shrink(0)
				kit.Title(c, name).FontWeight(400).FontSize(14)
			}, nil)
			if r.Clicked() {
				st.Shell.SetFolder(p, folder)
			}
		}
		for _, o := range f.Objects {
			s.fileRow(c, rows, p, chat, o, o.Key == active)
		}
	})
	if f.Next != "" {
		more := kit.Row(c).Justify(kit.Center).Padding(8).Children(func() { kit.Spinner(c, "Loading more files") })
		if more.InViewMargin(400) && !loading {
			st.Files.More(p)
		}
	}
}

func (s *Screen) fileRow(c *kit.Context, rows kit.TreeRows, p id.Project, chat id.Chat, o app.Object, active bool) {
	st := s.st
	name := state.NameOf(o.Key)
	icon := kit.IconFile
	switch state.KindOf(o.MIME) {
	case state.KindImage:
		icon = kit.IconFileImage
	case state.KindText, state.KindMarkdown:
		icon = kit.IconFileText
	}
	r := rows.Item(o.Key, name, nil, active, func() {
		kit.Icon(c, icon).Size(16, 16).Shrink(0)
		kit.Lines(c, name, o.Title, kit.Neutral)
	}, func(m *kit.Menu) { s.fileMenu(c, m, p, o) })
	r.Tooltip(o.Key)
	if r.Clicked() && chat != "" {
		st.OpenFile(p, chat, o.Key)
	}
}

// fileMenu is a file's actions: Copy link, Open with the system app for a
// type that opens outside the app, and Delete.
func (s *Screen) fileMenu(c *kit.Context, m *kit.Menu, p id.Project, o app.Object) {
	if m.Item("Copy link").Chosen() {
		kit.CopyText(c, state.LinkOf(p, o.Key))
	}
	if o.Opens && m.Item("Open with the system app").Chosen() {
		s.st.OpenWithSystem(p, o.Key)
	}
	m.Separator()
	if m.Item("Delete").Chosen() {
		s.st.DeleteFile(p, o.Key)
	}
}

// note is what the panel shows in place of entries.
func note(c *kit.Context, title, text string) {
	kit.Column(c).Gap(4).Padding(16).AlignItems(kit.Center).Children(func() {
		kit.Text(c, title).FontSize(14).FontWeight(500).TextAlign(kit.Center)
		kit.Text(c, text).FontSize(13).TextColor(kit.TokensOf(c).MutedForeground).TextAlign(kit.Center)
	})
}
