package project

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/store"
)

// A new data folder must be a full path outside the projects folder and
// every project's folder, since the projects move into it (TASK-068).
func TestDataFolderProblem(t *testing.T) {
	root := t.TempDir()
	projects := filepath.Join(root, "Jenab", "projects")
	away := filepath.Join(root, "Elsewhere", "Coins")
	list := []store.ProjectEntry{{Name: "Coins", Folder: away}}
	for _, c := range []struct {
		dir, want string
	}{
		{"Jenab", "is not a full path"},
		{projects, "inside the projects folder"},
		{filepath.Join(projects, "p1", "data"), "inside the projects folder"},
		{away, "inside the folder of the project Coins"},
		{filepath.Join(away, "sub"), "inside the folder of the project Coins"},
		{filepath.Join(root, "Jenab"), ""},
		{filepath.Join(root, "Jenab", "projects2"), ""},
		{filepath.Join(root, "Elsewhere"), ""},
		{filepath.Join(root, "New"), ""},
	} {
		got := DataFolderProblem(c.dir, projects, list)
		if (c.want == "") != (got == "") || !strings.Contains(got, c.want) {
			t.Errorf("DataFolderProblem(%q) = %q, want %q", c.dir, got, c.want)
		}
	}
	if got := DataFolderProblem(filepath.Join(projects, "x"), "", nil); got != "" {
		t.Errorf("with no projects folder = %q", got)
	}
}

func TestWithin(t *testing.T) {
	sep := string(filepath.Separator)
	base := filepath.Join(sep+"a", "b")
	for _, c := range []struct {
		p, parent string
		caseless  bool
		want      bool
	}{
		{base, base, false, true},
		{filepath.Join(base, "c"), base, false, true},
		{filepath.Join(sep+"a", "bc"), base, false, false},
		{filepath.Join(sep + "a"), base, false, false},
		{filepath.Join(sep+"A", "B", "c"), base, true, true},
		{filepath.Join(base, "..b"), base, false, true},
	} {
		if got := within(c.p, c.parent, c.caseless); got != c.want {
			t.Errorf("within(%q, %q, %v) = %v", c.p, c.parent, c.caseless, got)
		}
	}
}
