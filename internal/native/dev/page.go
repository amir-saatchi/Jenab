// Package dev is the developer tools on MyGo (SPEC 8.4): the turn
// inspector and the runtime panel, as the React app's dev/inspector,
// runtime and dev-page. They read state.Dev and show what they read with
// the kit; the formatting helpers (format.ts) are ported here with their
// tests.
//
// They are read-only, and every text the service returns has its secrets
// redacted. The React app showed them as wide pages of Settings; here they
// open from the rail, as one page with the two tools, while the
// developer tools are on.
package dev

import (
	"time"

	"github.com/amir-saatchi/jenab/internal/appinfo"
	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/native/state"
)

// Screen shows the developer tools in the main area.
type Screen struct {
	st  *state.Stores
	dev *state.Dev
	// Now is the clock for how long work has run; nil is time.Now.
	Now func() time.Time

	// starting is the read of what this start used while it runs. The
	// stores fill it on the UI thread, so View takes it there.
	starting  <-chan error
	chosen    map[string]int // the request chosen, per turn
	blockOpen bool           // the block text's dialog
	lines     kit.ListState  // the block text's place

	// The pickers' values, and what they were set to from the pick: a
	// value that differs was chosen in the last frame.
	chatValue, chatShown string
	turnValue, turnShown string
}

// New makes the screen on the stores and the developer tools' store.
func New(st *state.Stores, dev *state.Dev) *Screen {
	return &Screen{st: st, dev: dev, chosen: map[string]int{}}
}

func (s *Screen) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// View builds the developer tool at place: the inspector or the runtime.
func (s *Screen) View(c *kit.Context, place *state.Place) {
	started := s.st.Settings.Started
	if s.starting != nil {
		select {
		case <-s.starting:
			s.starting = nil
		default:
		}
	}
	if started == nil && s.starting == nil {
		s.starting = s.st.Settings.LoadStarted()
	}
	on := started != nil && started.DevTools
	title, description := "Runtime", "What runs now. Reads again every second."
	if place.View == state.ViewInspector {
		title, description = "Turn inspector", "What the context builder sent, and what came back."
	}
	page := kit.Column(c).Grow(1).Shrink(1).MinHeight(0).AlignItems(kit.Stretch)
	// A place opened afresh, or come back to, starts its tool afresh, as
	// the React views mount again.
	opened := kit.Local(page, "place", func() *state.Place { return nil })
	if on && (*opened == nil || !state.SamePlace(*opened, place)) {
		p := *place
		*opened = &p
		s.open(place)
	}
	page.Children(func() {
		s.bar(c, place.View)
		kit.Scroll(c).Grow(1).Children(func() {
			kit.Column(c).Padding(24).Gap(16).AlignItems(kit.Stretch).MaxWidth(1200).Children(func() {
				kit.Row(c).Gap(16).AlignItems(kit.Start).Children(func() {
					kit.Column(c).Gap(4).Grow(1).Shrink(1).MinWidth(0).Children(func() {
						kit.Heading(c, title)
						kit.Muted(c, description).FontSize(14)
					})
					if on && place.View == state.ViewInspector {
						s.pickers(c)
					}
				})
				switch {
				case started == nil:
					kit.Spinner(c, "Loading").AlignSelf(kit.Start)
				case !on:
					off(c)
				case place.View == state.ViewInspector:
					s.inspector(c)
				default:
					s.runtime(c)
				}
			})
		})
	})
	s.blockDialog(c)
}

func (s *Screen) open(place *state.Place) {
	if place.View == state.ViewRuntime {
		s.dev.OpenRuntime()
		return
	}
	var at *state.TurnRef
	if place.Project != "" && place.Chat != "" && place.Turn > 0 {
		at = &state.TurnRef{Project: place.Project, Chat: place.Chat, Turn: place.Turn}
	}
	s.dev.OpenInspector(at)
}

// bar is the page's top bar: the two tools, and the badge saying they
// are read-only.
func (s *Screen) bar(c *kit.Context, view state.View) {
	kit.Bar(c, func() {
		kit.Icon(c, kit.IconWrench).Size(16, 16).Shrink(0)
		kit.Title(c, "Developer tools").PaddingEnd(8)
		for _, t := range []struct {
			view  state.View
			label string
			icon  *kit.SVG
		}{{state.ViewInspector, "Turn inspector", kit.IconSearch}, {state.ViewRuntime, "Runtime", kit.IconActivity}} {
			v := kit.Ghost
			if t.view == view {
				v = kit.Secondary
			}
			b := kit.SmallButton(c, t.icon, t.label, v).Height(kit.BarButton).PaddingX(12).FontSize(14).FocusRingOffset(0)
			if t.view == view {
				b.Current(kit.CurrentPage)
			}
			if b.Clicked() && t.view != view {
				s.st.Nav.Go(state.Place{View: t.view})
			}
		}
		kit.Spacer(c)
		kit.Badge(c, "Read-only · secrets redacted", kit.Neutral).Shrink(0)
		// At the window's top end corner, unless the narrow window's
		// header is over it (TASK-063).
		if !kit.Narrow(c) {
			kit.ControlsRoom(c)
		}
	})
}

// off is the page while the developer tools are off at this start.
func off(c *kit.Context) {
	framed(c, func() {
		kit.Empty(c, kit.IconWrench, "The developer tools are off",
			"Turn them on in Settings → Developer. They apply at the next start of "+appinfo.Name+".", nil)
	})
}

// framed is an Empty's frame, as className="border" on shadcn's Empty.
func framed(c *kit.Context, fn func()) {
	k := kit.TokensOf(c)
	kit.Column(c).Radius(kit.Radius*1.2).Border(1, k.Border).AlignItems(kit.Stretch).Children(fn)
}
