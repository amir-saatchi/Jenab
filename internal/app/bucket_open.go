package app

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"mime"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/project"
)

// openDir is the folder under the temp folder that holds the copies
// opened with the system app, one folder per content hash.
const openDir = "jenab-open"

// openKeep is how long a copy stays; older ones go at the next start.
const openKeep = 24 * time.Hour

// systemExt is the extension a file gets when it is opened with the system
// app, from the type detected when it was stored (SPEC 4.7), or "" when it
// is refused. It is the whole allow-list: the OS picks the app by this
// extension, so the key's own extension never decides what runs, and a
// key like run.txt.exe or evil.pdf that holds a program is refused.
//   - Shown images, audio, video and PDF open as themselves; SVG doesn't,
//     since a browser would open it and run its scripts.
//   - Text opens in the text editor: CSV, TSV, Markdown and JSON keep their
//     extension, any other text (HTML, XML, scripts) becomes .txt.
//   - Office and OpenDocument files are ZIPs to the detector, so a ZIP
//     opens only when its key ends in one of their macro-free extensions.
//     This is a stopgap until TASK-021 detects Office files properly;
//     macro-enabled types (.docm, .xlsm, .pptm, …) stay refused.
//   - Everything else is refused: programs, libraries, installers and
//     shortcuts (all application/octet-stream to the detector), archives,
//     fonts and unknown types.
func systemExt(mimeType, key string) string {
	base, _, err := mime.ParseMediaType(mimeType)
	if err != nil {
		return ""
	}
	if ext, ok := systemTypes[base]; ok {
		return ext
	}
	if base == "image/svg+xml" {
		return ".txt"
	}
	if strings.HasPrefix(base, "text/") || base == "application/xml" || strings.HasSuffix(base, "+xml") || base == "application/javascript" {
		return ".txt"
	}
	if base == "application/zip" {
		if ext := strings.ToLower(path.Ext(key)); officeExts[ext] {
			return ext
		}
	}
	return ""
}

var systemTypes = map[string]string{
	"image/png": ".png", "image/jpeg": ".jpg", "image/gif": ".gif", "image/webp": ".webp",
	"image/bmp": ".bmp", "image/avif": ".avif", "image/x-icon": ".ico", "image/vnd.microsoft.icon": ".ico",
	"audio/mpeg": ".mp3", "audio/wave": ".wav", "audio/aiff": ".aiff", "audio/basic": ".au", "audio/midi": ".mid",
	"application/ogg": ".ogg", "video/mp4": ".mp4", "video/webm": ".webm", "video/avi": ".avi",
	"application/pdf": ".pdf", "text/plain": ".txt", "text/csv": ".csv", "text/tab-separated-values": ".tsv",
	"text/markdown": ".md", "application/json": ".json",
}

var officeExts = map[string]bool{
	".docx": true, ".xlsx": true, ".pptx": true, ".odt": true, ".ods": true, ".odp": true,
}

// errRefusedType is a file the system app may not open.
var errRefusedType = &UIError{Kind: KindInvalid,
	Message: "This type of file isn't opened outside the app, since it could run a program. It stays in the project."}

// copyName is the name of a key's copy: its last segment with ext in
// place of its own extension. Windows' reserved device names get a "_".
func copyName(key, ext string) string {
	name := path.Base(key)
	name = strings.TrimSuffix(name, path.Ext(name))
	name = strings.Trim(name, ".")
	if name == "" {
		name = "file"
	}
	stem, _, _ := strings.Cut(name, ".")
	switch strings.ToUpper(stem) {
	case "CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		name = "_" + name
	}
	return name + ext
}

// OpenWithSystem opens a file's current version with the system app, after
// an explicit click (SPEC 4.7). Nothing in the bucket is opened in place:
// the bytes are copied to a read-only file in the temp folder, named by
// systemExt, and marked as downloaded from the internet, so Office opens
// it in Protected View and Windows warns before anything in it runs.
func (s *BucketService) OpenWithSystem(ctx context.Context, p id.Project, key string) (err error) {
	defer s.guard("bucket.open_with_system", &err)
	var file string
	err = open(ctx, s.projects, p, func(proj *project.Project) error {
		o, body, err := proj.DB.OpenObject(ctx, key)
		if err != nil {
			return err
		}
		defer body.Close()
		ext := systemExt(o.MIME, o.Key)
		if ext == "" {
			return errRefusedType
		}
		file, err = s.copyOut(o.SHA256, copyName(o.Key, ext), o.Size, body)
		return err
	})
	if err != nil || s.openFile == nil {
		return err
	}
	return s.openFile(file)
}

func (s *BucketService) openRoot() string {
	if s.tempDir != "" {
		return filepath.Join(s.tempDir, openDir)
	}
	return filepath.Join(os.TempDir(), openDir)
}

// copyOut writes body to <temp>/jenab-open/<hash>/<name>, or reuses a copy
// that is there already.
func (s *BucketService) copyOut(hash, name string, size int64, body io.Reader) (string, error) {
	dir := filepath.Join(s.openRoot(), hash[:min(len(hash), 32)])
	file := filepath.Join(dir, name)
	if fi, err := os.Stat(file); err == nil && fi.Mode().IsRegular() && fi.Size() == size {
		now := time.Now()
		_ = os.Chtimes(dir, now, now) // kept another day
		return file, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(dir, ".part-*")
	if err != nil {
		return "", err
	}
	_, err = io.Copy(tmp, body)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		_ = os.Chmod(file, 0o600) // an old copy of another size, read-only
		err = os.Rename(tmp.Name(), file)
	}
	if err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	markDownloaded(file)
	if err := os.Chmod(file, 0o400); err != nil {
		return "", err
	}
	return file, nil
}

// cleanOpened removes the copies older than a day. The app runs it at the
// start; a copy still open in another app is left for the next time.
func (s *BucketService) cleanOpened() {
	root := s.openRoot()
	ents, err := os.ReadDir(root)
	if err != nil {
		return
	}
	cut := time.Now().Add(-openKeep)
	for _, e := range ents {
		fi, err := e.Info()
		if err != nil || fi.ModTime().After(cut) {
			continue
		}
		dir := filepath.Join(root, e.Name())
		_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				_ = os.Chmod(p, 0o600) // read-only files can't be removed on Windows
			}
			return nil
		})
		if err := os.RemoveAll(dir); err != nil && !errors.Is(err, fs.ErrNotExist) {
			s.log.Debug("app: an opened copy is still in use", "dir", dir, "err", err)
		}
	}
}
