package logic

import (
	"fmt"
	"slices"
	"strings"

	"github.com/amir-saatchi/jenab/internal/app"
)

// The Connect form's choices (SPEC 3.9), from chat/provider-choice.ts as
// TASK-052 left it: the presets in their groups, then Other.

// Other is the choice for an OpenAI-compatible provider set up by hand.
const Other = "other"

// Choice is a provider in the form's list: its name and a muted second
// line.
type Choice struct{ ID, Name, Hint string }

// ChoiceGroup is a group of the list, such as "Cloud".
type ChoiceGroup struct {
	Key, Label string
	Items      []Choice
}

var groupLabels = map[string]string{"cloud": "Cloud", "local": "On this computer"}

// PresetHint is a preset's second line: the host for a cloud provider, "no
// key needed" for one on this computer.
func PresetHint(p app.PresetItem) string {
	if p.NoKey {
		if p.Running {
			return "running · no key needed"
		}
		return "no key needed"
	}
	return p.Host
}

// ProviderGroups puts the presets in their groups, in the order the list
// names them first (a running Ollama comes first, SPEC 3.9), then Other.
func ProviderGroups(presets []app.PresetItem) []ChoiceGroup {
	var groups []ChoiceGroup
	for _, p := range presets {
		i := slices.IndexFunc(groups, func(g ChoiceGroup) bool { return g.Key == p.Group })
		if i < 0 {
			label := groupLabels[p.Group]
			if label == "" {
				label = p.Group
			}
			groups = append(groups, ChoiceGroup{Key: p.Group, Label: label})
			i = len(groups) - 1
		}
		groups[i].Items = append(groups[i].Items, Choice{ID: p.ID, Name: p.Name, Hint: PresetHint(p)})
	}
	return append(groups, ChoiceGroup{Key: Other, Label: "Other",
		Items: []Choice{{ID: Other, Name: "OpenAI-compatible…", Hint: "any base URL, set by hand"}}})
}

// FirstPick is the choice the form opens with: a provider that runs on
// this computer, else none ("").
func FirstPick(presets []app.PresetItem) string {
	for _, p := range presets {
		if p.Running {
			return p.ID
		}
	}
	return ""
}

// FormShape is what the form asks for once a provider is picked.
type FormShape struct {
	Preset *app.PresetItem
	Chosen bool     // a provider is picked
	Other  bool     // the name and base URL are typed in
	Key    bool     // the key field is shown and needed
	Fields []string // a field for each base-URL placeholder
}

// Replace names a connected provider whose key is replaced; its models
// stay.
type Replace struct{ Name, Kind, BaseURL string }

// FormFor is what the form asks for once pick is chosen. A replaced key
// keeps its provider's placeholders.
func FormFor(pick string, presets []app.PresetItem, replace *Replace) FormShape {
	switch {
	case replace != nil:
		return FormShape{Chosen: true, Key: true, Fields: Placeholders(replace.BaseURL)}
	case pick == Other:
		return FormShape{Chosen: true, Other: true, Key: true, Fields: []string{}}
	}
	i := slices.IndexFunc(presets, func(p app.PresetItem) bool { return p.ID == pick })
	if i < 0 {
		return FormShape{Fields: []string{}}
	}
	p := presets[i]
	fields := p.Fields
	if fields == nil {
		fields = []string{}
	}
	return FormShape{Preset: &p, Chosen: true, Key: !p.NoKey, Fields: fields}
}

// FieldLabel names a base-URL placeholder: account_id reads "Account ID".
func FieldLabel(field string) string {
	var words []string
	for _, w := range strings.Split(field, "_") {
		if w == "" {
			continue
		}
		if strings.EqualFold(w, "id") {
			w = "ID"
		}
		words = append(words, w)
	}
	s := strings.Join(words, " ")
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// FreeName is name, or name-2, name-3… when a provider has it.
func FreeName[V any](name string, taken map[string]V) string {
	if _, ok := taken[name]; !ok {
		return name
	}
	for n := 2; ; n++ {
		s := fmt.Sprintf("%s-%d", name, n)
		if _, ok := taken[s]; !ok {
			return s
		}
	}
}

// ConnectedText is the toast after a provider is connected, or its key
// replaced.
func ConnectedText(name string, r app.ConnectResult, replaced bool) string {
	switch {
	case replaced:
		return name + ": the new key works and is saved"
	case r.NoModelList:
		return name + " connected. It lists no models; add them in Settings → Models."
	case r.Models == 1:
		return name + " connected · 1 model on"
	}
	return fmt.Sprintf("%s connected · %d models on", name, r.Models)
}

// KindName is a provider kind as the Models page names it.
func KindName(kind string) string {
	if n, ok := kindNames[kind]; ok {
		return n
	}
	return kind
}

var kindNames = map[string]string{
	"anthropic":         "Anthropic",
	"openai":            "OpenAI",
	"gemini":            "Gemini",
	"openai_compatible": "OpenAI-compatible",
	"ollama":            "Ollama",
}
