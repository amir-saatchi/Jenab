//go:build !windows && !linux && !darwin

package project

// renameNoReplace renames from to to, and fails with an error that is
// fs.ErrExist when to exists.
func renameNoReplace(from, to string) error { return renameNoReplaceSlow(from, to) }
