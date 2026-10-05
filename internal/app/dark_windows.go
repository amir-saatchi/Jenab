package app

import "github.com/wailsapp/wails/v3/pkg/w32"

// systemDark tells whether Windows uses dark mode for apps. It works
// before Run, unlike Wails' Env.IsDarkMode.
func systemDark() bool { return w32.IsCurrentlyDarkMode() }
