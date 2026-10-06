package project

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/store"
)

// MoveResult is what MoveProjects did.
type MoveResult struct {
	From   string   `json:"from"`
	To     string   `json:"to"`
	Moved  int      `json:"moved"`
	Failed []string `json:"failed"` // the projects left in the old folder, by name
}

// ErrNested is a new data folder inside the old one, or the old one inside
// the new: the projects can't move into or out of themselves.
var ErrNested = errors.New("project: the new data folder and the old one are inside each other")

// rename, removeAll and saveProject are swapped in tests to fail.
var (
	rename      = os.Rename
	removeAll   = os.RemoveAll
	saveProject = (*store.Registry).SaveProject
)

// errGone is a project folder that isn't there any more, so there is
// nothing to move.
var errGone = errors.New("the folder is gone")

// MoveProjects moves the projects to the data folder in paths when it
// changed (SPEC 2.1). It runs at start, before any project opens. Only the
// projects move; the settings, registry and logs stay in paths.Root.
//
// Every project lives in <data folder>/projects/<id>, so a registered one
// in another projects folder is from an earlier data folder and moves, as
// does every project folder in the data folder of the last start.
//
// A project that can't be moved stays where it is and still opens from
// there, and the next start tries again. MoveProjects returns nil when
// there was nothing to move and the data folder didn't change.
func MoveProjects(ctx context.Context, paths config.Paths, reg *store.Registry, log *slog.Logger) (*MoveResult, error) {
	last, err := os.ReadFile(paths.LastData)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	from := strings.TrimSpace(string(last))
	changed := from != "" && !samePath(from, paths.DataFolder)
	moves, err := pendingMoves(ctx, paths, from, changed, reg, log)
	if err != nil {
		return nil, err
	}
	if len(moves) == 0 {
		if !changed {
			return nil, saveLast(paths, from)
		}
		return &MoveResult{From: from, To: paths.DataFolder, Failed: []string{}}, saveLast(paths, from)
	}
	r := &MoveResult{From: from, To: paths.DataFolder, Failed: []string{}}
	if !changed {
		r.From = filepath.Dir(filepath.Dir(moves[0].src))
	}
	for _, mv := range moves {
		if old := filepath.Dir(mv.src); nested(old, paths.Projects) {
			for _, mv := range moves {
				r.Failed = append(r.Failed, mv.name)
			}
			return r, fmt.Errorf("%w: %s and %s", ErrNested, old, paths.Projects)
		}
	}
	if err := os.MkdirAll(paths.Projects, 0o755); err != nil {
		return nil, fmt.Errorf("project: making the new data folder: %w", err)
	}
	for _, mv := range moves {
		dst := filepath.Join(paths.Projects, string(mv.pid))
		err := moveProject(ctx, reg, mv, dst, log)
		if errors.Is(err, errGone) {
			log.Warn("project: not moved, its folder is gone", "project", mv.pid, "folder", mv.src)
			continue
		}
		if err != nil {
			log.Warn("project: not moved to the new data folder", "project", mv.pid, "err", err)
			r.Failed = append(r.Failed, mv.name)
			continue
		}
		log.Info("project: moved to the new data folder", "project", mv.pid, "folder", dst)
		r.Moved++
	}
	if len(r.Failed) > 0 {
		return r, nil
	}
	return r, saveLast(paths, from)
}

// pendingMove is a project folder to move.
type pendingMove struct {
	pid   id.Project
	name  string
	src   string
	entry *store.ProjectEntry // nil when the registry doesn't know it
}

// pendingMoves lists the projects to move: the registered ones in another
// data folder's projects folder, and the folders in the projects folder of
// the last start's data folder, from, when the data folder changed.
func pendingMoves(ctx context.Context, paths config.Paths, from string, changed bool, reg *store.Registry, log *slog.Logger) ([]pendingMove, error) {
	ps, err := reg.Projects(ctx)
	if err != nil {
		return nil, err
	}
	known := map[id.Project]store.ProjectEntry{}
	var out []pendingMove
	for _, p := range ps {
		known[p.ID] = p
		dir := filepath.Dir(p.Folder)
		if filepath.Base(p.Folder) != string(p.ID) || !strings.EqualFold(filepath.Base(dir), "projects") || samePath(dir, paths.Projects) {
			continue
		}
		out = append(out, pendingMove{pid: p.ID, name: p.Name, src: p.Folder, entry: &p})
	}
	if !changed {
		return out, nil
	}
	old := filepath.Join(from, "projects")
	ents, err := os.ReadDir(old)
	if errors.Is(err, os.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return nil, fmt.Errorf("project: reading the old data folder: %w", err)
	}
	for _, e := range ents {
		if !e.IsDir() || !id.Valid(e.Name()) {
			continue
		}
		pid := id.Project(e.Name())
		src := filepath.Join(old, e.Name())
		p, ok := known[pid]
		switch {
		case !ok:
			out = append(out, pendingMove{pid: pid, name: e.Name(), src: src})
		case samePath(p.Folder, src):
			// listed above
		case hasProject(ctx, p.Folder, pid):
			// A copy that stopped before it removed the old folder.
			if err := removeAll(src); err != nil {
				log.Warn("project: the old copy of a moved project could not be removed", "folder", src, "err", err)
			}
		default:
			// The registry points at a folder that is gone.
			out = append(out, pendingMove{pid: pid, name: p.Name, src: src, entry: &p})
		}
	}
	return out, nil
}

// moveProject moves one project folder to dst and points its registry
// entry there. The registry is updated before the old folder is removed,
// so a crash or failure leaves it pointing at a complete project: the old
// folder, or the new one, which the next start or open finds again.
func moveProject(ctx context.Context, reg *store.Registry, mv pendingMove, dst string, log *slog.Logger) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := os.Lstat(mv.src); errors.Is(err, os.ErrNotExist) {
		// A rename that stopped before the registry was updated.
		if mv.entry != nil && hasProject(ctx, dst, mv.pid) {
			return point(ctx, reg, mv.entry, dst)
		}
		return errGone
	}
	if _, err := os.Lstat(dst); err == nil {
		// A copy that stopped before the registry was updated, if dst
		// holds the same files.
		if !sameTree(mv.src, dst) {
			return fmt.Errorf("%s already exists", dst)
		}
		return finishCopy(ctx, reg, mv, dst, log)
	}
	err := rename(mv.src, dst)
	switch {
	case err == nil:
		if err := point(ctx, reg, mv.entry, dst); err != nil {
			if back := rename(dst, mv.src); back != nil {
				log.Error("project: moved, but neither the registry nor the folder could be put back; the next start fixes the registry", "project", mv.pid, "err", back)
			}
			return err
		}
		return nil
	case !crossDevice(err):
		// e.g. a file still open: copying it could lose writes.
		return err
	}
	tmp := dst + ".moving" // not a project ID, so List skips it
	if err := removeAll(tmp); err != nil {
		return err
	}
	if err := copyDir(mv.src, tmp); err != nil {
		removeAll(tmp)
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		removeAll(tmp)
		return err
	}
	return finishCopy(ctx, reg, mv, dst, log)
}

// finishCopy points the registry at the copy in dst, then removes the old
// folder. If the registry can't be updated, the copy goes.
func finishCopy(ctx context.Context, reg *store.Registry, mv pendingMove, dst string, log *slog.Logger) error {
	if err := point(ctx, reg, mv.entry, dst); err != nil {
		if rm := removeAll(dst); rm != nil {
			log.Warn("project: the copy in the new data folder could not be removed", "folder", dst, "err", rm)
		}
		return err
	}
	if err := removeAll(mv.src); err != nil {
		log.Warn("project: copied to the new data folder, but the old folder could not be removed", "folder", mv.src, "err", err)
	}
	return nil
}

// point sets a registered project's folder. An unregistered one is added
// by List from its folder.
func point(ctx context.Context, reg *store.Registry, e *store.ProjectEntry, dir string) error {
	if e == nil {
		return nil
	}
	p := *e
	p.Folder = dir
	return saveProject(reg, ctx, p)
}

// hasProject tells whether dir holds project pid.
func hasProject(ctx context.Context, dir string, pid id.Project) bool {
	if _, err := os.Stat(filepath.Join(dir, "project.db")); err != nil {
		return false
	}
	meta, err := store.ReadProjectMeta(ctx, dir)
	return err == nil && meta[metaID] == string(pid)
}

// copyDir copies the folder src to dst, which must not exist. Files keep
// their modification times, so sameTree can tell a finished copy.
func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		to := filepath.Join(dst, rel)
		switch {
		case d.IsDir():
			return os.MkdirAll(to, 0o755)
		case d.Type().IsRegular():
			return copyFile(path, to)
		default:
			return fmt.Errorf("%s is not a file or folder", path)
		}
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	st, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := errors.Join(out.Sync(), out.Close()); err != nil {
		return err
	}
	return os.Chtimes(dst, st.ModTime(), st.ModTime())
}

// sameTree tells whether folders a and b hold the same files, by size and
// modification time.
func sameTree(a, b string) bool {
	type file struct {
		dir  bool
		size int64
		mod  int64
	}
	list := func(root string) (map[string]file, error) {
		out := map[string]file{}
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			if d.IsDir() {
				out[rel] = file{dir: true}
				return nil
			}
			st, err := d.Info()
			if err != nil {
				return err
			}
			out[rel] = file{size: st.Size(), mod: st.ModTime().UnixNano()}
			return nil
		})
		return out, err
	}
	fa, err := list(a)
	if err != nil {
		return false
	}
	fb, err := list(b)
	if err != nil || len(fa) != len(fb) {
		return false
	}
	for k, v := range fa {
		if w, ok := fb[k]; !ok || w != v {
			return false
		}
	}
	return true
}

// saveLast records the data folder in use, if it isn't recorded yet.
func saveLast(paths config.Paths, last string) error {
	if last != "" && samePath(last, paths.DataFolder) {
		return nil
	}
	dir, err := filepath.Abs(paths.DataFolder)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(paths.LastData), 0o755); err != nil {
		return err
	}
	return os.WriteFile(paths.LastData, []byte(dir+"\n"), 0o644)
}

// samePath tells whether a and b name the same folder. When both exist the
// file system decides, so junctions, symlinks, subst drives and short names
// match; otherwise the paths are compared, ignoring case where the OS
// usually does.
func samePath(a, b string) bool {
	if sa, err := os.Stat(a); err == nil {
		if sb, err := os.Stat(b); err == nil {
			return os.SameFile(sa, sb)
		}
	}
	a, b = filepath.Clean(a), filepath.Clean(b)
	if aa, err := filepath.Abs(a); err == nil {
		a = aa
	}
	if bb, err := filepath.Abs(b); err == nil {
		b = bb
	}
	if caseless(runtime.GOOS) {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// nested tells whether folder a is inside b or b inside a.
func nested(a, b string) bool {
	return inside(a, b) || inside(b, a)
}

// inside tells whether folder a is somewhere below folder b.
func inside(a, b string) bool {
	a, err := filepath.Abs(a)
	if err != nil {
		return false
	}
	for d := filepath.Dir(a); d != a; a, d = d, filepath.Dir(d) {
		if samePath(d, b) {
			return true
		}
	}
	return false
}
