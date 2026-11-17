package settings

import (
	"strings"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/native/settings/logic"
	"github.com/amir-saatchi/jenab/internal/native/state"
)

// ConnectForm connects a provider with the user's own key (SPEC 3.9,
// provider-card.tsx): a provider from the list, or any OpenAI-compatible
// URL, the base URL's placeholders and the key, checked with the provider
// and stored in the OS keychain. With Replace set it saves a new key for
// a connected provider, whose models stay.
//
// To show one, as Settings › Models and the chat's Connect card do: make a
// new form each time its dialog or card opens, call View in each frame it
// shows, and call Clear when it closes or is cancelled, so the typed key
// and placeholder values are dropped. Connected may still arrive after
// Clear, for a connect that was already sent; check that the form is
// still the one shown before acting on it.
type ConnectForm struct {
	Replace *logic.Replace
	// Connected is called after a connect that worked; nil for none.
	Connected func(app.ConnectResult)

	asked   bool
	presets []app.PresetItem
	loaded  bool
	failed  string // why the presets couldn't be read
	pick    string
	key     string
	name    string
	url     string
	fields  map[string]string
	busy    bool
}

func (f *ConnectForm) load(st *state.Stores) {
	if f.asked {
		return
	}
	f.asked = true
	if f.Replace != nil {
		f.loaded = true
		return
	}
	st.Pages.Presets(func(ps []app.PresetItem, err error) {
		if err != nil {
			f.failed = state.ErrorText(err)
			return
		}
		f.presets, f.pick, f.loaded = ps, logic.FirstPick(ps), true
	})
}

// Clear drops what was typed that is secret: the key and the base URL's
// placeholder values. Call it when the form's dialog or card closes or is
// cancelled. The map stays, as a frame may still fill it.
func (f *ConnectForm) Clear() {
	f.key = ""
	clear(f.fields)
}

// removes tells whether saving removes the key of a provider on this
// computer: its optional key is left empty (TASK-074).
func (f *ConnectForm) removes(shape logic.FormShape) bool {
	return f.Replace != nil && shape.OptionalKey && strings.TrimSpace(f.key) == ""
}

// ready tells whether the form can be sent.
func (f *ConnectForm) ready(shape logic.FormShape) bool {
	if f.busy || !shape.Chosen {
		return false
	}
	if shape.Other && (strings.TrimSpace(f.name) == "" || strings.TrimSpace(f.url) == "") {
		return false
	}
	if shape.Key && strings.TrimSpace(f.key) == "" {
		return false
	}
	if f.removes(shape) && !f.Replace.Keyed {
		return false // no key to remove
	}
	for _, w := range shape.Fields {
		if strings.TrimSpace(f.fields[w]) == "" {
			return false
		}
	}
	return true
}

func (f *ConnectForm) request(st *state.Stores, shape logic.FormShape) app.ConnectRequest {
	switch {
	case f.Replace != nil:
		return app.ConnectRequest{Name: f.Replace.Name, Kind: f.Replace.Kind, BaseURL: f.Replace.BaseURL, Key: f.key, Fields: f.fields}
	case shape.Other:
		return app.ConnectRequest{Name: strings.TrimSpace(f.name), Kind: "openai_compatible", BaseURL: strings.TrimSpace(f.url), Key: f.key, Fields: map[string]string{}}
	}
	var taken map[string]bool
	if v := st.Settings.View; v != nil {
		taken = map[string]bool{}
		for name := range v.Settings.LLM.Providers {
			taken[name] = true
		}
	}
	p := shape.Preset
	return app.ConnectRequest{Name: logic.FreeName(p.ID, taken), Kind: p.Kind, BaseURL: p.BaseURL, Key: f.key, Fields: f.fields}
}

func (f *ConnectForm) connect(st *state.Stores, shape logic.FormShape) {
	f.busy = true
	req := f.request(st, shape)
	removed := f.removes(shape)
	st.Settings.Connect(req, func(r app.ConnectResult, err error) {
		f.busy = false
		if err != nil {
			st.Error(err)
			return
		}
		f.Clear()
		name := r.Provider
		if f.Replace == nil && !shape.Other && shape.Preset != nil {
			name = shape.Preset.Name
		}
		text := logic.ConnectedText(name, r, f.Replace != nil)
		if removed {
			text = logic.KeyRemovedText(name)
		}
		st.Toasts = append(st.Toasts, state.Toast{Text: text})
		if f.Connected != nil {
			f.Connected(r)
		}
	})
}

// View builds the form.
func (f *ConnectForm) View(c *kit.Context, st *state.Stores) {
	f.load(st)
	if f.failed != "" {
		kit.Column(c).Gap(12).AlignItems(kit.Stretch).Children(func() {
			kit.Alert(c, kit.Red, "The providers couldn't be read", f.failed)
			if kit.Button(c, "Try again", kit.Outline).AlignSelf(kit.Start).Clicked() {
				f.asked, f.failed = false, ""
			}
		})
		return
	}
	if !f.loaded {
		kit.Spinner(c, "Loading the providers")
		return
	}
	if f.fields == nil {
		f.fields = map[string]string{}
	}
	shape := logic.FormFor(f.pick, f.presets, f.Replace).WithURL(f.url)
	kit.Column(c).Gap(16).AlignItems(kit.Stretch).Children(func() {
		if f.Replace == nil {
			field(c, "Provider", func() {
				label := "Choose a provider"
				switch {
				case shape.Other:
					label = "OpenAI-compatible…"
				case shape.Preset != nil:
					label = shape.Preset.Name
				}
				kit.MenuButton(c, label, func(m *kit.Menu) {
					for _, g := range logic.ProviderGroups(f.presets) {
						m.Item(g.Label).Disabled(true)
						for _, ch := range g.Items {
							if m.Item(ch.Name).ID(ch.ID).Subtitle(ch.Hint).Radio(ch.ID == f.pick).Chosen() && ch.ID != f.pick {
								// A key or account ID typed for one provider
								// never goes to another one's URL.
								f.pick = ch.ID
								f.Clear()
							}
						}
					}
				}).Label("Provider").FillWidth()
			})
		}
		if shape.Other {
			field(c, "Name", func() {
				kit.TextInput(c, &f.name, "Name").Placeholder("my-provider")
			})
			field(c, "Base URL", func() {
				kit.TextInput(c, &f.url, "Base URL").Placeholder("https://example.com/v1/").Direction(kit.LeftToRight)
				kit.Hint(c, "Any OpenAI-compatible API.")
			})
		}
		for _, w := range shape.Fields {
			label := logic.FieldLabel(w)
			field(c, label, func() {
				v := f.fields[w]
				kit.TextInput(c, &v, label)
				f.fields[w] = v
				kit.Hint(c, "Part of the base URL · stored in the OS keychain")
			})
		}
		if shape.Key {
			field(c, "API key", func() {
				kit.TextInput(c, &f.key, "API key").Password()
				hint := "Stored in the OS keychain · also in Settings → Models"
				if f.Replace != nil {
					hint = "Checked with the provider, then stored in the OS keychain"
				}
				kit.Hint(c, hint)
			})
		} else if shape.OptionalKey {
			field(c, "API key (optional)", func() {
				kit.TextInput(c, &f.key, "API key (optional)").Password()
				hint := "Only if the server asks for a token · stored in the OS keychain"
				if f.Replace != nil && f.Replace.Keyed {
					hint = "Leave it empty to remove the saved key · checked with the server first"
				}
				kit.Hint(c, hint)
			})
		}
		if p := shape.Preset; p != nil && p.NoKey {
			text := "No key is needed. Start " + p.Name + " on this computer first."
			if p.Running {
				text = p.Name + " runs on this computer; no key is needed."
			}
			kit.Hint(c, text)
		}
		label := "Connect"
		switch {
		case f.removes(shape) && f.Replace.Keyed:
			label = "Remove key"
		case f.Replace != nil:
			label = "Save key"
		}
		ok := f.ready(shape)
		b := kit.Disable(kit.Button(c, label, kit.Default), !ok).AlignSelf(kit.Start)
		if f.busy {
			b.Label(label + ", working")
		}
		if b.Clicked() && ok {
			f.connect(st, shape)
		}
	})
}
