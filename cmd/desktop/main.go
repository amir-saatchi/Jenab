// Command desktop is the Jenab app: it builds everything and starts Wails.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/amir-saatchi/jenab/frontend"
	"github.com/amir-saatchi/jenab/internal/agent"
	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/logfile"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/provider/backends"
	"github.com/amir-saatchi/jenab/internal/secret"
	"github.com/amir-saatchi/jenab/internal/skill"
	"github.com/amir-saatchi/jenab/internal/store"
	"github.com/amir-saatchi/jenab/internal/tool"
	"github.com/amir-saatchi/jenab/internal/web"
)

// appName is the one place the app's name is set (Q1).
const appName = "Jenab"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run builds everything in import order and blocks until the app quits
// (Q28, CODE-OUTLINE 2). The scheduler, pipelines and updates join it in
// later phases.
func run(_ []string) error {
	paths, err := config.DefaultPaths(appName)
	if err != nil {
		return err
	}
	settings, problems, err := config.LoadSettings(paths.Settings)
	if err != nil {
		return err
	}
	defaultData := paths.DataFolder
	paths = paths.WithDataFolder(settings.DataFolder)
	log, closeLog, err := logfile.Open(paths.Logs, settings.DevTools)
	if err != nil {
		return err
	}
	defer closeLog()
	for _, p := range problems {
		log.Warn("setting ignored", "problem", p.String())
	}
	ctx, cancel := context.WithCancel(context.Background()) // the app context (Q20)
	defer cancel()

	assets, err := frontend.Assets()
	if err != nil {
		return err
	}
	live := app.NewSettings(paths.Settings, settings, problems)
	secrets := secret.New(secret.OSKeyring(strings.ToLower(appName)))
	registry, err := store.OpenRegistry(ctx, paths.Registry)
	if err != nil {
		return err
	}
	defer registry.Close()
	// Before any project opens: a changed data folder moves the projects
	// (SPEC 2.1). One that can't move opens from its old place.
	moved, err := project.MoveProjects(ctx, paths, registry, log)
	if err != nil {
		log.Error("project: moving the projects to the new data folder failed", "err", err)
	}
	calls := limit.NewGate(settings.LLM.MaxParallelCalls)
	models := provider.NewRegistry(provider.Deps{Settings: settings.LLM, Secrets: secrets, Gate: calls, Backends: backends.All(), Log: log})
	live.OnChange(func(s config.Settings) {
		calls.SetSize(s.LLM.MaxParallelCalls)
		models.Apply(s.LLM)
	})

	wapp := app.New(app.Deps{Name: appName, Assets: assets, Log: log, // first, so events can be sent
		WebviewData: filepath.Join(paths.Root, "webview")})
	projects := project.NewManager(project.Deps{Paths: paths, Registry: registry, Log: log, Events: wapp.Publisher(),
		Level: func() project.Level {
			l, err := project.ParseLevel(live.Get().Approvals.DefaultLevel)
			if err != nil {
				return project.Standard
			}
			return l
		}})
	tools := tool.NewRegistry()
	tools.Add(tool.Builtin(tool.Deps{Web: web.NewClient(web.Options{})})...)
	tools.Add(agent.Tools()...)
	skills, err := skill.Builtin()
	if err != nil {
		return err
	}
	var traces *agent.Traces // the turn inspector's record (SPEC 8.4)
	if settings.DevTools {
		traces = agent.NewTraces()
	}
	orch := agent.New(agent.Deps{Context: ctx, Projects: projects, Models: models, Tools: tools,
		Settings: live.Get, Events: wapp.Publisher(), Skills: skills, Traces: traces, Log: log})

	wapp.Bind(app.Services{Orchestrator: orch, Projects: projects, Registry: registry, Models: models,
		Secrets: secrets, Settings: live, Traces: traces, Logs: paths.Logs, Log: log,
		Started: app.Started{DataFolder: paths.DataFolder, DefaultDataFolder: defaultData, DevTools: settings.DevTools, Moved: moved}})
	wapp.OnShutdown(func() {
		_ = app.Shutdown{ // logged inside
			Refuse: []func(){orch.Refuse},
			Cancel: cancel,
			Wait:   []func(context.Context) error{orch.Wait},
			Close:  projects.CloseAll,
			Log:    log,
		}.Run()
	})
	return wapp.Run()
}
