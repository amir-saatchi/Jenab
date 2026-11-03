package version

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
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
	m := semver.FindStringSubmatch(want)
	if m == nil {
		t.Fatalf("VERSION is %q, not a semantic version such as 0.1.0 or 0.1.0-dev", want)
	}
	// Windows' fixed file and product versions are four numbers.
	four := m[1] + "." + m[2] + "." + m[3] + ".0"

	var cfg struct {
		Info struct {
			Version string `yaml:"version"`
		} `yaml:"info"`
	}
	readFile(t, "build/config.yml", yaml.Unmarshal, &cfg)
	check(t, "build/config.yml info.version", cfg.Info.Version, want)

	var info struct {
		Fixed struct {
			FileVersion    string `json:"file_version"`
			ProductVersion string `json:"product_version"`
		} `json:"fixed"`
		Info map[string]map[string]string `json:"info"`
	}
	readFile(t, "build/windows/info.json", json.Unmarshal, &info)
	check(t, "build/windows/info.json fixed.file_version", info.Fixed.FileVersion, four)
	check(t, "build/windows/info.json fixed.product_version", info.Fixed.ProductVersion, four)
	if len(info.Info) == 0 {
		t.Error("build/windows/info.json has no info table")
	}
	for lang, table := range info.Info {
		check(t, "build/windows/info.json info."+lang+".FileVersion", table["FileVersion"], want)
		check(t, "build/windows/info.json info."+lang+".ProductVersion", table["ProductVersion"], want)
	}

	var pkg struct {
		Version string `json:"version"`
	}
	readFile(t, "frontend/package.json", json.Unmarshal, &pkg)
	check(t, "frontend/package.json version", pkg.Version, want)
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
