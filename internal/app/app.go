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

	"github.com/amir-saatchi/jenab/internal/bucket"
)

// Deps are what the App needs.
type Deps struct {
	Name   string // the app's name, also the window title
	Assets fs.FS  // the built frontend, with index.html at its root
	// WebviewData is where WebView2 keeps its cache and storage on
	// Windows; empty means Wails' default, %APPDATA%\<exe name>.
	WebviewData string
	Log         *slog.Logger
}

// App is the Wails application.
type App struct {
	w    *application.App
	main *application.WebviewWindow
	pub  *Publisher
	log  *slog.Logger
	name string
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
		Windows: application.WindowsOptions{WebviewUserDataPath: d.WebviewData},
	})
	a.pub = &Publisher{emit: func(name string, data any) { a.w.Event.Emit(name, data) }}
	return a
}

// Publisher sends events to the frontend; agent and project get it.
func (a *App) Publisher() *Publisher { return a.pub }

// Bind registers the services and the objects route. DevService is bound
// only when the developer tools are on at the start (8.4).
func (a *App) Bind(s Services) {
	b := NewServices(s)
	a.w.RegisterService(application.NewService(b.Project))
	a.w.RegisterService(application.NewService(b.Chat))
	a.w.RegisterService(application.NewService(b.Settings))
	a.w.RegisterService(application.NewService(b.Bucket))
	if s.Settings.Get().DevTools {
		a.w.RegisterService(application.NewService(b.Dev))
	}
	a.w.RegisterService(application.NewServiceWithOptions(&objects{objectHandler(s.Projects)}, application.ServiceOptions{Route: bucket.Route}))
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
	a.main = a.w.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     a.name,
		Width:     1280,
		Height:    800,
		MinWidth:  640, // SPEC 5.12
		MinHeight: 480,
	})
	return a.w.Run()
}

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
