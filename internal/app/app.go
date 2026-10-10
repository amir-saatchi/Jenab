// Package app is the thin Wails layer between Go and the React frontend
// (CODE-OUTLINE 9, Q31–Q33): the bound services, the events, the error
// shape the frontend gets, the objects route and the quit order. It holds
// no logic of its own; everything below it works without Wails.
package app

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"github.com/amir-saatchi/jenab/internal/bucket"
	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/id"
)

// Deps are what the App needs.
type Deps struct {
	Name   string // the app's name, also the window title
	Assets fs.FS  // the built frontend, with index.html at its root
	// WebviewData is where WebView2 keeps its cache and storage on
	// Windows; empty means Wails' default, %APPDATA%\<exe name>.
	WebviewData string
	// BrowserArgs are extra WebView2 arguments on Windows. Only the bench
	// build sets them (cmd/desktop/bench.go).
	BrowserArgs []string
	Log         *slog.Logger
}

// App is the Wails application.
type App struct {
	w    *application.App
	main *application.WebviewWindow
	pub  *Publisher
	log  *slog.Logger
	name string
	ui   *Settings // set by Bind
}

// New makes the Wails application. It shows nothing until Run, but events
// can be sent from now on.
func New(d Deps) *App {
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	a := &App{log: d.Log, name: d.Name}
	a.w = application.New(application.Options{
		Name:   d.Name,
		Assets: application.AssetOptions{Handler: application.AssetFileServerFS(d.Assets)},
		// Wails logs every asset request at Info; the log gets its
		// warnings and errors only.
		Logger:  slog.New(minLevel{d.Log.Handler(), slog.LevelWarn}),
		Windows: application.WindowsOptions{WebviewUserDataPath: d.WebviewData, AdditionalBrowserArgs: d.BrowserArgs},
	})
	a.pub = &Publisher{emit: func(name string, data any) { a.w.Event.Emit(name, data) }}
	return a
}

// Publisher sends events to the frontend; agent and project get it.
func (a *App) Publisher() *Publisher { return a.pub }

// Bind registers the services and the objects route. DevService is bound
// only when the developer tools are on at the start (8.4).
func (a *App) Bind(s Services) {
	a.ui = s.Settings
	s.Settings.OnChange(func(c config.Settings) {
		if a.main != nil {
			bg, _ := windowTheme(c.UI.Theme, systemDark())
			a.main.SetBackgroundColour(bg)
		}
	})
	b := NewServices(s)
	b.System.notify = a.notifier()
	a.w.RegisterService(application.NewService(b.Project))
	a.w.RegisterService(application.NewService(b.Chat))
	a.w.RegisterService(application.NewService(b.Settings))
	a.w.RegisterService(application.NewService(b.Bucket))
	a.w.RegisterService(application.NewService(b.System))
	if s.Settings.Get().DevTools {
		a.w.RegisterService(application.NewService(b.Dev))
	}
	a.w.RegisterService(application.NewServiceWithOptions(&objects{objectHandler(s.Projects)}, application.ServiceOptions{Route: bucket.Route}))
}

// notifier sends desktop notifications through Wails' service. On
// Windows it registers the app with the toast system under
// HKCU\Software\Classes at start. A click shows the window and sends
// app:open, so the frontend opens the chat.
func (a *App) notifier() func(Notification) error {
	ns := notifications.New()
	a.w.RegisterService(application.NewService(ns))
	ns.OnNotificationResponse(func(r notifications.NotificationResult) {
		if r.Error != nil {
			a.log.Warn("notification response", "err", r.Error)
			return
		}
		p, _ := r.Response.UserInfo["project"].(string)
		c, _ := r.Response.UserInfo["chat"].(string)
		if a.main != nil {
			a.main.UnMinimise()
			a.main.Show()
			a.main.Focus()
		}
		if p != "" && c != "" {
			a.pub.emit(EventOpen, Open{Project: id.Project(p), Chat: id.Chat(c)})
		}
	})
	return func(n Notification) error {
		return ns.SendNotification(notifications.NotificationOptions{
			ID: "chat-" + string(n.Chat), Title: n.Title, Body: n.Body,
			Data: map[string]any{"project": string(n.Project), "chat": string(n.Chat)},
		})
	}
}

// objects is the route for /objects/<project_id>/<key>. Wails binds no
// methods of it: ServeHTTP is left out of the bindings.
type objects struct{ h http.Handler }

func (o *objects) ServeHTTP(w http.ResponseWriter, r *http.Request) { o.h.ServeHTTP(w, r) }

// OnShutdown runs fn when the app quits, after the window is hidden, so a
// slow quit doesn't show a frozen window. fn should be a Shutdown's Run.
func (a *App) OnShutdown(fn func()) {
	a.w.OnShutdown(func() {
		if a.main != nil {
			a.main.Hide()
		}
		fn()
	})
}

// Run opens the main window and blocks until the app quits.
func (a *App) Run() error {
	theme := "system"
	if a.ui != nil {
		theme = a.ui.Get().UI.Theme
	}
	bg, frame := windowTheme(theme, systemDark())
	a.main = a.w.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            a.name,
		Width:            1280,
		Height:           800,
		MinWidth:         640, // SPEC 5.12
		MinHeight:        480,
		BackgroundColour: bg,
		Windows:          application.WindowsWindow{Theme: frame},
	})
	return a.w.Run()
}

// windowTheme is the window's background before the page paints, so a dark
// theme shows no white flash, and the Windows frame for ui.theme (SPEC
// 5.11). The colours are --background in index.css. Wails sets the frame
// only when the window opens; a later change applies after a restart.
func windowTheme(theme string, systemDark bool) (application.RGBA, application.Theme) {
	switch theme {
	case "light":
		return lightBackground, application.Light
	case "dark":
		return darkBackground, application.Dark
	}
	if systemDark {
		return darkBackground, application.SystemDefault
	}
	return lightBackground, application.SystemDefault
}

var (
	lightBackground = application.RGBA{Red: 255, Green: 255, Blue: 255, Alpha: 255}
	darkBackground  = application.RGBA{Red: 10, Green: 10, Blue: 10, Alpha: 255} // oklch(0.145 0 0)
)

// minLevel drops records below level.
type minLevel struct {
	slog.Handler
	level slog.Level
}

func (h minLevel) Enabled(ctx context.Context, l slog.Level) bool {
	return l >= h.level && h.Handler.Enabled(ctx, l)
}

func (h minLevel) WithAttrs(as []slog.Attr) slog.Handler {
	return minLevel{h.Handler.WithAttrs(as), h.level}
}

func (h minLevel) WithGroup(name string) slog.Handler {
	return minLevel{h.Handler.WithGroup(name), h.level}
}
