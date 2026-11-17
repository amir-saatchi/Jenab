package main

import (
	"path/filepath"

	"github.com/amir-saatchi/jenab/internal/appinfo"
	"github.com/amir-saatchi/jenab/internal/config"
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

// crashNotice is the toast that tells the user the last run crashed, with
// a button that opens the newest crash log. ok is false with no crash
// logs.
func crashNotice(logs []string, open func(string) error) (t state.Toast, ok bool) {
	if len(logs) == 0 {
		return state.Toast{}, false
	}
	last := logs[len(logs)-1]
	return state.Toast{
		Text:    appinfo.Name + " closed unexpectedly",
		Detail:  "What happened is in " + filepath.Base(last) + ", in the logs folder. Nothing was sent anywhere.",
		Warning: true,
		Sticky:  true,
		Action:  "Open the log",
		OnAction: func() {
			if open != nil {
				open(last)
			}
		},
	}, true
}
