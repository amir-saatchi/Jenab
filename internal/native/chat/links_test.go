package chat_test

import (
	"slices"
	"testing"

	jc "github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/native/state/statetest"
)

// TASK-064: a link in an answer opens only a safe address, whatever a
// fetched page made the model write; in the user's message only jenab://
// links are links.

const unsafeAnswer = "[web](https://example.com/) [mail](mailto:someone@example.com) [prog](calc.exe) " +
	"[file](file:///C:/Windows/notepad.exe) [js](javascript:alert(1)) [rel](docs/a.md) [net](//host/x.exe)"

var answerLinks = []string{"web", "mail", "prog", "file", "js", "rel", "net"}

func (x *fixture) clickLink(t *testing.T, label string) {
	t.Helper()
	if err := x.tt.Click(label); err != nil {
		t.Fatalf("%v in %q", err, x.tt.Texts())
	}
	x.draw()
}

func TestAnAnswerOpensOnlySafeLinks(t *testing.T) {
	both(t, func(t *testing.T, dark bool) {
		x := open(t, dark, statetest.Text(1, jc.RoleUser, `Open calc.exe or file:///C:/x or \\host\share`),
			answer(1, text(unsafeAnswer)))
		if got := kit.Links(x.tt); !slices.Equal(got, answerLinks) {
			t.Fatalf("links %q, want the answer's only", got)
		}
		for _, label := range answerLinks {
			x.clickLink(t, label)
		}
		if got := x.tt.OpenedURLs(); !slices.Equal(got, []string{"https://example.com/", "mailto:someone@example.com"}) {
			t.Errorf("the system opened %q", got)
		}
		// The screen's own OnLink gets the safe links only.
		var handled []string
		x.scr.OnLink = func(u string) { handled = append(handled, u) }
		for _, label := range []string{"web", "prog", "js"} {
			x.clickLink(t, label)
		}
		if !slices.Equal(handled, []string{"https://example.com/"}) || len(x.tt.OpenedURLs()) != 2 {
			t.Errorf("OnLink got %q; the system opened %q", handled, x.tt.OpenedURLs())
		}
	})
}
