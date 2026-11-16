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

// Level is an approval level the composer's picker offers (SPEC 8.8).
type Level struct{ ID, Name, Hint string }

// Levels are the approval levels, in the picker's order (composer.tsx).
var Levels = []Level{
	{"strict", "Strict", "Asks before every change"},
	{"standard", "Standard", "Asks for new hosts, code and destructive changes"},
	{"auto", "Auto", "Asks less, but always for destructive changes"},
}

// LevelName is the name the picker shows for a project's level: Standard
// when it isn't known yet, and the ID itself when it is no level the
// picker knows.
func LevelName(level string) string {
	if level == "" {
		level = "standard"
	}
	for _, l := range Levels {
		if l.ID == level {
			return l.Name
		}
	}
	return level
}

// Starters are first messages for an empty Mother chat (SPEC 3.9); a
// click sends one.
var Starters = []string{
	"Track a price every day",
	"Summarise a news feed",
	"Collect a table from a web page",
}
