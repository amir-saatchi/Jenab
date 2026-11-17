//go:build !windows

package secret

// Names can't list the keychain here: go-keyring has no way to on macOS
// and Linux, so a removed key is found by its shape only (Shapes).
func (k osKeyring) Names() ([]string, error) { return nil, ErrNotListable }
