package version

import (
	"encoding/json"
	"regexp"
	"runtime/debug"
	"testing"

	"github.com/amir-saatchi/jenab"
	"github.com/amir-saatchi/jenab/internal/appinfo"
)

// semver is a semantic version without build metadata, such as 0.1.0 or
// 0.1.0-dev.
var semver = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(-[0-9A-Za-z.-]+)?$`)

// TestMyGoJSON checks mygo.json, the one place the version is set. The
// release build (TASK-059) writes its version into the exe, the installer
// and the update manifest.
func TestMyGoJSON(t *testing.T) {
	var c struct {
		Version    string `json:"version"`
		Publisher  string `json:"publisher"`
		Identifier string `json:"identifier"`
	}
	if err := json.Unmarshal(jenab.MyGoJSON, &c); err != nil {
		t.Fatalf("mygo.json: %v", err)
	}
	if !semver.MatchString(c.Version) {
		t.Errorf("mygo.json's version is %q, not a semantic version such as 0.1.0 or 0.1.0-dev", c.Version)
	}
	// The app finds its Apps entry by the identifier the installer names
	// it with (TASK-062).
	if c.Identifier != appinfo.Identifier {
		t.Errorf("mygo.json's identifier is %q; appinfo.Identifier is %q", c.Identifier, appinfo.Identifier)
	}
	// The repo is public: the exe and the installer name the app, not a
	// person.
	if c.Publisher != appinfo.Name {
		t.Errorf("mygo.json's publisher is %q; want the app's name, %q", c.Publisher, appinfo.Name)
	}
	if got := Get().Version; got != c.Version {
		t.Errorf("Get().Version = %q; want mygo.json's %q", got, c.Version)
	}
}

func TestGetInATest(t *testing.T) {
	// A test binary has no VCS stamp, and the tests set no -X flags.
	i := Get()
	if i.Commit != "" || i.Modified {
		t.Errorf("Get() = %+v; want no commit", i)
	}
	if i.Go == "" || i.OS == "" {
		t.Errorf("Get() = %+v; want the Go version and the OS", i)
	}
}

func TestConfigVersion(t *testing.T) {
	for in, want := range map[string]string{
		`{"version": "0.2.0"}`: "0.2.0",
		`{"name": "Jenab"}`:    "dev",
		`not json`:             "dev",
	} {
		if got := configVersion([]byte(in)); got != want {
			t.Errorf("configVersion(%s) = %q; want %q", in, got, want)
		}
	}
}

func TestVCS(t *testing.T) {
	for _, c := range []struct {
		in       []debug.BuildSetting
		commit   string
		modified bool
	}{
		{nil, "", false},
		{[]debug.BuildSetting{{Key: "vcs", Value: "git"}, {Key: "vcs.revision", Value: "d449b7e1f2a3b4c5d6e7f8091a2b3c4d5e6f7a8b"}, {Key: "vcs.modified", Value: "false"}}, "d449b7e", false},
		{[]debug.BuildSetting{{Key: "vcs.revision", Value: "d449b7e1f2a3"}, {Key: "vcs.modified", Value: "true"}}, "d449b7e", true},
		{[]debug.BuildSetting{{Key: "vcs.revision", Value: "abc"}}, "abc", false},
	} {
		commit, modified := vcs(c.in)
		if commit != c.commit || modified != c.modified {
			t.Errorf("vcs(%v) = %q, %v; want %q, %v", c.in, commit, modified, c.commit, c.modified)
		}
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
