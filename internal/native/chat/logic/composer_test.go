package logic

// modelName has no test in the React app; these say what its comment does.

import (
	"testing"

	"github.com/amir-saatchi/jenab/internal/app"
)

func TestModelName(t *testing.T) {
	groups := []app.ModelGroup{{Provider: "gemini", Models: []app.ModelItem{
		{Ref: "gemini/gemini-3-pro", Name: "Gemini 3 Pro", Aliases: []string{"default"}},
		{Ref: "gemini/gemini-3-flash", Name: "Gemini 3 Flash", Aliases: []string{"fast"}},
	}}}
	cases := []struct {
		groups []app.ModelGroup
		model  string
		want   string
	}{
		{nil, "default", "No model"},
		{groups, "gemini/gemini-3-flash", "Gemini 3 Flash"},
		{groups, "default", "Gemini 3 Pro"},
		{groups, "other/model", "other/model"},
	}
	for _, c := range cases {
		if got := ModelName(c.groups, c.model); got != c.want {
			t.Errorf("ModelName(%q) = %q, want %q", c.model, got, c.want)
		}
	}
}
