package app

import (
	"errors"
	"slices"

	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/secret"
)

// withKeys adds to v the providers on this computer that have a key in the
// keychain, so their rows can tell a token from none (TASK-074). Only
// whether a key is there is looked at; its value isn't used.
func (s *SettingsService) withKeys(v SettingsView) SettingsView {
	v.LocalKeys = []string{}
	if s.secrets == nil {
		return v
	}
	for name, p := range v.Settings.LLM.Providers {
		if !provider.LocalURL(provider.Kind(p.Kind), p.BaseURL) {
			continue
		}
		_, err := s.secrets.Get(provider.KeyName(name))
		switch {
		case err == nil:
			v.LocalKeys = append(v.LocalKeys, name)
		case !errors.Is(err, secret.ErrNotFound):
			s.log.Warn("settings: reading the keychain", "provider", name, "err", err)
		}
	}
	slices.Sort(v.LocalKeys)
	return v
}
