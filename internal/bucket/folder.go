package bucket

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"iter"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// Blob is content that was stored.
type Blob struct {
	Hash string // SHA-256, lower-case hex
	Size int64
	Head []byte // the first bytes, for DetectMIME
}

// Stored is one file in the store, for the sweep.
type Stored struct {
	Hash    string
	Size    int64
	ModTime time.Time
}

// ObjectStore holds content by hash (SPEC 4.8). v1 is a local folder;
// an S3-compatible store can come later.
type ObjectStore interface {
	// Put stores r and returns its hash. A reader longer than max fails
	// with ErrTooLarge. Content that is already there is stored once.
	Put(ctx context.Context, r io.Reader, max int64) (Blob, error)
	// Open reads the content with hash; ErrNotFound if it isn't there.
	Open(ctx context.Context, hash string) (io.ReadSeekCloser, error)
	// Delete removes the content; missing content is not an error.
	Delete(ctx context.Context, hash string) error
	// All lists the stored content, for the sweep (SPEC 4.4).
	All(ctx context.Context) iter.Seq2[Stored, error]
	// CleanTmp removes temporary files last changed before t.
	CleanTmp(ctx context.Context, t time.Time) (int, error)
}

// headSize is how much DetectMIME reads, as http.DetectContentType does.
const headSize = 512

// Folder is an ObjectStore in a folder: objects/ab/12/ab12f9… Writes go
// through tmp/, which must be on the same volume.
type Folder struct {
	objects, tmp string
}

// NewFolder returns the store for a project's objects/ and tmp/ folders.
func NewFolder(objects, tmp string) *Folder { return &Folder{objects: objects, tmp: tmp} }

var _ ObjectStore = (*Folder)(nil)

// ValidHash reports whether s is a lower-case hex SHA-256.
func ValidHash(s string) bool {
	if len(s) != 2*sha256.Size {
		return false
	}
	for i := 0; i < len(s); i++ {
		if c := s[i]; !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func (f *Folder) path(hash string) string {
	return filepath.Join(f.objects, hash[:2], hash[2:4], hash)
}

// Put follows the write order in SPEC 4.3: the bytes go to tmp/ while the
// hash is computed, are flushed, and then renamed into objects/. The
// caller writes the index row after Put returns.
func (f *Folder) Put(ctx context.Context, r io.Reader, max int64) (b Blob, err error) {
	if err := os.MkdirAll(f.tmp, 0o755); err != nil {
		return Blob{}, err
	}
	tmp, err := os.CreateTemp(f.tmp, "put-*.partial")
	if err != nil {
		return Blob{}, err
	}
	defer func() {
		if tmp != nil {
			tmp.Close()
			os.Remove(tmp.Name())
		}
	}()
	h := sha256.New()
	head := &headWriter{max: headSize}
	src := io.Reader(ctxReader{ctx, r})
	if max > 0 {
		src = io.LimitReader(src, max+1)
	}
	n, err := io.Copy(io.MultiWriter(tmp, h, head), src)
	if err != nil {
		return Blob{}, err
	}
	if max > 0 && n > max {
		return Blob{}, fmt.Errorf("%w: more than %d bytes", ErrTooLarge, max)
	}
	if err := tmp.Sync(); err != nil {
		return Blob{}, err
	}
	if err := tmp.Close(); err != nil {
		return Blob{}, err
	}
	b = Blob{Hash: hex.EncodeToString(h.Sum(nil)), Size: n, Head: head.b}
	dst := f.path(b.Hash)
	if _, err := os.Stat(dst); err == nil {
		return b, nil // stored already; the deferred cleanup removes tmp
	}
	dir := filepath.Dir(dst)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Blob{}, err
	}
	if err := os.Rename(tmp.Name(), dst); err != nil {
		if _, serr := os.Stat(dst); serr == nil {
			return b, nil // another Put stored the same bytes first
		}
		return Blob{}, err
	}
	tmp = nil
	// The rename must be on disk before the index row (SPEC 4.3).
	for _, d := range []string{dir, filepath.Dir(dir), f.objects} {
		if err := syncDir(d); err != nil {
			return Blob{}, err
		}
	}
	return b, nil
}

// Open opens the content with hash.
func (f *Folder) Open(_ context.Context, hash string) (io.ReadSeekCloser, error) {
	if !ValidHash(hash) {
		return nil, fmt.Errorf("bucket: %q is not a hash", hash)
	}
	file, err := os.Open(f.path(hash))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, hash)
	}
	return file, err
}

// Delete removes the content with hash.
func (f *Folder) Delete(_ context.Context, hash string) error {
	if !ValidHash(hash) {
		return fmt.Errorf("bucket: %q is not a hash", hash)
	}
	err := os.Remove(f.path(hash))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// All lists the files in objects/ whose place matches their name. Other
// files are left alone: they are not the bucket's.
func (f *Folder) All(ctx context.Context) iter.Seq2[Stored, error] {
	return func(yield func(Stored, error) bool) {
		for _, a := range readDir(f.objects) {
			if !a.IsDir() || len(a.Name()) != 2 {
				continue
			}
			for _, b := range readDir(filepath.Join(f.objects, a.Name())) {
				if !b.IsDir() || len(b.Name()) != 2 {
					continue
				}
				for _, e := range readDir(filepath.Join(f.objects, a.Name(), b.Name())) {
					if err := ctx.Err(); err != nil {
						yield(Stored{}, err)
						return
					}
					n := e.Name()
					if !e.Type().IsRegular() || !ValidHash(n) || n[:2] != a.Name() || n[2:4] != b.Name() {
						continue
					}
					info, err := e.Info()
					if err != nil {
						continue // removed meanwhile
					}
					if !yield(Stored{Hash: n, Size: info.Size(), ModTime: info.ModTime()}, nil) {
						return
					}
				}
			}
		}
	}
}

// CleanTmp removes what is in tmp/ and was last changed before t.
func (f *Folder) CleanTmp(ctx context.Context, t time.Time) (int, error) {
	n := 0
	for _, e := range readDir(f.tmp) {
		if err := ctx.Err(); err != nil {
			return n, err
		}
		info, err := e.Info()
		if err != nil || !info.ModTime().Before(t) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(f.tmp, e.Name())); err == nil {
			n++
		}
	}
	return n, nil
}

// readDir lists dir, or nothing if it can't be read.
func readDir(dir string) []os.DirEntry {
	es, _ := os.ReadDir(dir)
	return es
}

// syncDir flushes a folder, so a rename into it survives a crash. Windows
// can't open a folder for that; NTFS journals the rename itself.
func syncDir(dir string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	return errors.Join(d.Sync(), d.Close())
}

// headWriter keeps the first max bytes written to it.
type headWriter struct {
	b   []byte
	max int
}

func (w *headWriter) Write(p []byte) (int, error) {
	if room := w.max - len(w.b); room > 0 {
		w.b = append(w.b, p[:min(room, len(p))]...)
	}
	return len(p), nil
}

// ctxReader stops reading when ctx ends.
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
