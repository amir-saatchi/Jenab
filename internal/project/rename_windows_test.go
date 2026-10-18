package project

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLongPath(t *testing.T) {
	long := `C:\` + strings.Repeat(`a\`, 130)
	for in, want := range map[string]string{
		`C:\short\x.zip`:                `C:\short\x.zip`,
		`rel\x`:                         `rel\x`,
		long + `x`:                      `\\?\` + long + `x`,
		long + `b\..\x`:                 `\\?\` + long + `x`, // cleaned first
		`\\srv\share\` + long[3:] + `x`: `\\?\UNC\srv\share\` + long[3:] + `x`,
		`\\?\` + long + `x`:             `\\?\` + long + `x`,
	} {
		if got := longPath(in); got != want {
			t.Errorf("longPath(%.20q…) = %.30q…", in, got)
		}
	}
}

// TestExportLongPath: a target of 250 characters, and so a .partial of
// 285, past MAX_PATH; the final rename and the folder's rmdir go through
// longPath.
func TestExportLongPath(t *testing.T) {
	e := newEnv(t, t.TempDir())
	defer e.stop(t)
	p, _ := seedProject(t, e, 2, 100)
	defer p.Release()
	for _, f := range []ExportFormat{ExportZip, ExportFolder} {
		dir := t.TempDir()
		for len(dir) < 200 {
			dir = filepath.Join(dir, strings.Repeat("d", 40))
		}
		name := strings.Repeat("n", 250-len(dir)-1-4) + ".zip"
		target := filepath.Join(dir, name)
		if len(target) != 250 {
			t.Fatalf("the target has %d characters", len(target))
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if f == ExportFolder {
			if err := os.Mkdir(target, 0o755); err != nil { // an empty folder, removed by rmdir
				t.Fatal(err)
			}
		}
		res, err := p.Export(context.Background(), ExportOptions{Target: target, Format: f})
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		other := t.TempDir()
		place(t, res, testPaths(other).Projects)
		checkCopy(t, other, res.ID, "صادرات BTC", nil)
		if left := partials(target); len(left) != 0 {
			t.Errorf("%s: left %v", f, left)
		}
		// Taken now: at this length too, the rename refuses it and rmdir
		// leaves a folder that isn't empty.
		src := target + ".other"
		if err := userFile(src); err != nil {
			t.Fatal(err)
		}
		if err := renameNoReplace(src, target); !errors.Is(err, fs.ErrExist) {
			t.Errorf("%s: rename onto the export: %v", f, err)
		}
		if f == ExportFolder {
			if err := rmdir(target); err == nil || !exists(target) {
				t.Errorf("%s: rmdir of the export: %v", f, err)
			}
		}
	}
}
