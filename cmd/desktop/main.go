// Command desktop is the Jenab app: it builds everything and starts Wails.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

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

// benchArgs gives extra WebView2 arguments; only the bench build sets it.
var benchArgs = func() []string { return nil }

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, errRunning) {
			fatal(err.Error() + ".")
		} else {
			fatal(fmt.Sprintf("%s couldn't start: %v", appName, err))
		}
		os.Exit(1)
	}
}

// run builds everything in import order and blocks until the app quits
// (Q28, CODE-OUTLINE 2). The scheduler, pipelines and updates join it in
// later phases.
func run(_ []string) (err error) {
	paths, err := config.DefaultPaths(appName)
	if err != nil {
		return err
	}
	// One copy at a time, before anything is moved or opened. The file
	// stays open, and locked, until the process ends.
	lock, err := lockRoot(paths.Root)
	if err != nil {
		return err
	}
	defer lock.Close()
	// A config.yaml that can't be parsed gives the defaults and a problem
	// (SPEC 2.1); a save keeps a copy of it.
	settings, problems := config.LoadSettingsOrDefaults(paths.Settings)
	defaultData := paths.DataFolder
	paths = paths.WithDataFolder(settings.DataFolder)
	log, logClose, err := logfile.Open(paths.Logs, settings.DevTools)
	if err != nil {
		return err
	}
	// The shutdown closes the log and the registry, since on macOS Wails
	// ends the process after it and the defers never run. Once, so the
	// defers don't close them again.
	closeLog := sync.OnceValue(logClose)
	defer closeLog()
	defer func() {
		if err != nil {
			log.Error("app: start", "err", err) // main shows it too
		}
	}()
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
	closeRegistry := sync.OnceValue(registry.Close)
	defer closeRegistry()
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
		WebviewData: filepath.Join(paths.Root, "webview"), BrowserArgs: benchArgs()})
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
	// shutdown runs once: from Wails' hook, or after Run returns, which
	// in a server build may be while the hook still runs (Ctrl+C).
	var once sync.Once
	shutdown := func() {
		once.Do(func() {
			_ = app.Shutdown{ // logged inside
				Refuse: []func(){orch.Refuse},
				Cancel: cancel,
				Wait:   []func(context.Context) error{orch.Wait},
				Close: func(ctx context.Context) error {
					return errors.Join(projects.CloseAll(ctx), closeRegistry())
				},
				Log: log,
			}.Run()
			closeLog() // after Run, so its last line is written
		})
	}
	wapp.OnShutdown(shutdown)
	err = wapp.Run()
	shutdown()
	return err
}
