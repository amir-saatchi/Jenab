package version

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/appinfo"
)

// root is the repo root, from this package's folder.
var root = filepath.Join("..", "..")

// semver is a semantic version without build metadata, such as 0.1.0 or
// 0.1.0-dev.
var semver = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(-[0-9A-Za-z.-]+)?$`)

// TestFilesMatchVERSION fails when a file that carries the version differs
// from VERSION, the one place it is set.
func TestFilesMatchVERSION(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(root, "VERSION"))
	if err != nil {
		t.Fatal(err)
	}
	want := strings.TrimSpace(string(b))
	if !semver.MatchString(want) {
		t.Fatalf("VERSION is %q, not a semantic version such as 0.1.0 or 0.1.0-dev", want)
	}
	// The native app's build (TASK-059) writes this version into the exe,
	// the installer and the update manifest.
	var mygo struct {
		Version    string `json:"version"`
		Publisher  string `json:"publisher"`
		Identifier string `json:"identifier"`
	}
	readFile(t, "mygo.json", json.Unmarshal, &mygo)
	check(t, "mygo.json version", mygo.Version, want)
	// The app finds its Apps entry by the identifier the installer names
	// it with (TASK-062).
	if mygo.Identifier != appinfo.Identifier {
		t.Errorf("mygo.json identifier is %q; appinfo.Identifier is %q", mygo.Identifier, appinfo.Identifier)
	}
	// The repo is public: the exe and the installer name the app, not a
	// person.
	check(t, "mygo.json publisher", mygo.Publisher, appinfo.Name)
}

func readFile(t *testing.T, name string, unmarshal func([]byte, any) error, v any) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
	if err != nil {
		t.Fatal(err)
	}
	if err := unmarshal(b, v); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
}

func check(t *testing.T, where, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s is %q; want %q, from VERSION", where, got, want)
	}
}

func TestGetWithoutFlags(t *testing.T) {
	i := Get()
	if i.Version != "dev" || i.Commit != "" || i.Modified {
		t.Errorf("Get() = %+v; want version dev and no commit", i)
	}
	if i.Go == "" || i.OS == "" {
		t.Errorf("Get() = %+v; want the Go version and the OS", i)
	}
}

func TestString(t *testing.T) {
	for _, c := range []struct {
		in   Info
		want string
	}{
		{Info{Version: "dev"}, "dev"},
		{Info{Version: "0.1.0-dev", Commit: "d449b7e"}, "0.1.0-dev (d449b7e)"},
		{Info{Version: "0.1.0-dev", Commit: "d449b7e", Modified: true}, "0.1.0-dev (d449b7e, modified)"},
	} {
		if got := c.in.String(); got != c.want {
			t.Errorf("%+v.String() = %q; want %q", c.in, got, c.want)
		}
	}
}
