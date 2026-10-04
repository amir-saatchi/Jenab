package provider

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"
)

// Model is a catalog entry (SPEC 3.9): facts about one model's limits and
// prices, never prompts or rules for it. Zero means not known.
type Model struct {
	Kind     Kind   `json:"kind"`
	ID       string `json:"id"`
	Name     string `json:"name"`
	Context  int    `json:"context"`
	Output   int    `json:"output"`
	Tools    bool   `json:"tools"`
	Images   bool   `json:"images"`
	Thinking bool   `json:"thinking"`
	// ThinkingBudget: thinking is turned on with a token budget rather
	// than left to the model (Anthropic's extended thinking, e.g. Claude
	// Haiku 4.5). A protocol fact, read by the backend.
	ThinkingBudget bool `json:"thinking_budget,omitempty"`
	// Price is in US dollars per million tokens; nil if not known.
	Price *Price `json:"price,omitempty"`
	// Default and Fast mark the provider's suggestions for the aliases.
	Default bool `json:"default,omitempty"`
	Fast    bool `json:"fast,omitempty"`
	// Retired models stay usable while the provider serves them; the model
	// picker suggests Replacement.
	Retired     bool   `json:"retired,omitempty"`
	Replacement string `json:"replacement,omitempty"`
}

// Price is the price per million tokens.
type Price struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cache_read,omitempty"`
	CacheWrite float64 `json:"cache_write,omitempty"`
}

// Catalog is the built-in model list.
type Catalog struct {
	models []Model
}

//go:embed models.json
var modelsJSON []byte

// MustCatalog returns the built-in catalog. It panics if models.json is
// broken, which a test catches.
func MustCatalog() *Catalog {
	c, err := ParseCatalog(modelsJSON)
	if err != nil {
		panic(err)
	}
	return c
}

// ParseCatalog reads a catalog in the models.json format.
func ParseCatalog(data []byte) (*Catalog, error) {
	var f struct {
		Models []Model `json:"models"`
	}
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("provider: models.json: %w", err)
	}
	seen := map[[2]string]bool{}
	defaults := map[Kind]int{}
	fasts := map[Kind]int{}
	for _, m := range f.Models {
		if !slices.Contains(Kinds, m.Kind) || m.ID == "" || m.Name == "" {
			return nil, fmt.Errorf("provider: models.json: bad entry %q/%q", m.Kind, m.ID)
		}
		k := [2]string{string(m.Kind), m.ID}
		if seen[k] {
			return nil, fmt.Errorf("provider: models.json: %s/%s twice", m.Kind, m.ID)
		}
		seen[k] = true
		if m.Default {
			defaults[m.Kind]++
		}
		if m.Fast {
			fasts[m.Kind]++
		}
		if defaults[m.Kind] > 1 || fasts[m.Kind] > 1 {
			return nil, fmt.Errorf("provider: models.json: more than one default or fast model for %s", m.Kind)
		}
	}
	return &Catalog{models: f.Models}, nil
}

// Find returns the entry for a model of kind k.
func (c *Catalog) Find(k Kind, id string) (Model, bool) {
	for _, m := range c.models {
		if m.Kind == k && m.ID == id {
			return m, true
		}
	}
	return Model{}, false
}

// Models lists kind k's entries in catalog order.
func (c *Catalog) Models(k Kind) []Model {
	var out []Model
	for _, m := range c.models {
		if m.Kind == k {
			out = append(out, m)
		}
	}
	return out
}

// Suggested returns kind k's suggested default and fast model IDs, "" if
// none.
func (c *Catalog) Suggested(k Kind) (def, fast string) {
	for _, m := range c.models {
		if m.Kind != k {
			continue
		}
		if m.Default {
			def = m.ID
		}
		if m.Fast {
			fast = m.ID
		}
	}
	return def, fast
}
