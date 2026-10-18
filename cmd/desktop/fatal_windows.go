package main

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"

	"github.com/amir-saatchi/jenab/internal/appinfo"
)

// fatal tells the user why the app can't start. The release build has no
// console (-H windowsgui), so it is a message box; stderr gets it too, for
// a dev build.
func fatal(msg string) {
	fmt.Fprintln(os.Stderr, msg)
	text, err1 := windows.UTF16PtrFromString(msg)
	title, err2 := windows.UTF16PtrFromString(appinfo.Name)
	if err1 != nil || err2 != nil {
		return
	}
	_, _ = windows.MessageBox(0, text, title, windows.MB_OK|windows.MB_ICONERROR|windows.MB_SETFOREGROUND)
}
