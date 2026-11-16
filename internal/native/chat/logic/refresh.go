package logic

import (
	"fmt"
	"strings"

	"github.com/amir-saatchi/jenab/internal/agent"
)

// Refresh memory (SPEC 3.5): the dialog before a long one, and the toast
// of a batch's undo.

func plural(n int, one string) string {
	if n == 1 {
		return thousands(n) + " " + one
	}
	return thousands(n) + " " + one + "s"
}

// EstimateTitle and EstimateText are the dialog before a long refresh.
func EstimateTitle(e agent.RefreshEstimate, full bool) string {
	what := "Refresh memory"
	if full {
		what = "Review the whole chat again"
	}
	return fmt.Sprintf("%s: %s?", what, plural(e.Turns, "turn"))
}

// EstimateText is the dialog's text; model is the name the picker shows
// for e.Model (ModelName).
func EstimateText(e agent.RefreshEstimate, model string) string {
	saved := ""
	if e.Saved > 0 {
		verb := "are"
		if e.Saved == 1 {
			verb = "is"
		}
		saved = fmt.Sprintf(" %s of %d reviewed before %s kept.", plural(e.Saved, "part"), e.Chunks, verb)
	}
	return fmt.Sprintf("This makes %s to %s, with about %s of input.%s", plural(e.Calls, "call"), model, plural(e.Tokens, "token"), saved) +
		" Memory changes only when it ends, and Stop cancels it."
}

// UndoChangedText is the toast when an undo of a batch did nothing,
// because sections changed since.
func UndoChangedText(sections []string) string {
	what := "A section"
	if len(sections) > 0 {
		what = strings.Join(sections, "; ")
	}
	return upperFirst(what) + " changed since, so nothing was undone. Edit memory in the settings."
}
