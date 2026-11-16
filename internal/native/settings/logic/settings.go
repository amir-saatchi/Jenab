package logic

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/provider"
)

// Pages are Settings' page list (settings-view.tsx). Desktop control is
// Windows-only in v1 (SPEC 8.10), and so is its page.
func Pages(windows bool) []string {
	ps := []string{"General", "Memory", "Appearance", "Models", "Usage"}
	if windows {
		ps = append(ps, "Desktop")
	}
	return append(ps, "Developer")
}

// Level is an approval level as the toggle names it.
type Level struct{ Value, Label string }

// Levels are the approval levels (SPEC 8.8), as project-settings.tsx.
var Levels = []Level{{"strict", "Strict"}, {"standard", "Standard"}, {"auto", "Auto"}}

// Themes are the theme choices (SPEC 5.11).
var Themes = []Level{{"light", "Light"}, {"dark", "Dark"}, {"system", "System"}}

// UsageRanges are the days the Usage page sums.
var UsageRanges = []int{7, 30, 90}

// Limit is a limit of SPEC 7.6 on the Models page.
type Limit struct {
	Label, Hint string
	Min         int
	Get         func(*config.Settings) int
	Set         func(*config.Settings, int)
}

// Limits are the limits, in the order the page shows them.
var Limits = []Limit{
	{"Parallel background LLM calls", "Chats count but never wait", 1,
		func(s *config.Settings) int { return s.LLM.MaxParallelCalls },
		func(s *config.Settings, v int) { s.LLM.MaxParallelCalls = v }},
	{"Parallel pipeline runs", "", 1,
		func(s *config.Settings) int { return s.Scheduler.MaxParallelRuns },
		func(s *config.Settings, v int) { s.Scheduler.MaxParallelRuns = v }},
	{"Background tasks per chat", "", 0,
		func(s *config.Settings) int { return s.LLM.MaxBackgroundTasksPerChat },
		func(s *config.Settings, v int) { s.LLM.MaxBackgroundTasksPerChat = v }},
	{"Subagents at once per chat", "", 1,
		func(s *config.Settings) int { return s.LLM.MaxSubagentsPerChat },
		func(s *config.Settings, v int) { s.LLM.MaxSubagentsPerChat = v }},
	{"Requests per turn", "", 2,
		func(s *config.Settings) int { return s.LLM.TurnMaxRequests },
		func(s *config.Settings, v int) { s.LLM.TurnMaxRequests = v }},
	{"Requests per app-started turn", "", 2,
		func(s *config.Settings) int { return s.LLM.SystemTurnMaxRequests },
		func(s *config.Settings, v int) { s.LLM.SystemTurnMaxRequests = v }},
	{"Tokens per app-started turn", "", 1000,
		func(s *config.Settings) int { return s.LLM.SystemTurnMaxTokens },
		func(s *config.Settings, v int) { s.LLM.SystemTurnMaxTokens = v }},
}

// ProviderLimit sets a provider's own background limit; nil takes it out,
// so the provider gets the default again.
func ProviderLimit(name string, v *int) func(*config.Settings) {
	return func(s *config.Settings) {
		if v == nil {
			delete(s.LLM.ProviderMaxParallelCalls, name)
			return
		}
		if s.LLM.ProviderMaxParallelCalls == nil {
			s.LLM.ProviderMaxParallelCalls = map[string]int{}
		}
		s.LLM.ProviderMaxParallelCalls[name] = *v
	}
}

// Provider is a connected provider, as its row shows it.
type Provider struct {
	Name string
	config.ProviderSettings
}

// Providers are the connected providers by name.
func Providers(s config.Settings) []Provider {
	var out []Provider
	for name, p := range s.LLM.Providers {
		out = append(out, Provider{name, p})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Local tells whether the provider runs on this computer, so it needs no
// key: its base URL is this computer, by the registry's own rule. An
// Ollama, an LM Studio or any server on localhost (TASK-068).
func (p Provider) Local() bool {
	return provider.LocalURL(provider.Kind(p.Kind), p.BaseURL)
}

// Line is the row's second line: where the key is, and how many models
// are on.
func (p Provider) Line() string {
	where := "Key in the keychain"
	if p.Local() {
		where = "On this computer · no key needed"
	}
	n := len(p.Models)
	if n == 1 {
		return where + " · 1 model on"
	}
	return fmt.Sprintf("%s · %d models on", where, n)
}

// Aliased are the aliases (default, fast) that point at a provider's
// model, which can't be turned off until they move.
func Aliased(aliases map[string]string, provider, model string) []string {
	var out []string
	for _, a := range slices.Sorted(mapKeys(aliases)) {
		if aliases[a] == provider+"/"+model {
			out = append(out, a)
		}
	}
	return out
}

func mapKeys(m map[string]string) func(func(string) bool) {
	return func(yield func(string) bool) {
		for k := range m {
			if !yield(k) {
				return
			}
		}
	}
}

// Toggle turns a provider's model on or off. A model the catalog doesn't
// know keeps its context window.
func Toggle(provider string, m app.ModelOption, on bool, context int) func(*config.Settings) {
	return func(s *config.Settings) {
		p, ok := s.LLM.Providers[provider]
		if !ok {
			return
		}
		ms := slices.DeleteFunc(slices.Clone(p.Models), func(x config.ModelSettings) bool { return x.ID == m.ID })
		if on {
			if m.Known {
				ms = append(ms, config.ModelSettings{ID: m.ID})
			} else {
				ms = append(ms, config.ModelSettings{ID: m.ID, Context: context})
			}
		}
		p.Models = ms
		s.LLM.Providers[provider] = p
	}
}

// ProviderModelRows are the models a provider's dialog lists first: the
// catalog models it lists, then those typed in by hand that are on and
// not listed.
func ProviderModelRows(pm app.ProviderModels, on []config.ModelSettings) []app.ModelOption {
	listed := map[string]bool{}
	for _, m := range append(slices.Clone(pm.Models), pm.Other...) {
		listed[m.ID] = true
	}
	rows := slices.Clone(pm.Models)
	for _, m := range on {
		if !listed[m.ID] {
			rows = append(rows, app.ModelOption{ID: m.ID, Name: m.ID, On: true, Context: m.Context})
		}
	}
	return rows
}

// CanAddModels tells whether models may be typed in for a provider: it
// lists none, or its kind lists models the catalog doesn't know.
func CanAddModels(pm app.ProviderModels) bool {
	return pm.NoModelList || pm.Kind == "openai_compatible" || pm.Kind == "ollama"
}

// ModelLine is a model row's second line: its ID, and its context window
// when known and not asked for.
func ModelLine(m app.ModelOption, askContext bool) string {
	if !askContext && m.Context > 0 {
		return m.ID + " · " + ContextSize(m.Context) + " context"
	}
	return m.ID
}

// SwitchHint says why a model's switch is off: it is an alias's model, or
// an Other model needs its context window first.
func SwitchHint(aliases []string, on, needsContext bool) string {
	switch {
	case on && len(aliases) > 0:
		return "Used as " + strings.Join(aliases, " and ") + "; pick another model for it first"
	case needsContext:
		return "Enter the context window first"
	}
	return ""
}

// AliasChoice is a model the default and fast pickers offer, under its
// provider; NotOn is a model an alias names that is not on any more.
type AliasChoice struct {
	Group, Ref, Name string
	NotOn            bool
}

// AliasChoices are the models default and fast can name: those that are
// on, by provider, then value under "Not on" when it is none of them.
func AliasChoices(groups []app.ModelGroup, value string) []AliasChoice {
	var out []AliasChoice
	found := false
	for _, g := range groups {
		for _, m := range g.Models {
			out = append(out, AliasChoice{Group: g.Provider, Ref: m.Ref, Name: m.Name})
			found = found || m.Ref == value
		}
	}
	if value != "" && !found {
		out = append(out, AliasChoice{Group: "Not on", Ref: value, Name: value, NotOn: true})
	}
	return out
}

// AliasName is what an alias picker shows for value.
func AliasName(groups []app.ModelGroup, value string) string {
	if value == "" {
		return "No model"
	}
	for _, c := range AliasChoices(groups, value) {
		if c.Ref == value {
			return c.Name
		}
	}
	return value
}

// ProblemLine is a setting config.yaml had wrong, as the alert lists it.
func ProblemLine(p config.Problem) string {
	s := ""
	if p.Line > 0 {
		s = fmt.Sprintf("Line %d: ", p.Line)
	}
	if p.Path != "" {
		s += p.Path + ": "
	}
	return s + p.Msg
}

// MovedText is what moving the projects to a new data folder did at this
// start; "" when it moved nothing and nothing failed.
func MovedText(moved int, failed []string) string {
	s := ""
	switch {
	case moved == 1:
		s = "1 project was moved to the new data folder at this start. "
	case moved > 1:
		s = fmt.Sprintf("%d projects were moved to the new data folder at this start. ", moved)
	}
	if len(failed) > 0 {
		s += "Not moved, still opened from the old folder: " + strings.Join(failed, ", ") + ". " + AppName +
			" tries again at the next start; the log has the reason."
	}
	return strings.TrimSpace(s)
}
