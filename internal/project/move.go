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

// rename is os.Rename, swapped in tests to take the copy path.
var rename = os.Rename

// MoveProjects moves the projects to the data folder in paths when it
// changed since the last start (SPEC 2.1). It runs at start, before any
// project opens. Only the projects move; the settings, registry and logs
// stay in paths.Root.
//
// A project that can't be moved stays where it is and still opens from
// there, and the next start tries again. MoveProjects returns nil when the
// data folder didn't change.
func MoveProjects(ctx context.Context, paths config.Paths, reg *store.Registry, log *slog.Logger) (*MoveResult, error) {
	last, err := os.ReadFile(paths.LastData)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	from := strings.TrimSpace(string(last))
	if from == "" || samePath(from, paths.DataFolder) {
		return nil, saveLast(paths, from)
	}
	r := &MoveResult{From: from, To: paths.DataFolder, Failed: []string{}}
	old := filepath.Join(from, "projects")
	ents, err := os.ReadDir(old)
	if errors.Is(err, os.ErrNotExist) {
		return r, saveLast(paths, from)
	}
	if err != nil {
		return nil, fmt.Errorf("project: reading the old data folder: %w", err)
	}
	known := map[id.Project]store.ProjectEntry{}
	ps, err := reg.Projects(ctx)
	if err != nil {
		return nil, err
	}
	for _, p := range ps {
		known[p.ID] = p
	}
	if err := os.MkdirAll(paths.Projects, 0o755); err != nil {
		return nil, fmt.Errorf("project: making the new data folder: %w", err)
	}
	for _, e := range ents {
		if !e.IsDir() || !id.Valid(e.Name()) {
			continue
		}
		pid := id.Project(e.Name())
		src, dst := filepath.Join(old, e.Name()), filepath.Join(paths.Projects, e.Name())
		p, registered := known[pid]
		name := e.Name()
		if registered {
			name = p.Name
		}
		if err := moveProject(ctx, reg, p, registered, src, dst, log); err != nil {
			log.Warn("project: not moved to the new data folder", "project", pid, "err", err)
			r.Failed = append(r.Failed, name)
			continue
		}
		log.Info("project: moved to the new data folder", "project", pid, "folder", dst)
		r.Moved++
	}
	if len(r.Failed) > 0 {
		return r, nil
	}
	return r, saveLast(paths, from)
}

// moveProject moves one project folder and points its registry entry at
// the new place. If the registry can't be updated, the folder goes back.
func moveProject(ctx context.Context, reg *store.Registry, p store.ProjectEntry, registered bool, src, dst string, log *slog.Logger) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := moveDir(src, dst, log); err != nil {
		return err
	}
	if !registered || !samePath(p.Folder, src) {
		return nil // List adds it from its folder
	}
	p.Folder = dst
	if err := reg.SaveProject(ctx, p); err != nil {
		if back := moveDir(dst, src, log); back != nil {
			log.Error("project: moved, but neither the registry nor the folder could be put back", "project", p.ID, "err", back)
		}
		return err
	}
	return nil
}

// moveDir renames src to dst, or copies it and removes src when a rename
// can't do it, e.g. to another drive.
func moveDir(src, dst string, log *slog.Logger) error {
	if _, err := os.Lstat(dst); err == nil {
		return fmt.Errorf("%s already exists", dst)
	}
	if err := rename(src, dst); err == nil {
		return nil
	}
	tmp := dst + ".moving" // not a project ID, so List skips it
	if err := os.RemoveAll(tmp); err != nil {
		return err
	}
	if err := copyDir(src, tmp); err != nil {
		os.RemoveAll(tmp)
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.RemoveAll(tmp)
		return err
	}
	if err := os.RemoveAll(src); err != nil {
		log.Warn("project: copied to the new data folder, but the old folder could not be removed", "folder", src, "err", err)
	}
	return nil
}

// copyDir copies the folder src to dst, which must not exist.
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
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return errors.Join(out.Sync(), out.Close())
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

// samePath tells whether a and b name the same folder, ignoring case where
// the OS usually does.
func samePath(a, b string) bool {
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
