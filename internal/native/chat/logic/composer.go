package logic

import (
	"slices"

	"github.com/amir-saatchi/jenab/internal/app"
)

// ModelName is the name the picker shows for a chat's model: an alias
// shows the model it points at.
func ModelName(groups []app.ModelGroup, model string) string {
	if len(groups) == 0 {
		return "No model"
	}
	for _, g := range groups {
		for _, m := range g.Models {
			if m.Ref == model || slices.Contains(m.Aliases, model) {
				return m.Name
			}
		}
	}
	return model
}

// Starters are first messages for an empty Mother chat (SPEC 3.9); a
// click sends one.
var Starters = []string{
	"Track a price every day",
	"Summarise a news feed",
	"Collect a table from a web page",
}
