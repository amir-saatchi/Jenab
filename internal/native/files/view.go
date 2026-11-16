package files

import (
	"fmt"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/native/state"
)

// PictureHeight is the most a picture takes on screen, as the frontend's
// max-h-[70vh] in a window of about 700 px.
const PictureHeight = 480

// View is file key of project p opened in the main area, with chat ch in
// the dock (file-view.tsx): text and Markdown as in the chat, pictures,
// and for other types their name, type and size with *Open with the
// system app*. Its versions are listed under it. controls builds the
// dock's buttons at the header's end.
func (s *Screen) View(c *kit.Context, p id.Project, ch id.Chat, key string, controls func()) {
	st := s.st
	v := st.Files.View(p, key, st.Shell.Rev[p])
	o := v.Object
	kit.Bar(c, func() {
		kit.Icon(c, kit.IconFile).Size(16, 16).Shrink(0).TextColor(kit.TokensOf(c).MutedForeground)
		kit.Column(c).Shrink(1).MinWidth(0).Children(func() {
			kit.Title(c, state.NameOf(key))
			sub := "Files"
			switch {
			case o != nil && o.Title != "":
				sub = o.Title
			case state.ParentOf(key) != "":
				sub = state.ParentOf(key)
			}
			kit.Muted(c, sub)
		})
		kit.Spacer(c)
		kit.Row(c).Shrink(0).Gap(4).AlignItems(kit.Center).Children(func() {
			if o != nil {
				if button(c, kit.IconLink, "Copy link").Clicked() {
					kit.CopyText(c, state.LinkOf(p, key))
				}
				if o.Opens && button(c, kit.IconExternalLink, "Open with the system app").Clicked() {
					st.OpenWithSystem(p, key)
				}
				if button(c, kit.IconTrash, "Delete").Clicked() {
					st.DeleteFile(p, key)
				}
				kit.Separator(c, 20, true).Margin(0, 4)
			}
			if controls != nil {
				controls()
			}
		})
	})
	kit.Scroll(c).Grow(1).Shrink(1).MinHeight(0).Children(func() {
		kit.Column(c).Gap(24).Padding(24).Width(896).MaxWidthPercent(100).AlignSelf(kit.Center).AlignItems(kit.Stretch).Children(func() {
			switch {
			case v.NotFound:
				kit.Empty(c, nil, "This file no longer exists", "It was deleted or moved.", nil)
			case v.Err != "":
				kit.Empty(c, nil, "This file can't be shown", v.Err, nil)
			case o == nil:
				kit.Spinner(c, "Opening the file").AlignSelf(kit.Center)
			default:
				s.content(c, p, key, *o, v)
				versions(c, *o, v.Versions)
			}
		})
	})
}

// button is a header's icon button, its label as its tooltip.
func button(c *kit.Context, icon *kit.SVG, label string) kit.Element {
	return kit.Tooltip(kit.IconButton(c, icon, label, kit.Ghost).Size(32, 32), label)
}

func (s *Screen) content(c *kit.Context, p id.Project, key string, o app.Object, v *state.FileView) {
	switch kind := state.KindOf(o.MIME); kind {
	case state.KindMarkdown, state.KindText:
		s.text(c, p, key, o, v, kind == state.KindMarkdown)
		return
	case state.KindImage:
		// A picture too big to read whole is opened with the system app.
		if v.Data != nil || !v.Loaded {
			if !v.Loaded {
				kit.Spinner(c, "Reading the picture").AlignSelf(kit.Center)
				return
			}
			label := o.Title
			if label == "" {
				label = state.NameOf(key)
			}
			if _, ok := kit.Picture(c, o.SHA256, v.Data, label, PictureHeight); ok {
				return
			}
		}
	}
	// Audio and video aren't played on MyGo (v0.9.1 has no player): they
	// open with the system app, as the types the app doesn't show.
	s.other(c, p, o)
}

// text is the first state.TextLimit bytes of a text file. A cut in the
// middle of a character shows as one replacement character.
func (s *Screen) text(c *kit.Context, p id.Project, key string, o app.Object, v *state.FileView, markdown bool) {
	switch {
	case v.ReadErr != "":
		kit.Alert(c, kit.Red, "Can't show this file", v.ReadErr)
		return
	case !v.Loaded:
		kit.Spinner(c, "Reading the file").AlignSelf(kit.Center)
		return
	}
	if v.Cut {
		kit.Alert(c, kit.Amber, "Only the first "+state.FormatSize(state.TextLimit)+" is shown",
			"The file is "+state.FormatSize(o.Size)+". Open it with the system app to see all of it.")
	}
	text := string(v.Data)
	if !markdown {
		k := kit.TokensOf(c)
		kit.Box(c).Padding(16).Radius(kit.Radius).Border(1, k.Border).Background(k.Muted.Alpha(0.4)).Children(func() {
			kit.Pre(c, text).FontSize(14)
		})
		return
	}
	ref := state.FileRef{Project: p, Key: key}
	md := s.md[ref]
	if md == nil {
		md = &kit.Markdown{Own: s.own, CodeLink: state.CodeLink}
		s.md[ref] = md
	}
	md.View(c, state.MarkLinks(text))
}

// own opens the jenab:// links of a file, as the chat's.
func (s *Screen) own(url string) bool {
	if !state.IsLink(url) {
		return false
	}
	s.st.OpenLink(url, nil)
	return true
}

// other is a file the app doesn't show: its name, type and size, and
// *Open with the system app* for a type that is opened outside the app.
func (s *Screen) other(c *kit.Context, p id.Project, o app.Object) {
	k := kit.TokensOf(c)
	kit.Box(c).Radius(kit.Radius).Border(1, k.Border).Children(func() {
		kit.Empty(c, kit.IconFile, state.NameOf(o.Key), state.TypeName(o.MIME)+" · "+state.FormatSize(o.Size), func() {
			if o.Opens {
				if kit.Button(c, "Open with the system app", kit.Outline).Clicked() {
					s.st.OpenWithSystem(p, o.Key)
				}
				return
			}
			kit.Text(c, "This type isn't opened outside the app, since it could run a program.").FontSize(14).
				TextColor(k.MutedForeground)
		})
	})
}

// versions lists the file's versions, the newest first (SPEC 4.3).
func versions(c *kit.Context, o app.Object, vs []app.ObjectVersion) {
	if len(vs) == 0 {
		return
	}
	kit.Column(c).Gap(8).AlignItems(kit.Stretch).Children(func() {
		kit.Separator(c, 0, false).AlignSelf(kit.Stretch)
		kit.Heading(c, "Versions").FontSize(14).FontWeight(500)
		for _, ver := range vs {
			badge := ""
			if ver.Version == o.Version {
				badge = "Current"
			}
			when := kit.MediumDate(c, ver.CreatedAt.Local()) + ", " + kit.ShortTime(c, ver.CreatedAt.Local())
			sub := state.FormatSize(ver.Size) + " · by " + state.WhoText(ver.CreatedBy) + " · " + when
			c.Key(ver.Version)
			kit.Item(c, fmt.Sprintf("Version %d", ver.Version), badge, kit.Neutral, sub)
		}
	})
}
