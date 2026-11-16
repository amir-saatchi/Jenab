// Command desktop-native is the Jenab app on MyGo, with a native UI and no
// web view (SPIKE-048). It builds the services as cmd/desktop does; the
// screens are the stores (internal/native/state) shown with the kit
// (internal/native/kit). Part A shows a placeholder; the chat comes in
// Part B.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"

	"github.com/amir-saatchi/jenab/internal/agent"
	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/appinfo"
	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/desktop"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/lifecycle"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/logfile"
	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/native/state"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/provider/backends"
	"github.com/amir-saatchi/jenab/internal/secret"
	"github.com/amir-saatchi/jenab/internal/skill"
	"github.com/amir-saatchi/jenab/internal/sqlworker"
	"github.com/amir-saatchi/jenab/internal/store"
	"github.com/amir-saatchi/jenab/internal/tool"
	"github.com/amir-saatchi/jenab/internal/version"
	"github.com/amir-saatchi/jenab/internal/web"
)

func main() {
	// A query worker (SPEC 2.2) is this executable started again; it must
	// not take a lock or open a window.
	sqlworker.IfWorker()
	mygo.App.SetName(appinfo.Name)
	// A second launch passes its command line to the running app, which
	// shows its window, and ends here.
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	if err := run(); err != nil {
		if errors.Is(err, lifecycle.ErrRunning) {
			lifecycle.Fatal(err.Error() + ".")
		} else {
			lifecycle.Fatal(fmt.Sprintf("%s couldn't start: %v", appinfo.Name, err))
		}
		os.Exit(1)
	}
}

// run builds everything in the order cmd/desktop does and blocks until
// the app quits.
func run() (err error) {
	paths, err := config.DefaultPaths(appinfo.Name)
	if err != nil {
		return err
	}
	// The Wails app takes the same lock: one of the two runs at a time.
	lock, err := lifecycle.LockRoot(paths.Root)
	if err != nil {
		return err
	}
	defer lock.Close()
	settings, problems := config.LoadSettingsOrDefaults(paths.Settings)
	defaultData := paths.DataFolder
	paths = paths.WithDataFolder(settings.DataFolder)
	log, logClose, err := logfile.Open(paths.Logs, settings.DevTools)
	if err != nil {
		return err
	}
	closeLog := sync.OnceValue(logClose)
	defer closeLog()
	v := version.Get()
	log.Info("app: start", "ui", "native", "version", v.Version, "commit", v.Commit, "modified", v.Modified, "go", v.Go, "os", v.OS)
	defer func() {
		if err != nil {
			log.Error("app: start", "err", err)
		}
	}()
	for _, p := range problems {
		log.Warn("setting ignored", "problem", p.String())
	}
	if err := kit.RegisterFonts(); err != nil {
		return err
	}
	kit.SetThemeSource(settings.UI.Theme)
	ctx, cancel := context.WithCancel(context.Background()) // the app context (Q20)
	defer cancel()

	live := app.NewSettings(paths.Settings, settings, problems)
	secrets := secret.New(secret.OSKeyring(strings.ToLower(appinfo.Name)))
	registry, err := store.OpenRegistry(ctx, paths.Registry)
	if err != nil {
		return err
	}
	closeRegistry := sync.OnceValue(registry.Close)
	defer closeRegistry()
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

	// The Publisher sends the events to the stores, on the UI thread. The
	// mailbox keeps those sent before the window exists. Window.Update
	// returns without waiting, so the services never wait on the UI.
	var mailbox state.Mailbox
	var stores *state.Stores // set before the mailbox is attached
	pub := app.NewPublisher(func(name string, data any) {
		mailbox.Post(func() { stores.Apply(name, data) })
	})

	projects := project.NewManager(project.Deps{Paths: paths, Registry: registry, Log: log, Events: pub,
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
	var chatReset func(id.Project, id.Chat)
	if runtime.GOOS == "windows" { // desktop control is Windows-only in v1 (SPEC 8.10)
		prog, _ := desktop.ProgramPath()
		dm := desktop.NewManager(desktop.Deps{
			Runner: &desktop.CLI{Path: prog, Home: filepath.Join(paths.Root, "agent-desktop")},
			On:     func() bool { return live.Get().DesktopControl },
			Self:   os.Getpid(), Class: desktop.WindowClass, Log: log, Store: registry,
		})
		tools.Add(dm.Tools()...)
		chatReset = dm.EndChat
	}
	skills, err := skill.Builtin()
	if err != nil {
		return err
	}
	var traces *agent.Traces
	if settings.DevTools {
		traces = agent.NewTraces()
	}
	orch := agent.New(agent.Deps{Context: ctx, Projects: projects, Models: models, Tools: tools,
		Settings: live.Get, Events: pub, Skills: skills, Traces: traces, Log: log, ChatReset: chatReset,
		Card: func(ctx context.Context, p *project.Project) (string, error) { return p.DB.Card(ctx, time.Now()) }})

	var win *mygo.Window // set in WhenReady
	b := app.NewServices(app.Services{Orchestrator: orch, Projects: projects, Registry: registry, Models: models,
		Secrets: secrets, Settings: live, Traces: traces, Logs: paths.Logs, Log: log,
		Events: pub, Notify: notifier(func() *mygo.Window { return win }, pub), OpenFile: mygo.Shell.OpenPath,
		Started: app.Started{DataFolder: paths.DataFolder, DefaultDataFolder: defaultData, DevTools: settings.DevTools, Moved: moved}})
	go b.CleanOpened()

	stores = state.NewStores(state.API{Projects: b.Project, Chats: b.Chat, Settings: b.Settings, System: b.System}, mailbox.Post)
	defer stores.Close()
	stores.Settings.OnTheme = kit.SetThemeSource
	sh := &shell{stores: stores, post: mailbox.Post, quit: mygo.App.Quit}

	var quitting atomic.Bool
	mygo.App.OnSecondInstance(func([]string, string) { show(win) })
	// Closing the window hides it; the app keeps running for the chats
	// still working, and a second launch or a notification shows it again.
	mygo.App.OnWindowAllClosed(func() {})
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) {
		quitting.Store(true)
		if win != nil {
			win.Hide() // so a slow shutdown shows no frozen window
		}
	})
	mygo.App.WhenReady(func() {
		win = mygo.NewWindow(mygo.WindowOptions{
			Title: appinfo.Name, Width: 1280, Height: 800,
			MinWidth: 640, MinHeight: 480, // SPEC 5.12
			StateKey: "main",
			Content:  kit.View(sh.view),
		})
		win.OnClose(func(e *mygo.CloseEvent) {
			if !quitting.Load() {
				e.PreventDefault()
				win.Hide()
			}
		})
		stores.WindowActive = func() bool { return win.IsVisible() && !win.IsMinimized() && win.IsFocused() }
		mailbox.Attach(win.Update)
		sh.start()
	})

	err = mygo.App.Run()
	_ = lifecycle.Shutdown{ // logged inside
		Refuse: []func(){orch.Refuse},
		Cancel: cancel,
		Wait:   []func(context.Context) error{orch.Wait},
		Close: func(ctx context.Context) error {
			return errors.Join(projects.CloseAll(ctx), closeRegistry())
		},
		Log: log,
	}.Run()
	closeLog()
	return err
}

// show brings the window to the front.
func show(win *mygo.Window) {
	if win == nil {
		return
	}
	if win.IsMinimized() {
		win.Restore()
	}
	win.Show()
	win.Focus()
}

// notifier shows desktop notifications. A click shows the window and sends
// app:open, so the stores open the chat, as in the Wails app.
func notifier(win func() *mygo.Window, pub *app.Publisher) func(app.Notification) error {
	return func(n app.Notification) error {
		note := mygo.NewNotification(mygo.NotificationOptions{ID: "chat-" + string(n.Chat), Title: n.Title, Body: n.Body})
		note.OnClick(func() {
			show(win())
			pub.Open(app.Open{Project: n.Project, Chat: n.Chat})
		})
		return note.Show()
	}
}
