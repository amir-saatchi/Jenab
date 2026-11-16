package logic

// The placeholders cases are ported from the React app's settings/format.test.ts;
// freeName and the first pick have no test in the React app.

import (
	"reflect"
	"testing"

	"github.com/amir-saatchi/jenab/internal/app"
)

func TestBaseURLPlaceholdersAreFound(t *testing.T) {
	if got := Placeholders("https://api.cloudflare.com/client/v4/accounts/{account_id}/ai/v1/"); !reflect.DeepEqual(got, []string{"account_id"}) {
		t.Errorf("got %q", got)
	}
	if got := Placeholders(""); !reflect.DeepEqual(got, []string{}) {
		t.Errorf("got %q", got)
	}
}

func TestFreeName(t *testing.T) {
	taken := map[string]int{"gemini": 1, "gemini-2": 1, "openai": 1}
	cases := map[string]string{"groq": "groq", "openai": "openai-2", "gemini": "gemini-3"}
	for in, want := range cases {
		if got := FreeName(in, taken); got != want {
			t.Errorf("FreeName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFirstPick(t *testing.T) {
	if got := FirstPick(nil); got != OtherProvider {
		t.Errorf("no presets: %q", got)
	}
	if got := FirstPick([]app.PresetItem{{ID: "ollama"}, {ID: "gemini"}}); got != "ollama" {
		t.Errorf("got %q", got)
	}
}
