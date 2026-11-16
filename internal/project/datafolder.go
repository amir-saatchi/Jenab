package project

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/amir-saatchi/jenab/internal/store"
)

// DataFolderProblem says why dir can't be the next data folder, or "".
// At the next start the projects move into dir's projects folder, so dir
// must be a full path, and not the projects folder now (projects), a
// folder inside it, or a folder inside a project's folder. projects ""
// skips that check.
func DataFolderProblem(dir, projects string, list []store.ProjectEntry) string {
	if !filepath.IsAbs(dir) {
		return fmt.Sprintf("%q is not a full path. Choose a folder.", dir)
	}
	in := func(parent string) bool { return within(resolve(dir), resolve(parent), caseless(runtime.GOOS)) }
	if projects != "" && in(projects) {
		return "The data folder can't be inside the projects folder, " + projects + ". Choose a folder outside it."
	}
	for _, p := range list {
		if p.Folder != "" && in(p.Folder) {
			return "The data folder can't be inside the folder of the project " + p.Name + ". Choose a folder outside it."
		}
	}
	return ""
}

// within tells whether p is parent or a folder inside it. Both are full
// paths.
func within(p, parent string, caseless bool) bool {
	p, parent = filepath.Clean(p), filepath.Clean(parent)
	if caseless {
		p, parent = strings.ToLower(p), strings.ToLower(parent)
	}
	rel, err := filepath.Rel(parent, p)
	if err != nil || filepath.IsAbs(rel) {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
