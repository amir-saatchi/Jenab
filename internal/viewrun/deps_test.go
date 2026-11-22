package viewrun

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const module = "github.com/amir-saatchi/jenab/"

// The runtime can't call a model or open a connection (R-48): nothing it
// builds on, in this module, imports sockets, TLS, a provider, the agent,
// the web server or the UI; viewrun itself imports nothing of net. (The
// bucket uses net/http only to sniff content types, and store and sqlworker
// net/url to build SQLite connection strings and read links.)
func TestNoNetworkImports(t *testing.T) {
	anywhere := []string{"net", "crypto/tls",
		module + "internal/provider", module + "internal/agent", module + "internal/web", module + "internal/native"}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	var walk func(pkg string, via []string)
	walk = func(pkg string, via []string) {
		if seen[pkg] {
			return
		}
		seen[pkg] = true
		dir := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(pkg, module)))
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			n := e.Name()
			if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
				continue
			}
			f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, n), nil, parser.ImportsOnly)
			if err != nil {
				t.Fatal(err)
			}
			for _, im := range f.Imports {
				p, _ := strconv.Unquote(im.Path.Value)
				if len(via) == 0 && strings.HasPrefix(p, "net/") {
					t.Errorf("viewrun imports %s", p)
				}
				for _, bad := range anywhere {
					if p == bad || (bad != "net" && strings.HasPrefix(p, bad+"/")) {
						t.Errorf("%s imports %s (via %s)", pkg, p, strings.Join(append(via, pkg), " → "))
					}
				}
				if strings.HasPrefix(p, module) {
					walk(p, append(via, pkg))
				}
			}
		}
	}
	walk(module+"internal/viewrun", nil)
	if len(seen) < 3 {
		t.Fatalf("walked only %d packages", len(seen))
	}
}
