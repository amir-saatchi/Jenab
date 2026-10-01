// Command desktop is the Jenab app: it builds everything and starts Wails.
package main

import (
	"fmt"
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/amir-saatchi/jenab/frontend"
)

// appName is the one place the app's name is set (Q1).
const appName = "Jenab"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run builds the app and blocks until it quits. For now it only opens the
// main window; the packages in CODE-OUTLINE section 2 are added ticket by ticket.
func run(_ []string) error {
	assets, err := frontend.Assets()
	if err != nil {
		return err
	}
	app := application.New(application.Options{
		Name:   appName,
		Assets: application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
	})
	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     appName,
		Width:     1280,
		Height:    800,
		MinWidth:  640,
		MinHeight: 480,
	})
	return app.Run()
}
