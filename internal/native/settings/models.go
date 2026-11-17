package settings

import (
	"slices"
	"strconv"
	"strings"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/native/settings/logic"
)

// modelsPage is what Settings › Models has open: the connect form, a
// provider's new key, its models, or its removal.
type modelsPage struct {
	adding    bool
	add       *ConnectForm
	replacing bool
	replace   *ConnectForm
	removing  bool
	remove    string
	list      providerModels
}

// providerModels is a provider's models dialog.
type providerModels struct {
	shown    bool
	name     string
	pm       *app.ProviderModels
	seq      int
	contexts map[string]*int // an Other model's context window, as typed
	addID    string
	addCtx   string
}

func (d *providerModels) open(s *Screen, name string) {
	d.seq++
	*d = providerModels{shown: true, name: name, seq: d.seq, contexts: map[string]*int{}}
	seq := d.seq
	s.st.Pages.ProviderModels(name, func(pm app.ProviderModels, err error) {
		if seq != d.seq {
			return
		}
		if err != nil {
			s.st.Error(err)
			d.shown = false
			return
		}
		d.pm = &pm
	})
}

// modelsPage is Settings › Models (SPEC 3.9): the connected providers with
// their keys, models and limits, and the default and fast models.
func (s *Screen) modelsPage(c *kit.Context, set config.Settings) {
	p := &s.models
	kit.SettingsSection(c, "Models", logic.AppName+" works the same with every model. Keys are stored in the OS keychain.", func() {
		providers := logic.Providers(set)
		if len(providers) == 0 {
			kit.Text(c, "No provider yet. Add one with your own key.").FontSize(14).TextColor(kit.TokensOf(c).MutedForeground)
		}
		var keyed []string
		if v := s.st.Settings.View; v != nil {
			keyed = v.LocalKeys
		}
		for _, pr := range providers {
			pr.Key = slices.Contains(keyed, pr.Name)
			s.providerRow(c, set, pr)
		}
		kit.Row(c).Gap(12).AlignItems(kit.Center).Children(func() {
			if kit.SmallButton(c, kit.IconPlus, "Add provider", kit.Outline).Clicked() {
				p.openAdd()
			}
			kit.Hint(c, "Any OpenAI-compatible URL")
		})
	})
	s.aliases(c, set)
}

func (s *Screen) providerRow(c *kit.Context, set config.Settings, pr logic.Provider) {
	p := &s.models
	k := kit.TokensOf(c)
	kit.Row(c).Gap(12).Padding(12).Radius(kit.Radius).Border(1, k.Border).AlignItems(kit.Start).Label(pr.Name).Children(func() {
		icon := kit.IconKey
		if pr.Local() {
			icon = kit.IconServer
		}
		kit.Box(c).Size(32, 32).Radius(kit.Radius * 0.8).Background(k.Muted).AlignItems(kit.Center).Justify(kit.Center).Shrink(0).Children(func() {
			kit.Icon(c, icon).Size(16, 16)
		})
		kit.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(4).Children(func() {
			kit.Row(c).Gap(8).AlignItems(kit.Center).Children(func() {
				kit.Text(c, pr.Name).FontSize(14).FontWeight(500).SingleLine().Shrink(1)
				kit.Badge(c, logic.KindName(pr.Kind), kit.Neutral)
			})
			kit.Muted(c, pr.Line())
			if pr.BaseURL != "" {
				kit.Tooltip(kit.Mono(c, pr.BaseURL).SingleLine().Shrink(1).TextColor(k.MutedForeground), pr.BaseURL)
			}
			kit.Row(c).Gap(8).AlignItems(kit.Center).Children(func() {
				kit.Text(c, "Background calls at once").FontSize(12).TextColor(k.MutedForeground)
				own := ownLimit(set, pr.Name)
				n := kit.NumberInput(c, own, "Background calls at once for "+pr.Name, kit.NumberSpec{Min: 1, Optional: true})
				n.Placeholder(strconv.Itoa(logic.LimitDefault(pr.Kind, pr.BaseURL, set.LLM.MaxParallelCalls))).Height(28).Width(64)
				kit.Tooltip(n.Element, "Empty uses the default")
				if n.Committed {
					s.wait(s.st.Settings.Edit(logic.ProviderLimit(pr.Name, n.Value)), func(err error) {
						if err != nil {
							n.Failed()
							s.st.Error(err)
							return
						}
						n.Saved()
					})
				}
			})
		})
		kit.Row(c).Gap(4).Shrink(0).Children(func() {
			if kit.SmallButton(c, nil, "Models", kit.Ghost).Label("Models of " + pr.Name).Clicked() {
				p.list.open(s, pr.Name)
			}
			// A provider on this computer may have a token too, which can be
			// changed or removed (TASK-074).
			if kit.SmallButton(c, nil, "Replace key", kit.Ghost).Label("Replace the key of " + pr.Name).Clicked() {
				p.openReplace(&logic.Replace{Name: pr.Name, Kind: pr.Kind, BaseURL: pr.BaseURL, Keyed: pr.Key || !pr.Local()})
			}
			if kit.SmallButton(c, nil, "Remove", kit.Ghost).Label("Remove " + pr.Name).TextColor(k.MutedForeground).Clicked() {
				p.removing, p.remove = true, pr.Name
			}
		})
	})
}

// ownLimit is a provider's own background limit, nil for the default.
func ownLimit(set config.Settings, name string) *int {
	if v, ok := set.LLM.ProviderMaxParallelCalls[name]; ok {
		return &v
	}
	return nil
}

// openAdd opens *Add provider* with a new form. A connect closes the
// dialog only while it still shows that form, not one opened since.
func (p *modelsPage) openAdd() {
	p.closeForms()
	form := &ConnectForm{}
	form.Connected = func(app.ConnectResult) {
		if p.add == form {
			p.adding = false
		}
	}
	p.adding, p.add = true, form
}

// openReplace opens a provider's *New key* dialog, as openAdd does.
func (p *modelsPage) openReplace(r *logic.Replace) {
	p.closeForms()
	form := &ConnectForm{Replace: r}
	form.Connected = func(app.ConnectResult) {
		if p.replace == form {
			p.replacing = false
		}
	}
	p.replacing, p.replace = true, form
}

// closeForms drops the forms of dialogs that closed, with the key and
// placeholder values typed in them (TASK-068).
func (p *modelsPage) closeForms() {
	if !p.adding && p.add != nil {
		p.add.Clear()
		p.add = nil
	}
	if !p.replacing && p.replace != nil {
		p.replace.Clear()
		p.replace = nil
	}
}

// dialogs are the Models page's dialogs, over the screen.
func (p *modelsPage) dialogs(c *kit.Context, s *Screen) {
	defer p.closeForms()
	if p.adding && p.add != nil {
		kit.Dialog(c, &p.adding, "Add provider", func() {
			kit.Muted(c, "The key is checked with the provider and stored in the OS keychain.")
			p.add.View(c, s.st)
		})
	}
	if p.replacing && p.replace != nil {
		kit.Dialog(c, &p.replacing, "New key for "+p.replace.Replace.Name, func() {
			text := "The saved key is never shown. A new one replaces it; the models stay as they are."
			if r := p.replace.Replace; r.Local() && r.Keyed {
				text = "The saved key is never shown. A new one replaces it, and an empty field removes it; the models stay as they are."
			} else if r.Local() {
				text = "This server is on this computer and has no key. Add one only if it asks for a token; the models stay as they are."
			}
			kit.Muted(c, text)
			p.replace.View(c, s.st)
		})
	}
	if p.list.shown {
		p.list.view(c, s)
	}
	if p.removing {
		name := p.remove
		switch kit.AlertDialog(c, &p.removing, "Remove "+name+"?",
			"Its key is deleted from the OS keychain and its models are turned off. Chats that use them need another model. Default and fast move to another provider's models.",
			"Cancel", "Remove") {
		case 1:
			s.report(s.st.Settings.RemoveProvider(name))
		}
	}
}

func (d *providerModels) view(c *kit.Context, s *Screen) {
	set := s.st.Settings.View
	kit.Dialog(c, &d.shown, "Models of "+d.name, func() {
		kit.Muted(c, "Models that are on appear in the model picker at once.")
		if d.pm == nil || set == nil {
			kit.Spinner(c, "Loading the models")
			return
		}
		pm := *d.pm
		on := set.Settings.LLM.Providers[d.name].Models
		aliases := set.Settings.LLM.Models
		kit.Scroll(c).MaxHeight(440).AlignItems(kit.Stretch).Children(func() {
			kit.Column(c).FillWidth().Gap(16).AlignItems(kit.Stretch).Padding(0, 12, 0, 0).Children(func() {
				if pm.Problem != "" {
					kit.Alert(c, kit.Amber, "The provider's model list couldn't be read: "+pm.Problem, "")
				}
				d.list(c, s, logic.ProviderModelRows(pm, on), on, aliases, false)
				if len(pm.Other) > 0 {
					kit.Column(c).Gap(8).AlignItems(kit.Stretch).Children(func() {
						kit.Subheading(c, "Other models")
						kit.Hint(c, "Listed by the provider but not in "+logic.AppName+"'s model list. Turning one on needs its context window.")
						d.list(c, s, pm.Other, on, aliases, true)
					})
				}
				if logic.CanAddModels(pm) {
					d.addModel(c, s)
				}
			})
		})
	})
}

func (d *providerModels) list(c *kit.Context, s *Screen, models []app.ModelOption, on []config.ModelSettings, aliases map[string]string, askContext bool) {
	if len(models) == 0 {
		kit.Muted(c, "No models listed.")
		return
	}
	k := kit.TokensOf(c)
	kit.Column(c).Radius(kit.Radius).Border(1, k.Border).AlignItems(kit.Stretch).Children(func() {
		for i, m := range models {
			if i > 0 {
				kit.Box(c).Height(1).Background(k.Border).Shrink(0)
			}
			d.row(c, s, m, slices.ContainsFunc(on, func(x config.ModelSettings) bool { return x.ID == m.ID }),
				logic.Aliased(aliases, d.name, m.ID), askContext)
		}
	})
}

func (d *providerModels) row(c *kit.Context, s *Screen, m app.ModelOption, on bool, aliases []string, askContext bool) {
	ctx := d.contexts[m.ID]
	if ctx == nil && m.Context > 0 {
		v := m.Context
		ctx = &v
	}
	needs := askContext && !on && ctx == nil
	locked := on && len(aliases) > 0
	kit.Row(c).Gap(12).PaddingX(12).PaddingY(8).AlignItems(kit.Center).Children(func() {
		kit.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
			kit.Row(c).Gap(6).AlignItems(kit.Center).Children(func() {
				kit.Text(c, m.Name).FontSize(14).SingleLine().Shrink(1)
				for _, a := range aliases {
					kit.Badge(c, a, kit.Neutral)
				}
			})
			kit.Mono(c, logic.ModelLine(m, askContext)).SingleLine().Shrink(1).TextColor(kit.TokensOf(c).MutedForeground)
		})
		if askContext {
			n := kit.NumberInput(c, ctx, "Context window of "+m.Name, kit.NumberSpec{Min: 1000, Optional: true})
			n.Placeholder("Context").Height(28)
			kit.Disable(n.Element, on)
			if n.Committed {
				d.contexts[m.ID] = n.Value
				if n.Value == nil {
					d.contexts[m.ID] = nil
				}
			}
		}
		value := on
		sw := kit.Switch(c, &value, m.Name)
		kit.Disable(sw, locked || needs)
		if hint := logic.SwitchHint(aliases, on, needs); hint != "" {
			kit.Tooltip(sw, hint)
		}
		if sw.Changed() && !locked && !needs {
			context := 0
			if ctx != nil {
				context = *ctx
			}
			s.report(s.st.Settings.Edit(logic.Toggle(d.name, m, value, context)))
		}
	})
}

// addModel types in a model the provider doesn't list, with its context
// window (SPEC 3.9).
func (d *providerModels) addModel(c *kit.Context, s *Screen) {
	kit.Column(c).Gap(8).AlignItems(kit.Stretch).Children(func() {
		kit.Subheading(c, "Add a model")
		id := strings.TrimSpace(d.addID)
		n, err := strconv.Atoi(strings.TrimSpace(d.addCtx))
		ok := id != "" && err == nil && n >= 1000
		add := func() {
			if !ok {
				return
			}
			m := app.ModelOption{ID: id, Name: id, Context: n}
			s.report(s.st.Settings.Edit(logic.Toggle(d.name, m, true, n)))
			d.addID, d.addCtx = "", ""
		}
		kit.Row(c).Gap(8).AlignItems(kit.End).Children(func() {
			kit.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(6).AlignItems(kit.Stretch).Children(func() {
				kit.Text(c, "Model ID").FontSize(14).FontWeight(500)
				in := kit.TextInput(c, &d.addID, "Model ID").Font(kit.FontMono).FontSize(12)
				if in.Submitted() {
					add()
				}
			})
			kit.Column(c).Width(128).Gap(6).AlignItems(kit.Stretch).Children(func() {
				kit.Text(c, "Context").FontSize(14).FontWeight(500)
				in := kit.TextInput(c, &d.addCtx, "Context").Placeholder("32000").FontFeatures("tnum")
				if in.Submitted() {
					add()
				}
			})
			if kit.Disable(kit.Button(c, "Add", kit.Outline), !ok).Clicked() {
				add()
			}
		})
		kit.Hint(c, "For a model the provider doesn't list. The context window is in tokens.")
	})
}

// aliases picks the models behind default and fast (SPEC 3.9).
func (s *Screen) aliases(c *kit.Context, set config.Settings) {
	groups := s.st.Settings.Models
	kit.SettingsSection(c, "Default and fast model", "Built-in model list, updated with the app.", func() {
		kit.Row(c).Gap(16).AlignItems(kit.Start).Children(func() {
			s.aliasPick(c, set, groups, "default", "Default model", "New chats start with it.")
			s.aliasPick(c, set, groups, "fast", "Fast model", "For titles and summaries.")
		})
	})
}

func (s *Screen) aliasPick(c *kit.Context, set config.Settings, groups []app.ModelGroup, alias, label, hint string) {
	value := set.LLM.Models[alias]
	kit.Column(c).Grow(1).Shrink(1).Basis(0).MinWidth(0).Gap(6).AlignItems(kit.Stretch).Children(func() {
		kit.Text(c, label).FontSize(14).FontWeight(500)
		b := kit.MenuButton(c, logic.AliasName(groups, value), func(m *kit.Menu) {
			group := ""
			for _, ch := range logic.AliasChoices(groups, value) {
				if ch.Group != group {
					group = ch.Group
					m.Item(group).Disabled(true)
				}
				if m.Item(ch.Name).ID(alias+":"+ch.Ref).Radio(ch.Ref == value).Chosen() && ch.Ref != value {
					ref := ch.Ref
					s.edit(func(c *config.Settings) {
						if c.LLM.Models == nil {
							c.LLM.Models = map[string]string{}
						}
						c.LLM.Models[alias] = ref
					})
				}
			}
		}).Label(label).FillWidth()
		kit.Disable(b, len(groups) == 0)
		kit.Hint(c, hint)
	})
}

// limits holds the limits of SPEC 7.6; a change applies at once.
func (s *Screen) limits(c *kit.Context, set config.Settings) {
	kit.SettingsSection(c, "Limits", "Starting values, tuned with the benchmark. Changes apply at once.", func() {
		kit.Column(c).Gap(8).MaxWidth(448).AlignItems(kit.Stretch).Children(func() {
			for _, l := range logic.Limits {
				v := l.Get(&set)
				kit.Row(c).Gap(16).AlignItems(kit.Center).Children(func() {
					kit.Column(c).Grow(1).Shrink(1).MinWidth(0).Children(func() {
						kit.Text(c, l.Label).FontSize(14)
						if l.Hint != "" {
							kit.Hint(c, l.Hint)
						}
					})
					n := kit.NumberInput(c, &v, l.Label, kit.NumberSpec{Min: l.Min})
					if n.Committed && n.Value != nil {
						nv := *n.Value
						s.wait(s.st.Settings.Edit(func(c *config.Settings) { l.Set(c, nv) }), func(err error) {
							if err != nil {
								n.Failed()
								s.st.Error(err)
								return
							}
							n.Saved()
						})
					}
				})
			}
		})
		kit.Row(c).Gap(6).AlignItems(kit.Center).Wrap().Children(func() {
			kit.Badge(c, "config.yaml", kit.Neutral)
			kit.Hint(c, "Saved in the user config file, with your comments kept. A limit for one provider is on its row above.")
		})
	})
}
