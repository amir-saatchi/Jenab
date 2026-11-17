package main

import (
	"path/filepath"

	"github.com/amir-saatchi/jenab/internal/appinfo"
	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/crash"
	"github.com/amir-saatchi/jenab/internal/native/state"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/secret"
)

// crashRedactor hides every provider key and base-URL field the settings
// name, as the logs and exports do (SPEC 6.7), for the crash logs
// (TASK-067).
func crashRedactor(s config.Settings, secrets *secret.Store) func(string) string {
	var names []string
	for name, ps := range s.LLM.Providers {
		names = append(names, provider.KeyName(name))
		base := ps.BaseURL
		if base == "" {
			base = provider.DefaultBaseURL(provider.Kind(ps.Kind))
		}
		for _, f := range provider.Placeholders(base) {
			names = append(names, provider.FieldName(name, f))
		}
	}
	return secrets.Redactor(names...)
}

// crashNotice is the toast that tells the user an earlier run crashed,
// with a button that opens the newest crash log. When no log could be
// kept, it says so in the text, since a toast without a button shows no
// detail line (TASK-073). ok is false with no crash.
func crashNotice(res crash.Result, open func(string) error) (t state.Toast, ok bool) {
	t = state.Toast{Text: appinfo.Name + " closed unexpectedly", Warning: true, Sticky: true}
	if len(res.Logs) == 0 {
		if len(res.Lost) == 0 {
			return state.Toast{}, false
		}
		t.Text += ". Its crash log couldn't be written, so what the crash left was deleted: it can hold keys. Nothing was sent anywhere."
		return t, true
	}
	last := res.Logs[len(res.Logs)-1]
	t.Detail = "What happened is in " + filepath.Base(last) + ", in the logs folder. Nothing was sent anywhere."
	t.Action = "Open the log"
	t.OnAction = func() {
		if open != nil {
			open(last)
		}
	}
	return t, true
}
