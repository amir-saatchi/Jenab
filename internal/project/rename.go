package project

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// renameNoReplaceSlow is renameNoReplace where the system has no such
// rename. A file is linked, then its old name removed, and a link fails
// when to exists. A folder, or a file where links don't work, is renamed
// after a check, which leaves a short window.
func renameNoReplaceSlow(from, to string) error {
	st, err := os.Lstat(from)
	if err != nil {
		return err
	}
	if st.Mode().IsRegular() {
		err := os.Link(from, to)
		if err == nil {
			return os.Remove(from)
		}
		if errors.Is(err, fs.ErrExist) {
			return err
		}
	}
	if _, err := os.Lstat(to); err == nil {
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: fs.ErrExist}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("rename %s: %w", to, err)
	}
	return os.Rename(from, to)
}
