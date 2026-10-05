package app

import (
	"context"
	"log/slog"

	"github.com/amir-saatchi/jenab/internal/agent"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/secret"
	"github.com/amir-saatchi/jenab/internal/store"
)

// Services are what the bound services work with.
type Services struct {
	Orchestrator *agent.Orchestrator
	Projects     *project.Manager
	Registry     *store.Registry
	Models       *provider.Registry
	Secrets      *secret.Store // removes secrets from error details; nil removes nothing
	Settings     *Settings
	Logs         string // the log folder, for the developer tools
	Log          *slog.Logger
}

// Bound are the services the frontend calls (Q31): one thin service per
// area. Every method takes ctx first, returns typed structs, and ends with
// guard, so its errors are *UIError values logged once. Later tickets add
// the methods their screens need; PageService and PipelineService come
// with the pages and pipelines.
type Bound struct {
	Project  *ProjectService
	Chat     *ChatService
	Settings *SettingsService
	Bucket   *BucketService
	Dev      *DevService // bound only with the developer tools on
}

// NewServices builds the services. They work without Wails, so tests
// drive them directly.
func NewServices(s Services) Bound {
	if s.Log == nil {
		s.Log = slog.New(slog.DiscardHandler)
	}
	b := base{log: s.Log, redact: func(t string) string { return t }}
	if s.Secrets != nil {
		b.redact = s.Secrets.Redact
	}
	return Bound{
		Project:  &ProjectService{base: b, projects: s.Projects},
		Chat:     &ChatService{base: b, orch: s.Orchestrator, projects: s.Projects},
		Settings: &SettingsService{base: b, settings: s.Settings, models: s.Models},
		Bucket:   &BucketService{base: b, projects: s.Projects},
		Dev:      &DevService{base: b, projects: s.Projects, models: s.Models, logs: s.Logs},
	}
}

// base is what every service shares.
type base struct {
	log    *slog.Logger
	redact func(string) string
}

// open leases a project for one call.
func open(ctx context.Context, pm *project.Manager, p id.Project, fn func(*project.Project) error) error {
	proj, err := pm.Open(ctx, p)
	if err != nil {
		return err
	}
	defer proj.Release()
	return fn(proj)
}
