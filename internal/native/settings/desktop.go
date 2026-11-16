package settings

import (
	"fmt"
	"strings"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/native/settings/logic"
	"github.com/amir-saatchi/jenab/internal/store"
)

// desktopPage is what Settings › Desktop has open: a macro's or a
// recipe's edit, and a new recipe.
type desktopPage struct {
	macro  *macroEdit   // the macro being edited
	recipe *recipeEdit  // the recipe being edited
	draft  *recipeDraft // a new recipe
	busy   bool
}

type macroEdit struct {
	id                         int64
	description, params, steps string
}

type recipeEdit struct {
	id   int64
	text string
}

type recipeDraft struct{ app, site, name, text string }

// run runs a change; then, on success, closes what was open.
func (d *desktopPage) run(s *Screen, change func(then func(error)), done func()) {
	if d.busy {
		return
	}
	d.busy = true
	change(func(err error) {
		d.busy = false
		if err != nil {
			s.st.Error(err)
			return
		}
		done()
	})
}

// desktopPage is Settings › Desktop (SPEC 8.11): per app, the macros
// agents saved, with their steps, params, last runs and failures, and the
// recipes; each can be edited or deleted. What was removed, by the user
// or the rules, stays listed for 30 days.
func (s *Screen) desktopPage(c *kit.Context) {
	list := s.st.Pages.Desktop
	kit.SettingsSection(c, "Desktop", "What agents saved while they used other apps' windows: macros they replay without you, and recipes, short notes on how to do a task in an app. A macro goes after 3 failed replays in a row or 90 days unused, an agent's recipe after a year unused; yours and those that come with the app stay.", func() {
		if list == nil {
			kit.Spinner(c, "Reading the macros and recipes")
			return
		}
		if len(list.Apps) == 0 {
			kit.Muted(c, "Nothing saved yet.")
		}
		for _, a := range list.Apps {
			box(c, a.App, func() {
				kit.Subheading(c, a.App)
				for _, m := range a.Macros {
					s.macroRow(c, m)
				}
				for _, r := range a.Recipes {
					s.recipeRow(c, r)
				}
			})
		}
		apps := make([]string, len(list.Apps))
		for i, a := range list.Apps {
			apps[i] = a.App
		}
		s.newRecipe(c, apps)
		removed := logic.RemovedItems(list.RemovedMacros, list.RemovedRecipes)
		if len(removed) > 0 {
			kit.Column(c).Gap(6).AlignItems(kit.Stretch).Children(func() {
				kit.Subheading(c, "Removed in the last 30 days")
				for _, r := range removed {
					kit.Muted(c, fmt.Sprintf("%s %s (%s), %s: %s", r.Kind, r.Name, r.App, logic.When(r.At, dateTime(c)), r.Why))
				}
			})
		}
	})
}

// itemHead is a macro's or a recipe's first line: its badge and name, and
// a muted line after them; Edit and Delete at its end unless editing.
func itemHead(c *kit.Context, badge, name string, muted []string, editing, busy bool) (edit, del bool) {
	kit.Row(c).Gap(8).AlignItems(kit.Center).Wrap().Children(func() {
		kit.Badge(c, badge, kit.Neutral)
		kit.Text(c, name).FontSize(14).FontWeight(500)
		for _, m := range muted {
			kit.Text(c, m).FontSize(12).TextColor(kit.TokensOf(c).MutedForeground)
		}
		if !editing {
			kit.Spacer(c)
			edit = kit.Disable(kit.SmallButton(c, nil, "Edit", kit.Ghost).Label("Edit "+name), busy).Clicked() && !busy
			del = kit.Disable(kit.SmallButton(c, nil, "Delete", kit.Ghost).Label("Delete "+name).TextColor(kit.TokensOf(c).Destructive), busy).Clicked() && !busy
		}
	})
	return
}

func (s *Screen) macroRow(c *kit.Context, item app.DesktopMacroItem) {
	d := &s.desk
	m := item.Macro
	editing := d.macro != nil && d.macro.id == m.ID
	kit.Column(c).Gap(6).AlignItems(kit.Stretch).Children(func() {
		var muted []string
		if m.Site != "" {
			muted = append(muted, "on "+m.Site)
		}
		muted = append(muted, fmt.Sprintf("version %d · %d runs, %d failed · last worked %s",
			m.Version, m.Runs, m.Failures, logic.When(m.LastWorkedAt, dateTime(c))))
		edit, del := itemHead(c, "Macro", m.Name+"("+strings.Join(m.Params, ", ")+")", muted, editing, d.busy)
		if edit {
			d.macro = &macroEdit{id: m.ID, description: m.Description, params: strings.Join(m.Params, ", "), steps: item.Steps}
		}
		if del {
			d.run(s, func(then func(error)) { s.st.Pages.DeleteMacro(m.ID, then) }, func() {})
		}
		if editing {
			e := d.macro
			field(c, "Description", func() {
				kit.TextInput(c, &e.description, "Description")
				if len([]rune(e.description)) > 300 {
					e.description = string([]rune(e.description)[:300])
				}
			})
			field(c, "Params", func() {
				kit.TextInput(c, &e.params, "Params").Direction(kit.LeftToRight)
				kit.Hint(c, "Comma-separated; each is written as {name} in the steps.")
			})
			field(c, "Steps", func() {
				kit.TextArea(c, &e.steps, "Steps").Lines(8, 24).Font(kit.FontMono).FontSize(12).Direction(kit.LeftToRight)
			})
			kit.Row(c).Gap(8).Children(func() {
				if kit.Disable(kit.SmallButton(c, nil, "Save", kit.Default), d.busy).Clicked() {
					var params []string
					for _, p := range strings.Split(e.params, ",") {
						if p = strings.TrimSpace(p); p != "" {
							params = append(params, p)
						}
					}
					edit := app.DesktopMacroEdit{ID: e.id, Description: e.description, Params: params, Steps: e.steps}
					d.run(s, func(then func(error)) { s.st.Pages.EditMacro(edit, then) }, func() { d.macro = nil })
				}
				if kit.Disable(kit.SmallButton(c, nil, "Cancel", kit.Ghost), d.busy).Clicked() && !d.busy {
					d.macro = nil
				}
			})
			return
		}
		if m.Description != "" {
			kit.Text(c, m.Description).FontSize(14)
		}
		kit.Pre(c, item.Listing)
		for i, r := range item.Runs {
			if i == 5 {
				break
			}
			kit.Text(c, logic.RunLine(r, dateTime(c))).FontSize(12).TextColor(kit.TokensOf(c).MutedForeground)
		}
	})
}

func (s *Screen) recipeRow(c *kit.Context, r store.DesktopRecipe) {
	d := &s.desk
	editing := d.recipe != nil && d.recipe.id == r.ID
	kit.Column(c).Gap(6).AlignItems(kit.Stretch).Children(func() {
		var muted []string
		if r.Site != "" {
			muted = append(muted, "on "+r.Site)
		}
		muted = append(muted, fmt.Sprintf("%s · used %d times", logic.SourceLabel(r.Source), r.Uses))
		edit, del := itemHead(c, "Recipe", r.Name, muted, editing, d.busy)
		if edit {
			d.recipe = &recipeEdit{id: r.ID, text: r.Text}
		}
		if del {
			d.run(s, func(then func(error)) { s.st.Pages.DeleteRecipe(r.ID, then) }, func() {})
		}
		if !editing {
			kit.Text(c, r.Text).FontSize(14)
			return
		}
		e := d.recipe
		recipeText(c, &e.text, "Recipe text", r.Source == "app")
		kit.Row(c).Gap(8).Children(func() {
			off := d.busy || strings.TrimSpace(e.text) == ""
			if kit.Disable(kit.SmallButton(c, nil, "Save", kit.Default), off).Clicked() && !off {
				w := app.DesktopRecipeWrite{ID: e.id, Text: e.text}
				d.run(s, func(then func(error)) { s.st.Pages.WriteRecipe(w, then) }, func() { d.recipe = nil })
			}
			if kit.Disable(kit.SmallButton(c, nil, "Cancel", kit.Ghost), d.busy).Clicked() && !d.busy {
				d.recipe = nil
			}
		})
	})
}

// recipeText is a recipe's text, kept to MaxRecipe characters, with its
// count under it.
func recipeText(c *kit.Context, text *string, label string, fromApp bool) {
	if r := []rune(*text); len(r) > logic.MaxRecipe {
		*text = string(r[:logic.MaxRecipe])
	}
	kit.TextArea(c, text, label).Lines(4, 16)
	hint := fmt.Sprintf("%d of %s characters.", len([]rune(*text)), logic.Thousands(logic.MaxRecipe))
	if fromApp {
		hint += " Your edit is kept as your own recipe; agents then see yours."
	}
	kit.Hint(c, hint)
}

func (s *Screen) newRecipe(c *kit.Context, apps []string) {
	d := &s.desk
	if d.draft == nil {
		if kit.SmallButton(c, kit.IconPlus, "Add recipe", kit.Outline).AlignSelf(kit.Start).Clicked() {
			first := ""
			if len(apps) > 0 {
				first = apps[0]
			}
			d.draft = &recipeDraft{app: first}
		}
		return
	}
	r := d.draft
	box(c, "New recipe", func() {
		field(c, "App", func() {
			kit.TextInput(c, &r.app, "App").Placeholder("such as calculatorapp, chrome or browser").Direction(kit.LeftToRight)
			kit.Hint(c, "The program's name without .exe; browser for every browser.")
		})
		field(c, "Site", func() {
			kit.TextInput(c, &r.site, "Site").Placeholder("only for a browser, such as https://example.com").Direction(kit.LeftToRight)
		})
		field(c, "Name", func() {
			kit.TextInput(c, &r.name, "Name").Placeholder("such as site_search").Direction(kit.LeftToRight)
			if len([]rune(r.name)) > 41 {
				r.name = string([]rune(r.name)[:41])
			}
		})
		field(c, "Text", func() { recipeText(c, &r.text, "Text", false) })
		kit.Row(c).Gap(8).Children(func() {
			off := d.busy || strings.TrimSpace(r.app) == "" || strings.TrimSpace(r.name) == "" || strings.TrimSpace(r.text) == ""
			if kit.Disable(kit.SmallButton(c, nil, "Save", kit.Default), off).Clicked() && !off {
				w := app.DesktopRecipeWrite{App: r.app, Site: r.site, Name: r.name, Text: r.text}
				d.run(s, func(then func(error)) { s.st.Pages.WriteRecipe(w, then) }, func() { d.draft = nil })
			}
			if kit.Disable(kit.SmallButton(c, nil, "Cancel", kit.Ghost), d.busy).Clicked() && !d.busy {
				d.draft = nil
			}
		})
	})
}
