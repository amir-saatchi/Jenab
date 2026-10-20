package project

import (
	"archive/zip"
	"bufio"
	"compress/flate"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/amir-saatchi/jenab/internal/bucket"
	"github.com/amir-saatchi/jenab/internal/disk"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/store"
)

// ExportFormat is what an export writes.
type ExportFormat string

const (
	ExportZip    ExportFormat = "zip"    // one zip file
	ExportFolder ExportFormat = "folder" // a folder
)

// ExportStep is what an export is copying.
type ExportStep string

const (
	StepProject ExportStep = "project.db"
	StepChats   ExportStep = "chats.db"
	StepObjects ExportStep = "objects"
	StepSkills  ExportStep = "skills"
	StepDone    ExportStep = "done"
	StepFailed  ExportStep = "failed" // sent by the app when an export fails or is stopped
)

var (
	ErrExportRunning = errors.New("project: an export of this project is already running")
	ErrExportExists  = errors.New("project: the export's place is taken")
	ErrNoSpace       = errors.New("project: not enough free disk space for the export")
	ErrExportInside  = errors.New("project: an export can't go inside the project's own folder")
)

// Meta keys an export writes into the copy's _jenab_meta.
const (
	metaExportedFrom = "exported_from"
	metaExportedAt   = "exported_at"
)

// spaceMargin is kept free on top of what an export needs.
const spaceMargin = 16 << 20

// freeSpace is disk.Free; tests swap it.
var freeSpace = disk.Free

// progressEvery is how often an export reports its progress, at most.
const progressEvery = 100 * time.Millisecond

// ExportOptions say where and how to export.
type ExportOptions struct {
	// Target is the zip file or the folder to write. Inside it is one
	// folder named by the copy's project ID, so it can be copied into the
	// projects folder as it is.
	Target string
	Format ExportFormat
	// Replace lets a zip export replace a file at Target, when the save
	// dialog asked the user. A folder is never replaced; an empty one is
	// used.
	Replace bool
	// IncludeApprovals keeps the project's trust in the copy: its
	// approvals, approval level and private-network hosts. Without it the
	// copy has no approvals or hosts, and the level a new project gets.
	IncludeApprovals bool
	// Progress gets the progress at each step and at most every
	// progressEvery between; nil gets none. It must not block.
	Progress func(ExportProgress)
}

// ExportProgress is how far an export is.
type ExportProgress struct {
	Project id.Project `json:"project"`
	Target  string     `json:"target"`
	Step    ExportStep `json:"step"`
	Done    int64      `json:"done"`  // bytes
	Total   int64      `json:"total"` // bytes; an estimate until project.db is copied
	// The app's last event of an export carries how it ended: Result with
	// StepDone, or Error with StepFailed (Stopped when it was stopped).
	Result  *ExportResult `json:"result,omitempty"`
	Error   string        `json:"error,omitempty"`
	Stopped bool          `json:"stopped,omitempty"`
}

// ExportResult is a finished export.
type ExportResult struct {
	Path   string       `json:"path"`
	Format ExportFormat `json:"format"`
	// ID is the copy's project ID: a new one, so the copy can sit next to
	// the project in the same data folder.
	ID      id.Project `json:"id"`
	Size    int64      `json:"size"` // the zip file, or the folder's files, in bytes
	Objects int        `json:"objects"`
	// Missing counts the objects the index points to whose bytes are not
	// in objects/. They are left out; the export is otherwise complete.
	Missing int   `json:"missing"`
	Millis  int64 `json:"ms"`
}

// ExportEstimate is what an export of the project would copy now, for the
// export dialog.
type ExportEstimate struct {
	Bytes     int64 `json:"bytes"`     // databases (with their WAL), objects/ and skills/
	Databases int64 `json:"databases"` // project.db and chats.db with their WAL
	Objects   int   `json:"objects"`   // files in objects/
}

// Estimate adds up what an export would copy now.
func (p *Project) Estimate(ctx context.Context) (ExportEstimate, error) {
	var e ExportEstimate
	for _, f := range []string{"project.db", "project.db-wal", "chats.db", "chats.db-wal"} {
		if st, err := os.Stat(filepath.Join(p.Dir, f)); err == nil {
			e.Databases += st.Size()
		}
	}
	e.Bytes = e.Databases
	for _, sub := range []string{"objects", "skills"} {
		err := filepath.WalkDir(filepath.Join(p.Dir, sub), func(_ string, d fs.DirEntry, err error) error {
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					return nil
				}
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if !d.Type().IsRegular() {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil // removed meanwhile
			}
			e.Bytes += info.Size()
			if sub == "objects" {
				e.Objects++
			}
			return nil
		})
		if err != nil {
			return e, err
		}
	}
	return e, nil
}

// Export copies the project to o.Target while it stays in use (SPEC 2.1).
// Each database is copied as of one moment with VACUUM INTO and the
// safe-copy rule (7.5): project.db first, then chats.db, so every change
// in the copy finds the message it points to (2.3). Then come the objects
// the copied project.db points to, and skills/. The copy gets a new
// project ID; snapshots/, tmp/ and the lock file are not copied.
//
// Everything is written under o.Target + "." + <new ID> + ".partial" and
// renamed to o.Target at the end, so a failed, cancelled or killed export leaves
// nothing under the final name. A zip export first copies the databases
// into the project's tmp/, which recovery empties after a crash.
func (p *Project) Export(ctx context.Context, o ExportOptions) (ExportResult, error) {
	if o.Format != ExportZip && o.Format != ExportFolder {
		return ExportResult{}, fmt.Errorf("project: unknown export format %q", o.Format)
	}
	if o.Target == "" {
		return ExportResult{}, errors.New("project: the export has no place")
	}
	target, err := filepath.Abs(o.Target)
	if err != nil {
		return ExportResult{}, err
	}
	if samePath(target, p.Dir) || inside(target, p.Dir) {
		return ExportResult{}, ErrExportInside
	}
	// One check under p.mu: close cancels p.ctx before waitExport reads
	// exportDone, so an export either is waited for or doesn't start.
	p.mu.Lock()
	switch {
	case p.ctx.Err() != nil:
		p.mu.Unlock()
		return ExportResult{}, ErrClosed
	case p.exportDone != nil:
		p.mu.Unlock()
		return ExportResult{}, ErrExportRunning
	}
	done := make(chan struct{})
	p.exportDone = done
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		p.exportDone = nil
		p.mu.Unlock()
		close(done)
	}()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer context.AfterFunc(p.ctx, cancel)() // the project closing stops it too

	if err := checkTarget(target, o); err != nil {
		return ExportResult{}, err
	}
	est, err := p.Estimate(ctx)
	if err != nil {
		return ExportResult{}, err
	}
	if err := checkSpace(target, p.Dir, o.Format, est); err != nil {
		return ExportResult{}, err
	}

	r := &exportRun{p: p, o: o, target: target, start: time.Now(), newID: id.Project(id.New())}
	// The new ID makes the name unique: an export never takes over a
	// .partial it didn't make, such as one another export is writing.
	partial := target + "." + string(r.newID) + ".partial"
	// The project's tmp/ keeps where it is, so that the next open removes
	// it if this process dies before the export ends.
	rec := filepath.Join(p.Dir, "tmp", "export-"+string(r.newID)+recordExt)
	if err := writeRecord(rec, partial); err != nil {
		return ExportResult{}, err
	}
	defer os.Remove(rec)
	r.total = est.Bytes
	if o.Format == ExportZip {
		r.total += est.Databases // copied to tmp/, then into the zip
	}
	defer p.Report(r)()
	var size int64
	if o.Format == ExportZip {
		size, err = r.zip(ctx, partial)
	} else {
		size, err = r.folder(ctx, partial)
	}
	if err == nil {
		err = finish(partial, target, o)
	}
	if err != nil {
		if rm := os.RemoveAll(partial); rm != nil {
			p.m.d.Log.Warn("project: a failed export could not be removed", "path", partial, "err", rm)
		}
		if ctx.Err() != nil { // SQLite reports a cancelled copy as interrupted
			return ExportResult{}, fmt.Errorf("project: export stopped: %w", ctx.Err())
		}
		return ExportResult{}, fmt.Errorf("project: export: %w", err)
	}
	r.step(StepDone)
	res := ExportResult{Path: target, Format: o.Format, ID: r.newID, Size: size, Objects: r.objects, Missing: r.missing,
		Millis: time.Since(r.start).Milliseconds()}
	p.m.d.Log.Info("project: exported", "project", p.ID, "copy", r.newID, "path", target, "format", o.Format,
		"bytes", size, "objects", r.objects, "missing", r.missing, "ms", res.Millis)
	return res, nil
}

// recordExt ends the name of the file in tmp/ that holds where a running
// export writes.
const recordExt = ".partial-path"

// isExportRecord tells an export's record in tmp/ by its name. Recovery
// keeps them: one is left only when cleanExports couldn't remove its
// .partial, and the next open tries again.
func isExportRecord(name string) bool {
	return strings.HasPrefix(name, "export-") && strings.HasSuffix(name, recordExt)
}

func writeRecord(name, partial string) error {
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	_, err = f.WriteString(partial)
	if err == nil {
		err = f.Sync()
	}
	return errors.Join(err, f.Close())
}

// cleanExports removes what exports that never ended left behind: their
// database copies in tmp/, and the .partial each one recorded there. A
// recorded path is removed only if its name is the one that export gave
// it, so a damaged record can't point it anywhere else. It runs at every
// open, before recovery empties tmp/.
func cleanExports(dir string, log *slog.Logger) {
	tmp := filepath.Join(dir, "tmp")
	ents, _ := os.ReadDir(tmp)
	for _, d := range ents {
		name, ok := strings.CutPrefix(d.Name(), "export-")
		if !ok {
			continue
		}
		path := filepath.Join(tmp, d.Name())
		if cid, ok := strings.CutSuffix(name, recordExt); ok && isExportRecord(d.Name()) && d.Type().IsRegular() {
			b, err := os.ReadFile(path)
			partial := string(b)
			if err == nil && id.Valid(cid) && filepath.IsAbs(partial) && strings.HasSuffix(filepath.Base(partial), "."+cid+".partial") {
				if err := os.RemoveAll(partial); err != nil {
					log.Warn("project: an export's partial file could not be removed", "path", partial, "err", err)
					continue // the record stays, through recovery too, and the next open tries again
				}
				log.Info("project: removed what an unfinished export left", "path", partial)
			}
		} else if !id.Valid(name) {
			continue
		}
		if err := os.RemoveAll(path); err != nil {
			log.Warn("project: an export's files in tmp/ could not be removed", "path", path, "err", err)
		}
	}
}

// waitExport waits, at most until ctx ends, for a running export to stop
// and remove what it wrote; close cancelled it.
func (p *Project) waitExport(ctx context.Context) {
	p.mu.Lock()
	done := p.exportDone
	p.mu.Unlock()
	if done == nil {
		return
	}
	select {
	case <-done:
	case <-ctx.Done():
	}
}

// checkTarget refuses a place that is taken: an existing zip file is
// replaced only with o.Replace, and an existing folder only when empty.
func checkTarget(target string, o ExportOptions) error {
	st, err := os.Lstat(target)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return err
	case o.Format == ExportZip && st.Mode().IsRegular() && o.Replace:
		return nil
	case o.Format == ExportFolder && st.IsDir():
		ents, err := os.ReadDir(target)
		if err != nil {
			return err
		}
		if len(ents) == 0 {
			return nil
		}
	}
	return fmt.Errorf("%w: %s", ErrExportExists, target)
}

// checkSpace refuses an export that would not fit. The databases and
// objects are counted as they are now, uncompressed; a zip export also
// needs room for the database copies in the project's tmp/.
func checkSpace(target, dir string, f ExportFormat, est ExportEstimate) error {
	free, vol, ok := freeSpace(filepath.Dir(target))
	if !ok {
		return nil
	}
	need := uint64(est.Bytes) + spaceMargin
	if f == ExportZip {
		pfree, pvol, ok := freeSpace(dir)
		switch {
		case ok && pvol == vol:
			need += uint64(est.Databases)
		case ok && pfree < uint64(est.Databases)+spaceMargin:
			return fmt.Errorf("%w: the copies of the databases need %s in the project's folder, and %s is free",
				ErrNoSpace, bytesText(est.Databases+spaceMargin), bytesText(int64(pfree)))
		}
	}
	if free < need {
		return fmt.Errorf("%w: it needs %s, and %s is free there", ErrNoSpace, bytesText(int64(need)), bytesText(int64(free)))
	}
	return nil
}

// finish gives the export its final name. What appeared at the target
// while the export ran stays: only an empty folder gives way to a folder
// export, and only Replace lets a zip export replace a file.
func finish(partial, target string, o ExportOptions) error {
	if o.Format == ExportZip && o.Replace {
		return os.Rename(partial, target)
	}
	if o.Format == ExportFolder {
		st, err := os.Lstat(target)
		switch {
		case errors.Is(err, fs.ErrNotExist):
		case err != nil:
			return err
		case !st.IsDir():
			return fmt.Errorf("%w: %s", ErrExportExists, target)
		default:
			if err := rmdir(target); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return fmt.Errorf("%w: %s", ErrExportExists, target) // no longer empty
			}
		}
	}
	err := renameNoReplace(partial, target)
	if errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("%w: %s", ErrExportExists, target)
	}
	return err
}

// exportRun is one export.
type exportRun struct {
	p      *Project
	o      ExportOptions
	target string
	start  time.Time
	newID  id.Project

	missing, objects int

	mu          sync.Mutex
	stepNow     ExportStep
	done, total int64
	last        time.Time // of the last progress report
	active      time.Time // of the last byte copied
}

// Status reports the export to the runtime panel (Reporter).
func (r *exportRun) Status() []Status {
	r.mu.Lock()
	defer r.mu.Unlock()
	return []Status{{ID: "export:" + string(r.newID), Kind: "export", Title: filepath.Base(r.target), State: "running",
		Started: r.start, LastActivity: r.active, Progress: fmt.Sprintf("%s: %s of %s", r.stepNow, bytesText(r.done), bytesText(r.total))}}
}

// step starts a step and reports it.
func (r *exportRun) step(s ExportStep) {
	r.mu.Lock()
	r.stepNow = s
	r.mu.Unlock()
	r.report(true)
}

// add counts n bytes copied.
func (r *exportRun) add(n int64) {
	r.mu.Lock()
	r.done += n
	r.active = time.Now()
	r.mu.Unlock()
	r.report(false)
}

// retotal sets the bytes to copy, once they are known.
func (r *exportRun) retotal(total int64) {
	r.mu.Lock()
	r.total = total
	r.mu.Unlock()
}

func (r *exportRun) report(now bool) {
	if r.o.Progress == nil {
		return
	}
	r.mu.Lock()
	t := time.Now()
	if !now && t.Sub(r.last) < progressEvery {
		r.mu.Unlock()
		return
	}
	r.last = t
	pr := ExportProgress{Project: r.p.ID, Target: r.target, Step: r.stepNow, Done: r.done, Total: max(r.total, r.done)}
	r.mu.Unlock()
	r.o.Progress(pr)
}

// copyDatabases copies project.db and chats.db into dir and returns the
// objects the copy of project.db points to.
func (r *exportRun) copyDatabases(ctx context.Context, dir string) ([]store.StoredObject, error) {
	r.step(StepProject)
	e := store.CopyEdit{Meta: map[string]string{
		metaID:           string(r.newID),
		metaExportedFrom: string(r.p.ID),
		metaExportedAt:   r.start.UTC().Format(time.RFC3339),
	}}
	if !r.o.IncludeApprovals {
		// The copy may go to someone else: it trusts nothing the original
		// was allowed, and starts at the level a new project gets.
		e.Meta[metaLevel] = string(r.p.m.d.Level())
		e.DropMeta = []string{metaPrivateHosts}
		e.DropApprovals = true
	}
	objs, err := r.p.DB.ExportCopy(ctx, filepath.Join(dir, "project.db"), e)
	if err != nil {
		return nil, err
	}
	pdb := fileSize(filepath.Join(dir, "project.db"))
	r.add(pdb)
	r.step(StepChats)
	if err := r.p.Chats.ExportCopy(ctx, filepath.Join(dir, "chats.db")); err != nil {
		return nil, err
	}
	cdb := fileSize(filepath.Join(dir, "chats.db"))
	r.add(cdb)
	// A hash that can't name a file in objects/ has no bytes to copy.
	objs = slices.DeleteFunc(objs, func(o store.StoredObject) bool {
		if bucket.ValidHash(o.Hash) {
			return false
		}
		r.missing++
		r.p.m.d.Log.Warn("project: export: an object has a bad hash", "project", r.p.ID, "sha256", o.Hash)
		return true
	})
	// Now the total is known, but for skills/.
	total := pdb + cdb
	if r.o.Format == ExportZip {
		total *= 2
	}
	for _, o := range objs {
		total += o.Size
	}
	r.retotal(total + skillsSize(r.p.Dir))
	return objs, nil
}

// folder writes the export as a folder at partial.
func (r *exportRun) folder(ctx context.Context, partial string) (int64, error) {
	root := filepath.Join(partial, string(r.newID))
	for _, sub := range []string{"objects", "snapshots", "tmp"} {
		if err := os.MkdirAll(filepath.Join(root, sub), 0o755); err != nil {
			return 0, err
		}
	}
	objs, err := r.copyDatabases(ctx, root)
	if err != nil {
		return 0, err
	}
	size := fileSize(filepath.Join(root, "project.db")) + fileSize(filepath.Join(root, "chats.db"))
	r.step(StepObjects)
	for _, o := range objs {
		n, err := r.copyObject(ctx, o, func(src io.Reader) (int64, error) {
			dst := filepath.Join(root, "objects", filepath.FromSlash(bucket.RelPath(o.Hash)))
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return 0, err
			}
			return r.writeFile(ctx, dst, src)
		})
		if err != nil {
			return 0, err
		}
		size += n
	}
	r.step(StepSkills)
	files, err := skillFiles(r.p.Dir)
	if err != nil {
		return 0, err
	}
	for _, rel := range files {
		dst := filepath.Join(root, "skills", filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return 0, err
		}
		n, err := r.copyPath(ctx, filepath.Join(r.p.Dir, "skills", filepath.FromSlash(rel)), func(src io.Reader) (int64, error) {
			return r.writeFile(ctx, dst, src)
		})
		if errors.Is(err, fs.ErrNotExist) {
			continue // removed while the export ran
		}
		if err != nil {
			return 0, err
		}
		size += n
	}
	return size, nil
}

// zip writes the export as one zip file at partial. The entries are in
// name order, and all carry the export's start time.
func (r *exportRun) zip(ctx context.Context, partial string) (size int64, err error) {
	stage := filepath.Join(r.p.Dir, "tmp", "export-"+string(r.newID))
	if err := os.MkdirAll(stage, 0o755); err != nil {
		return 0, err
	}
	defer func() {
		if rm := os.RemoveAll(stage); rm != nil {
			r.p.m.d.Log.Warn("project: the export's database copies could not be removed; recovery removes them", "path", stage, "err", rm)
		}
	}()
	objs, err := r.copyDatabases(ctx, stage)
	if err != nil {
		return 0, err
	}
	skills, err := skillFiles(r.p.Dir)
	if err != nil {
		return 0, err
	}

	// The entries, in name order.
	type entry struct {
		name string
		step ExportStep
		obj  *store.StoredObject // an object's bytes
		file string              // a file to copy; "" with obj nil is a folder
	}
	top := string(r.newID) + "/"
	es := []entry{
		{name: top},
		// No step of their own: copyDatabases said them already.
		{name: top + "project.db", file: filepath.Join(stage, "project.db")},
		{name: top + "chats.db", file: filepath.Join(stage, "chats.db")},
		{name: top + "objects/"},
		{name: top + "snapshots/"},
		{name: top + "tmp/"},
	}
	for i := range objs {
		es = append(es, entry{name: top + "objects/" + bucket.RelPath(objs[i].Hash), step: StepObjects, obj: &objs[i]})
	}
	if len(skills) > 0 {
		es = append(es, entry{name: top + "skills/"})
	}
	for _, rel := range skills {
		es = append(es, entry{name: top + "skills/" + rel, step: StepSkills, file: filepath.Join(r.p.Dir, "skills", filepath.FromSlash(rel))})
	}
	slices.SortFunc(es, func(a, b entry) int { return strings.Compare(a.name, b.name) })

	f, err := os.OpenFile(partial, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return 0, err
	}
	defer func() {
		if f != nil {
			f.Close()
		}
	}()
	bw := bufio.NewWriterSize(f, 1<<20)
	zw := zip.NewWriter(bw)
	zw.RegisterCompressor(zip.Deflate, func(w io.Writer) (io.WriteCloser, error) {
		return flate.NewWriter(w, flate.BestSpeed)
	})
	mod := r.start.Truncate(time.Second) // local, as zip tools show it
	step := ExportStep("")
	for _, e := range es {
		if e.step != "" && e.step != step {
			step = e.step
			r.step(step)
		}
		h := &zip.FileHeader{Name: e.name, Modified: mod}
		if e.obj == nil && e.file == "" {
			h.SetMode(fs.ModeDir | 0o755)
			if _, err := zw.CreateHeader(h); err != nil {
				return 0, err
			}
			continue
		}
		h.Method = zip.Deflate
		h.SetMode(0o644)
		write := func(src io.Reader) (int64, error) {
			w, err := zw.CreateHeader(h)
			if err != nil {
				return 0, err
			}
			return io.Copy(&progressWriter{w: w, r: r}, ctxReader{ctx, src})
		}
		if e.obj != nil {
			_, err = r.copyObject(ctx, *e.obj, write)
		} else {
			_, err = r.copyPath(ctx, e.file, write)
		}
		if errors.Is(err, fs.ErrNotExist) && e.step == StepSkills {
			err = nil // removed while the export ran
		}
		if err != nil {
			return 0, err
		}
	}
	if err := zw.Close(); err != nil {
		return 0, err
	}
	if err := bw.Flush(); err != nil {
		return 0, err
	}
	if err := f.Sync(); err != nil {
		return 0, err
	}
	st, err := f.Stat()
	if err != nil {
		return 0, err
	}
	err = f.Close()
	f = nil
	return st.Size(), err
}

// copyObject passes the bytes of o to write. Bytes that are missing are
// counted and left out.
func (r *exportRun) copyObject(ctx context.Context, o store.StoredObject, write func(io.Reader) (int64, error)) (int64, error) {
	src, err := r.p.DB.OpenStored(ctx, o.Hash)
	if errors.Is(err, bucket.ErrNotFound) {
		r.missing++
		r.p.m.d.Log.Warn("project: export: an object's bytes are missing", "project", r.p.ID, "sha256", o.Hash)
		r.add(o.Size) // counted in the total
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer src.Close()
	r.objects++
	return write(src)
}

// copyPath passes the file at name to write. A file that is gone gives
// fs.ErrNotExist before write is called.
func (r *exportRun) copyPath(ctx context.Context, name string, write func(io.Reader) (int64, error)) (int64, error) {
	src, err := os.Open(name)
	if err != nil {
		return 0, err
	}
	defer src.Close()
	return write(src)
}

// writeFile writes src to the new file dst and syncs it.
func (r *exportRun) writeFile(ctx context.Context, dst string, src io.Reader) (int64, error) {
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(&progressWriter{w: out, r: r}, ctxReader{ctx, src})
	if err == nil {
		err = out.Sync()
	}
	return n, errors.Join(err, out.Close())
}

// skillFiles lists the regular files in the project's skills/, as slash
// paths in name order. Links and other entries are left out.
func skillFiles(dir string) ([]string, error) {
	root := filepath.Join(dir, "skills")
	var out []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil // no skills/, or removed while it is walked
			}
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	slices.Sort(out)
	return out, err
}

func skillsSize(dir string) int64 {
	var n int64
	files, _ := skillFiles(dir)
	for _, f := range files {
		n += fileSize(filepath.Join(dir, "skills", filepath.FromSlash(f)))
	}
	return n
}

func fileSize(name string) int64 {
	st, err := os.Stat(name)
	if err != nil {
		return 0
	}
	return st.Size()
}

// bytesText writes n for people, e.g. "12.3 MB".
func bytesText(n int64) string {
	const unit = 1000
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	f, i := float64(n), 0
	for f >= unit && i < 4 {
		f /= unit
		i++
	}
	return fmt.Sprintf("%.1f %cB", f, "kMGT"[i-1])
}

// progressWriter counts the bytes written into the export.
type progressWriter struct {
	w io.Writer
	r *exportRun
}

func (p *progressWriter) Write(b []byte) (int, error) {
	n, err := p.w.Write(b)
	p.r.add(int64(n))
	return n, err
}

// ctxReader stops a copy when ctx ends.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}
