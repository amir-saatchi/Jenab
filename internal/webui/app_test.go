package webui

import (
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestWindowTheme(t *testing.T) {
	for _, c := range []struct {
		theme string
		dark  bool
		bg    application.RGBA
		frame application.Theme
	}{
		{"light", true, lightBackground, application.Light},
		{"dark", false, darkBackground, application.Dark},
		{"system", true, darkBackground, application.SystemDefault},
		{"system", false, lightBackground, application.SystemDefault},
	} {
		if bg, frame := windowTheme(c.theme, c.dark); bg != c.bg || frame != c.frame {
			t.Errorf("windowTheme(%s, %t) = %v, %v", c.theme, c.dark, bg, frame)
		}
	}
}
