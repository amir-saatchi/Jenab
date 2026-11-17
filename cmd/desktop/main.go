// Command desktop is the Jenab app on MyGo, with a native UI and no web
// view (SPIKE-048, TASK-061). It builds the services (internal/app); the
// screens are the stores (internal/native/state) shown with the kit
// (internal/native/kit): the frame (internal/native/shell), the chat
// (internal/native/chat) and the developer tools (internal/native/dev).
package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	"github.com/amir-saatchi/jenab/internal/crash"
	"github.com/amir-saatchi/jenab/internal/desktop"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/lifecycle"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/logfile"
	"github.com/amir-saatchi/jenab/internal/native/chat"
	devtools "github.com/amir-saatchi/jenab/internal/native/dev"
	"github.com/amir-saatchi/jenab/internal/native/kit"
	settingsui "github.com/amir-saatchi/jenab/internal/native/settings"
	"github.com/amir-saatchi/jenab/internal/native/shell"
	"github.com/amir-saatchi/jenab/internal/native/state"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/provider/backends"
	"github.com/amir-saatchi/jenab/internal/secret"
	"github.com/amir-saatchi/jenab/internal/skill"
	"github.com/amir-saatchi/jenab/internal/sqlworker"
	"github.com/amir-saatchi/jenab/internal/store"
	"github.com/amir-saatchi/jenab/internal/tool"
	"github.com/amir-saatchi/jenab/internal/update"
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

// run builds everything in order and blocks until the app quits.
func run() (err error) {
	paths, err := config.DefaultPaths(appinfo.Name)
	if err != nil {
		return err
	}
	// One copy runs at a time. The copy an update restarts waits for the
	// old one to let go of the lock.
	lockStart := time.Now()
	lock, err := lockApp(paths.Root, os.Args[1:])
	if err != nil {
		return err
	}
	lockWaited := time.Since(lockStart)
	defer lock.Close()
	// A crash, a panic nothing recovers on any goroutine or a fatal
	// error, writes its stacks to a file in the logs folder; the next
	// start shows it (TASK-067). Without it a crash of the windowed app
	// left no trace: it has no console. stopCrash runs at the end of a
	// normal quit, not in a defer: a panic runs the defers before the
	// runtime writes it, so a deferred stop would turn the output off
	// first. A start that fails leaves an empty file, which the next start
	// removes.
	crashFile, stopCrash, crashErr := crash.Start(paths.Logs)
	if crashErr != nil {
		stopCrash = func() {}
	}
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
	if lockWaited > 200*time.Millisecond {
		log.Info("app: waited for a copy that was quitting", "for", lockWaited.Round(time.Millisecond))
	}
	for _, p := range problems {
		log.Warn("setting ignored", "problem", p.String())
	}
	// After an update the Apps entry still had the version installed.
	if wrote, err := refreshAppsEntry(v.Version); err != nil {
		log.Warn("app: apps entry", "err", err)
	} else if wrote {
		log.Info("app: apps entry", "version", v.Version)
	}
	if err := kit.RegisterFonts(); err != nil {
		return err
	}
	kit.SetThemeSource(settings.UI.Theme)
	ctx, cancel := context.WithCancel(context.Background()) // the app context (Q20)
	defer cancel()

	live := app.NewSettings(paths.Settings, settings, problems)
	secrets := secret.New(newKeyring())
	if crashErr != nil {
		log.Warn("app: crashes will not be kept", "err", crashErr)
	}
	// The crash output of earlier runs becomes crash logs, without the
	// secrets.
	crashes, err := crash.Collect(paths.Logs, crashFile, crashRedactor(live.Get(), secrets))
	if err != nil {
		log.Warn("app: crash logs", "err", err)
	}
	for _, c := range crashes {
		log.Warn("app: an earlier run crashed", "log", filepath.Base(c))
	}
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
	appNotify = notifier(func() *mygo.Window { return win }, pub)
	b := app.NewServices(app.Services{Orchestrator: orch, Projects: projects, Registry: registry, Models: models,
		Secrets: secrets, Settings: live, Traces: traces, Logs: paths.Logs, Log: log,
		Events: pub, Notify: appNotify, OpenFile: mygo.Shell.OpenPath,
		Started: app.Started{DataFolder: paths.DataFolder, DefaultDataFolder: defaultData, DevTools: settings.DevTools, Moved: moved}})
	go b.CleanOpened()

	stores = state.NewStores(state.API{Projects: b.Project, Chats: b.Chat, Settings: b.Settings, System: b.System,
		Bucket: app.NativeBucket{BucketService: b.Bucket}, Status: barStatus{b.System, b.Settings}}, mailbox.Post)
	defer stores.Close()
	if t, ok := crashNotice(crashes, mygo.Shell.OpenPath); ok {
		mailbox.Post(func() { stores.Toasts = append(stores.Toasts, t) })
	}
	// The window's buttons follow the app's theme, over the app's own
	// title bar (TASK-063).
	stores.Settings.OnTheme = func(s string) {
		kit.SetThemeSource(s)
		if win != nil {
			win.SetWindowButtonsTheme(kit.ThemeSourceOf(s))
		}
	}
	// Updates (SPEC 2.8, TASK-059): checked at start, then every 6 hours.
	// The automatic restart waits until no work goes on: every turn, with
	// its subagents, every memory refresh and call (the orchestrator), and
	// every project's leased work (TASK-067).
	appBusy = func() bool { return orch.Busy() || projects.Busy() }
	updates := update.New(update.Deps{
		Updater: mygoUpdater{}, Current: v.Version, Log: log, Relaunch: relaunch,
		Settings: func() config.UpdateSettings { return live.Get().Updates },
		CanRestart: func() bool {
			return !appBusy() && onUI(mailbox.Post, func() bool { return !stores.WindowActive() })
		},
		OnStatus: func(st update.Status) { mailbox.Post(func() { stores.UpdateStatus(st) }) },
	})
	stores.Updates.API = updates
	updateService = updates
	go updates.Run(ctx)
	stores.Actions.Upload = uploader(app.UploadHandler(projects))
	stores.Actions.PickFiles = func() []string {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Title: "Attach files", Multiple: true, Parent: win})
		if err != nil {
			log.Warn("attach: the file dialog failed", "err", err)
		}
		return paths
	}
	// The exports ask where to save off the UI thread (TASK-057).
	stores.Exports.Save = func(r state.SaveRequest) (string, error) {
		opts := mygo.SaveDialogOptions{Parent: win, Title: r.Title, DefaultPath: r.Name, CreateDirectories: true}
		if r.Ext != "" {
			opts.Filters = []mygo.FileFilter{{Name: r.FilterName, Extensions: []string{r.Ext}}}
		}
		return mygo.Dialog.Save(opts)
	}
	stores.Pages.PickFolder = func(title, start string) string {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Title: title, DefaultPath: start, Directory: true, Parent: win})
		if err != nil {
			log.Warn("data folder: the folder dialog failed", "err", err)
		}
		if len(paths) == 0 {
			return ""
		}
		return paths[0]
	}
	sh := shell.New(stores, chat.New(stores), mailbox.Post)
	sh.Settings = settingsui.New(stores, mailbox.Post)
	sh.Projects.ProjectMemory = settingsui.ProjectMemory(stores)
	sh.Quit = mygo.App.Quit
	sh.Dev = devtools.New(stores, state.NewDev(b.Dev, mailbox.Post))

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
			// The app's top row is the title bar, under Windows' own
			// buttons (TASK-063). macOS and Linux keep the system's until
			// TASK-060.
			TitleBarStyle: titleBarStyle(),
			Content:       kit.View(sh.View),
		})
		win.SetWindowButtonsTheme(kit.ThemeSourceOf(live.Get().UI.Theme))
		win.OnClose(func(e *mygo.CloseEvent) {
			if !quitting.Load() {
				e.PreventDefault()
				win.Hide()
			}
		})
		stores.WindowActive = func() bool { return win.IsVisible() && !win.IsMinimized() && win.IsFocused() }
		stores.Bar.Visible = func() bool { return win.IsVisible() && !win.IsMinimized() }
		mailbox.Attach(win.Update)
		sh.Start()
		if benchHook != nil {
			benchHook(stores, mailbox.Post, win)
		}
		// Closing the window only hides it, so the tray shows it again and
		// quits.
		if err := newTray(func() { show(win) }); err != nil {
			log.Warn("tray: not shown", "err", err)
		}
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
	stopCrash()
	return err
}

// titleBarStyle hides the system's title bar on Windows, where the
// window keeps its buttons over the app's top row; elsewhere the system's
// stays (TASK-060).
func titleBarStyle() mygo.TitleBarStyle {
	if runtime.GOOS == "windows" {
		return mygo.TitleBarHidden
	}
	return mygo.TitleBarDefault
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
// app:open, so the stores open the chat.
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

// benchHook is set by a build with the bench tag (bench.go), to let
// cmd/jenab-bench drive the app.
var benchHook func(*state.Stores, state.Post, *mygo.Window)

// appNotify and updateService are the notifier and the updater, which the
// bench build drives too (TASK-059).
var (
	appNotify     func(app.Notification) error
	updateService *update.Service
	// appBusy reports whether any work goes on, which an automatic
	// restart must not cut off.
	appBusy func() bool
)

// newKeyring is where the app keeps its secrets: the OS keychain, or in a
// build with the bench tag a keyring in memory, so that the bench never
// reads or writes the user's.
var newKeyring = func() secret.Keyring { return secret.OSKeyring(strings.ToLower(appinfo.Name)) }

//go:embed tray.png
var trayIcon []byte

// newTray puts Jenab in the notification area: a click shows the window,
// and its menu shows it or quits.
func newTray(open func()) error {
	tray, err := mygo.NewTray(mygo.TrayOptions{ID: "jenab", Icon: trayIcon, ToolTip: appinfo.Name,
		Menu: mygo.NewMenu([]*mygo.MenuItem{
			{ID: "show", Label: "Show " + appinfo.Name, Click: func(*mygo.MenuItem, *mygo.Window) { open() }},
			mygo.Separator(),
			{ID: "quit", Label: "Quit " + appinfo.Name, Role: mygo.RoleQuit},
		})})
	if err != nil {
		return err
	}
	tray.OnClick(open)
	return nil
}

// uploader stores a file through the services' upload route, called
// in-process, so a file is stored as the route stores it.
func uploader(h http.Handler) state.Uploader {
	return func(ctx context.Context, p id.Project, path string) (app.Uploaded, error) {
		body, err := os.ReadFile(path)
		if err != nil {
			return app.Uploaded{}, err
		}
		req := httptest.NewRequestWithContext(ctx, http.MethodPost, app.UploadRoute+string(p), bytes.NewReader(body))
		req.Header.Set(app.UploadNameHeader, url.PathEscape(filepath.Base(path)))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			var e struct{ Error string }
			if json.Unmarshal(rec.Body.Bytes(), &e) == nil && e.Error != "" {
				return app.Uploaded{}, errors.New(e.Error)
			}
			return app.Uploaded{}, fmt.Errorf("the upload failed (%d)", rec.Code)
		}
		var up app.Uploaded
		return up, json.Unmarshal(rec.Body.Bytes(), &up)
	}
}

// barStatus is what the bottom bar reads (TASK-056): the memory the system
// can still hand out, and the providers' pauses.
type barStatus struct {
	*app.SystemService
	settings *app.SettingsService
}

func (s barStatus) Providers(ctx context.Context) ([]app.ProviderStatus, error) {
	return s.settings.Providers(ctx)
}

// onUI runs fn on the UI thread and returns its answer, or false when the
// UI doesn't answer within 2 s, as while the app quits.
func onUI(post state.Post, fn func() bool) bool {
	answer := make(chan bool, 1)
	post(func() { answer <- fn() })
	select {
	case ok := <-answer:
		return ok
	case <-time.After(2 * time.Second):
		return false
	}
}
