package project

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/amir-saatchi/jenab/internal/store"
)

// writeLock creates jenab.lock, which a clean close removes. Its text is
// only for people reading the folder.
func writeLock(dir string) error {
	f, err := os.Create(lockPath(dir))
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(f, "Jenab has this project open.\npid %d\nopened %s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339))
	if err == nil {
		err = f.Sync()
	}
	return errors.Join(err, f.Close())
}

// recoverProject runs the 2.7 steps after a crash, before the project is
// used:
//  1. quick_check on both databases; a failure returns the damage, and the
//     project opens read-only;
//  2. runs still `running` become `interrupted` (Phase 4, when runs exist);
//  3. tmp/ is emptied, and partial snapshots are deleted;
//  4. the objects/ sweep comes with the bucket (P1-05).
//
// SQLite needs no help: it discards an interrupted transaction itself.
func recoverProject(ctx context.Context, dir string) (*Damage, error) {
	var damage *Damage
	for _, f := range []string{"project.db", "chats.db"} {
		problem, err := store.QuickCheck(ctx, filepath.Join(dir, f))
		if err != nil {
			return nil, err
		}
		if problem != "" && damage == nil {
			damage = &Damage{File: f, Problem: problem, Snapshot: latestSnapshot(dir, f)}
		}
	}
	if err := emptyDir(filepath.Join(dir, "tmp")); err != nil {
		return nil, err
	}
	if err := store.RemovePartials(filepath.Join(dir, "snapshots")); err != nil {
		return nil, err
	}
	return damage, nil
}

// emptyDir removes everything inside dir and keeps dir itself.
func emptyDir(dir string) error {
	ents, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return os.MkdirAll(dir, 0o755)
	}
	if err != nil {
		return err
	}
	for _, e := range ents {
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

// latestSnapshot is the newest complete snapshot of file (project.db or
// chats.db) in snapshots/: a .db file whose name starts with the file's
// name and a dash. "" if there is none.
func latestSnapshot(dir, file string) string {
	prefix := strings.TrimSuffix(file, ".db") + "-"
	ents, err := os.ReadDir(filepath.Join(dir, "snapshots"))
	if err != nil {
		return ""
	}
	var best string
	var bestT time.Time
	for _, e := range ents {
		n := e.Name()
		if e.IsDir() || !strings.HasPrefix(n, prefix) || !strings.HasSuffix(n, ".db") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if best == "" || info.ModTime().After(bestT) {
			best, bestT = filepath.Join(dir, "snapshots", n), info.ModTime()
		}
	}
	return best
}
