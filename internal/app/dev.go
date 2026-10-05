package app

import (
	"context"

	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/logfile"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/provider"
)

// DevService is the developer tools' read-only view (SPEC 8.4). It is
// bound only when the developer tools are on; P1-17 adds the turn
// inspector and the rest of the runtime panel.
type DevService struct {
	base
	projects *project.Manager
	models   *provider.Registry
	logs     string
}

// Providers lists each provider's calls, limits and pause.
func (s *DevService) Providers(ctx context.Context) (ps []ProviderStatus, err error) {
	defer s.guard("dev.providers", &err)
	return providerStatus(s.models, s.redact), nil
}

// Activity is a project's work and its writers' queues.
func (s *DevService) Activity(ctx context.Context, p id.Project) (a project.Activity, err error) {
	defer s.guard("dev.activity", &err)
	err = open(ctx, s.projects, p, func(proj *project.Project) error {
		a = proj.Activity()
		return nil
	})
	return a, err
}

// Log returns the last log lines that contain match; "" returns the last
// lines.
func (s *DevService) Log(ctx context.Context, match string) (lines []string, err error) {
	defer s.guard("dev.log", &err)
	lines, err = logfile.Tail(s.logs, match)
	for i, l := range lines {
		lines[i] = s.redact(l)
	}
	if lines == nil {
		lines = []string{}
	}
	return lines, err
}
