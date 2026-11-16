package settings

import (
	"strings"
	"time"

	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/native/settings/logic"
	"github.com/amir-saatchi/jenab/internal/native/state"
	"github.com/amir-saatchi/jenab/internal/project"
)

// checkWait is how long the folder dialog waits after typing before it
// checks the folder.
var checkWait = 300 * time.Millisecond

// folderDialog picks a new data folder. It is saved now and used at the
// next start, when the projects move (SPEC 2.1).
type folderDialog struct {
	shown             bool
	dir, current      string
	fallback, checked string
	warning           *project.FolderWarning
	err               string
	busy              bool
	seq               int
}

func (d *folderDialog) open(current, fallback string) {
	*d = folderDialog{shown: true, dir: current, current: current, fallback: fallback, checked: current}
}

// check asks about the folder typed once typing pauses; only the latest
// answer shows.
func (d *folderDialog) check(s *Screen) {
	d.seq++
	seq, dir := d.seq, strings.TrimSpace(d.dir)
	d.checked = d.dir
	if dir == "" {
		return
	}
	go func() {
		time.Sleep(checkWait)
		s.post(func() {
			if seq != d.seq || !d.shown {
				return
			}
			s.st.Pages.CheckFolder(dir, func(w *project.FolderWarning, err error) {
				if seq != d.seq {
					return
				}
				d.warning, d.err = w, ""
				if err != nil {
					d.warning, d.err = nil, state.ErrorText(err)
				}
			})
		})
	}()
}

func (d *folderDialog) view(c *kit.Context, s *Screen) {
	if !d.shown {
		return
	}
	if d.dir != d.checked {
		d.check(s)
	}
	kit.Dialog(c, &d.shown, "Data folder", func() {
		kit.Text(c, "Where projects are kept. The change applies at the next start, when the projects move there.").
			FontSize(14).TextColor(kit.TokensOf(c).MutedForeground)
		kit.Column(c).Gap(8).Children(func() {
			kit.Text(c, "Folder").FontSize(14).FontWeight(500)
			kit.Row(c).Gap(8).Children(func() {
				in := kit.TextInput(c, &d.dir, "Folder").Grow(1).Shrink(1).Font(kit.FontMono).FontSize(12)
				if d.err != "" {
					in.Invalid(true)
				}
				if pick := s.st.Pages.PickFolder; pick != nil {
					if kit.Button(c, "Browse…", kit.Outline).Clicked() {
						start := d.dir
						if start == "" {
							start = d.fallback
						}
						if p := pick("Choose the data folder", start); p != "" {
							d.dir = p
						}
					}
				}
			})
			if d.err != "" {
				kit.Text(c, d.err).FontSize(14).TextColor(kit.TokensOf(c).Tone(kit.Red).Text)
			} else {
				kit.Hint(c, "Use a local disk, not a network drive or a synced folder. The default is "+d.fallback+".")
			}
		})
		if d.warning != nil {
			kit.Alert(c, kit.Amber, d.warning.Text, "")
		}
		kit.Row(c).Gap(8).Justify(kit.End).Children(func() {
			if kit.Button(c, "Cancel", kit.Outline).Clicked() {
				d.shown = false
			}
			dir := strings.TrimSpace(d.dir)
			off := d.busy || dir == "" || d.err != "" || logic.SamePath(dir, d.current)
			if kit.Disable(kit.Button(c, "Save", kit.Default), off).Clicked() && !off {
				d.busy = true
				value := dir
				if logic.SamePath(dir, d.fallback) {
					value = ""
				}
				s.wait(s.st.Settings.Edit(func(c *config.Settings) { c.DataFolder = value }), func(err error) {
					d.busy = false
					if err != nil {
						d.err = state.ErrorText(err)
						return
					}
					d.shown = false
				})
			}
		})
	})
}
