// SPIKE-003, Wails v2 app: hide on close, start hidden, single instance,
// asset route, repaint check, and optionally a separate tray (fyne.io/systray).
package main

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"os"
	rdebug "runtime/debug"
	"strings"

	"fyne.io/systray"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type v2win struct{ ctx *context.Context }

func (v v2win) Minimise() { runtime.WindowMinimise(*v.ctx) }
func (v v2win) Restore()  { runtime.WindowUnminimise(*v.ctx) }
func (v v2win) Hide()     { runtime.WindowHide(*v.ctx) }
func (v v2win) Show()     { runtime.WindowShow(*v.ctx) }
func (v v2win) Quit()     { runtime.Quit(*v.ctx) }
func (v v2win) Notify() error {
	if err := runtime.InitializeNotifications(*v.ctx); err != nil {
		return err
	}
	return runtime.SendNotification(*v.ctx, runtime.NotificationOptions{ID: "spike003", Title: "Burrow SPIKE-003", Body: "Test notification from Wails v2"})
}

func main() {
	out, withTray, frameless, alpha := "result-v2.json", false, false, uint8(255)
	for _, a := range os.Args[1:] {
		switch {
		case a == "--tray":
			withTray = true
		case a == "--frameless":
			frameless = true
		case a == "--alpha1": // the Nord-Agent setting: background alpha 1 of 255
			alpha = 1
		case strings.HasSuffix(a, ".json"):
			out = a
		}
	}
	var ctx context.Context
	v := v2win{&ctx}
	app := "Wails " + wailsVersion()
	if frameless {
		app += ", frameless"
	}
	if alpha != 255 {
		app += fmt.Sprintf(", background alpha %d", alpha)
	}
	if withTray {
		app += " + fyne.io/systray v1.12.2"
		// systray runs its own Win32 message loop on its own locked thread
		go systray.Run(func() {
			systray.SetIcon(trayIcon())
			systray.SetTooltip("Burrow SPIKE-003")
			open := systray.AddMenuItem("Open", "")
			quit := systray.AddMenuItem("Quit", "")
			note(func(r *result) { r.Extra["tray"] = "fyne systray onReady called; icon and menu set" })
			go func() {
				for {
					select {
					case <-open.ClickedCh:
						runtime.WindowShow(ctx)
					case <-quit.ClickedCh:
						runtime.Quit(ctx)
					}
				}
			}()
		}, nil)
	}
	sub, _ := fs.Sub(assets, "frontend")
	startTicker()
	err := wails.Run(&options.App{
		Title:             title,
		Width:             800,
		Height:            600,
		Frameless:         frameless,
		BackgroundColour:  &options.RGBA{R: 255, G: 0, B: 255, A: alpha},
		AssetServer:       &assetserver.Options{Assets: sub, Handler: handler()},
		HideWindowOnClose: true,
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "burrow.spike003.v2",
			OnSecondInstanceLaunch: func(d options.SecondInstanceData) {
				runtime.WindowShow(ctx)
				second <- strings.Join(d.Args, " ")
			},
		},
		Windows: &windows.Options{
			OnSuspend: func() { note(func(r *result) { r.Events = append(r.Events, "OnSuspend") }) },
			OnResume:  func() { note(func(r *result) { r.Events = append(r.Events, "OnResume") }) },
		},
		OnStartup: func(c context.Context) {
			ctx = c
			go run(v, app, out)
		},
		OnShutdown: func(context.Context) {
			if withTray {
				systray.Quit()
			}
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}

func wailsVersion() string {
	if bi, ok := rdebug.ReadBuildInfo(); ok {
		for _, d := range bi.Deps {
			if d.Path == "github.com/wailsapp/wails/v2" {
				return d.Version
			}
		}
	}
	return "v2 (unknown)"
}
