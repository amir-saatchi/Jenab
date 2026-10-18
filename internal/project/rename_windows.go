package project

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// renameNoReplace renames from to to, and fails with an error that is
// fs.ErrExist when to exists, whatever it is. MoveFileEx without
// MOVEFILE_REPLACE_EXISTING never replaces.
func renameNoReplace(from, to string) error {
	f, err := windows.UTF16PtrFromString(longPath(from))
	if err != nil {
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: err}
	}
	t, err := windows.UTF16PtrFromString(longPath(to))
	if err != nil {
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: err}
	}
	if err := windows.MoveFileEx(f, t, windows.MOVEFILE_WRITE_THROUGH); err != nil {
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: err}
	}
	return nil
}

// rmdir removes the folder name if it is empty, and never a file.
func rmdir(name string) error {
	p, err := windows.UTF16PtrFromString(longPath(name))
	if err != nil {
		return &os.PathError{Op: "rmdir", Path: name, Err: err}
	}
	if err := windows.RemoveDirectory(p); err != nil {
		return &os.PathError{Op: "rmdir", Path: name, Err: err}
	}
	return nil
}

// longPath gives an absolute path the \\?\ prefix, as package os does,
// so a path past MAX_PATH works. The prefix turns off Windows' own
// cleaning of the path, so it is cleaned first.
func longPath(p string) string {
	if !filepath.IsAbs(p) || strings.HasPrefix(p, `\\?\`) {
		return p
	}
	p = filepath.Clean(p)
	if len(p) < 248 {
		return p
	}
	if strings.HasPrefix(p, `\\`) { // UNC: \\server\share\…
		return `\\?\UNC\` + p[2:]
	}
	return `\\?\` + p
}
