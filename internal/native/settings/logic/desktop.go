package logic

import (
	"fmt"
	"math"
	"time"

	"github.com/amir-saatchi/jenab/internal/store"
)

// MaxRecipe is the longest a desktop recipe may be, in characters.
const MaxRecipe = 1200

// When is a stored time as the user reads it; Go's zero time is never.
// format writes it in the user's formats.
func When(t time.Time, format func(time.Time) string) string {
	if t.IsZero() || t.Year() == 1 {
		return "never"
	}
	return format(t)
}

// SourceLabel says who wrote a recipe.
func SourceLabel(source string) string {
	switch source {
	case "app":
		return "comes with the app"
	case "user":
		return "yours"
	}
	return "saved by an agent"
}

// RunLine is a macro's replay: when, and how it went.
func RunLine(r store.DesktopRun, format func(time.Time) string) string {
	at := When(r.At, format)
	if r.OK {
		s := fmt.Sprintf("%s: worked in %d s", at, int(math.Round(r.Seconds)))
		if r.Skipped > 0 {
			s += fmt.Sprintf(", %d optional skipped", r.Skipped)
		}
		return s
	}
	return fmt.Sprintf("%s: failed at step %d: %s", at, r.FailedAt, r.Reason)
}

// Removed is a macro or recipe removed in the last 30 days.
type Removed struct {
	Kind, Name, App, Why string
	At                   time.Time
}

// RemovedItems are the removed macros, then the removed recipes.
func RemovedItems(ms []store.DesktopMacro, rs []store.DesktopRecipe) []Removed {
	var out []Removed
	for _, m := range ms {
		out = append(out, Removed{Kind: "Macro", Name: m.Name, App: m.App, Why: m.RemovedWhy, At: m.RemovedAt})
	}
	for _, r := range rs {
		out = append(out, Removed{Kind: "Recipe", Name: r.Name, App: r.App, Why: r.RemovedWhy, At: r.RemovedAt})
	}
	return out
}
