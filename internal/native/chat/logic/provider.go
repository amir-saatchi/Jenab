package logic

import (
	"fmt"
	"regexp"

	"github.com/amir-saatchi/jenab/internal/app"
)

// The Connect form's choices (SPEC 3.9), from chat/provider-card.tsx and
// settings/format.ts.

// OtherProvider is the Connect form's choice for a provider no preset
// names: an OpenAI-compatible base URL.
const OtherProvider = "other"

// FirstPick is the Connect form's choice when it opens: the first preset,
// which is a local Ollama when it runs, or Other when there are none.
func FirstPick(presets []app.PresetItem) string {
	if len(presets) == 0 {
		return OtherProvider
	}
	return presets[0].ID
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

var placeholder = regexp.MustCompile(`\{([A-Za-z0-9_]+)\}`)

// Placeholders are the {field} names in a base URL, e.g. account_id.
func Placeholders(baseURL string) []string {
	out := []string{}
	for _, m := range placeholder.FindAllStringSubmatch(baseURL, -1) {
		out = append(out, m[1])
	}
	return out
}
