package settings_test

import (
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/native/kit"
)

// TestShots saves each page of Settings, and the Models dialogs, light and
// dark, as PNGs in the folder JENAB_SETTINGS_SHOTS names, to look at by
// eye (TASK-055). It is skipped without it; keep the folder out of the
// repo.
func TestShots(t *testing.T) {
	dir := os.Getenv("JENAB_SETTINGS_SHOTS")
	if dir == "" {
		t.Skip("JENAB_SETTINGS_SHOTS is not set")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	both(t, func(t *testing.T, x *fixture) {
		theme := filepath.Base(t.Name())
		save := func(name string) { saveShot(t, x.tt, filepath.Join(dir, name+"-"+theme+".png")) }
		for i, p := range []string{"General", "Memory", "Appearance", "Models", "Usage", "Desktop", "Developer"} {
			x.click(t, p)
			save(string(rune('1'+i)) + "-" + strings.ToLower(p))
		}
		x.click(t, "Models")
		x.click(t, "Models of gemini")
		x.settle(nil)
		save("8-provider-models")
		x.tt.Key(0, kit.KeyEscape)
		x.settle(nil)
		x.click(t, "Add provider")
		x.settle(nil)
		save("9-add-provider")
	})
}

// saveShot saves the window as a PNG at path.
func saveShot(t *testing.T, tt *kit.Tester, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, tt.Image()); err != nil {
		t.Fatal(err)
	}
}
