// SPIKE-003, Wails v3 app: tray, hide on close, asset route, repaint check.
package main

import (
	"io/fs"
	"log"
	"net/http"
	"os"
	rdebug "runtime/debug"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/icons"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

type v3win struct {
	app *application.App
	w   *application.WebviewWindow
	ns  *notifications.NotificationService
}

func (v v3win) Minimise() { application.InvokeSync(func() { v.w.Minimise() }) }
func (v v3win) Restore()  { application.InvokeSync(func() { v.w.Restore() }) }
func (v v3win) Hide()     { application.InvokeSync(func() { v.w.Hide() }) }
func (v v3win) Show()     { application.InvokeSync(func() { v.w.Show(); v.w.Focus() }) }
func (v v3win) Quit()     { v.app.Quit() }
func (v v3win) Notify() error {
	return v.ns.SendNotification(notifications.NotificationOptions{ID: "spike003", Title: "Burrow SPIKE-003", Body: "Test notification from Wails v3"})
}

func main() {
	out, frameless, alpha, label := "result-v3.json", false, uint8(255), "Wails "+wailsVersion()
	for _, a := range os.Args[1:] {
		switch {
		case strings.HasSuffix(a, ".json"):
			out = a
		case a == "--frameless":
			frameless = true
			label += ", frameless"
		case a == "--alpha1":
			alpha = 1
			label += ", background alpha 1"
		}
	}
	sub, _ := fs.Sub(assets, "frontend")
	files := application.AssetFileServerFS(sub)
	own := handler()
	ns := notifications.New()
	var v v3win
	app := application.New(application.Options{
		Name: "Burrow SPIKE-003 v3",
		Assets: application.AssetOptions{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/objects/") || r.URL.Path == "/report" || r.URL.Path == "/state" || r.URL.Path == "/input" {
				own.ServeHTTP(w, r)
				return
			}
			files.ServeHTTP(w, r)
		})},
		Services: []application.Service{application.NewService(ns)},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "burrow.spike003.v3",
			OnSecondInstanceLaunch: func(d application.SecondInstanceData) {
				if v.w != nil {
					v.w.Show()
					v.w.Focus()
				}
				second <- strings.Join(d.Args, " ")
			},
		},
	})
	v.app, v.ns = app, ns
	v.w = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            title,
		Width:            800,
		Height:           600,
		URL:              "/",
		BackgroundColour: application.NewRGBA(255, 0, 255, alpha), // magenta: shows up as "not painted" if the page is missing
		Frameless:        frameless,
	})
	// close hides the window; the app keeps running in the tray
	v.w.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		v.w.Hide()
		e.Cancel()
	})
	app.Event.OnApplicationEvent(events.Windows.APMSuspend, func(*application.ApplicationEvent) {
		note(func(r *result) { r.Events = append(r.Events, "APMSuspend") })
	})
	app.Event.OnApplicationEvent(events.Windows.APMResumeAutomatic, func(*application.ApplicationEvent) {
		note(func(r *result) { r.Events = append(r.Events, "APMResumeAutomatic") })
	})

	tray := app.SystemTray.New()
	tray.SetIcon(icons.SystrayLight)
	tray.SetTooltip("Burrow SPIKE-003")
	menu := app.NewMenu()
	menu.Add("Open").OnClick(func(*application.Context) { v.w.Show(); v.w.Focus() })
	menu.Add("Quit").OnClick(func(*application.Context) { app.Quit() })
	tray.SetMenu(menu)
	tray.OnClick(func() { v.w.Show(); v.w.Focus() })
	note(func(r *result) { r.Extra["tray"] = "created with icon, tooltip and menu" })

	startTicker()
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		go run(v, label, out)
	})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

func wailsVersion() string {
	if bi, ok := rdebug.ReadBuildInfo(); ok {
		for _, d := range bi.Deps {
			if d.Path == "github.com/wailsapp/wails/v3" {
				return d.Version
			}
		}
	}
	return "v3 (unknown)"
}
