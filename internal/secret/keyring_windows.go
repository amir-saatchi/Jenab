package secret

import "github.com/danieljoos/wincred"

// Names lists the app's entries in Credential Manager, the way go-keyring
// names them ("<service>:<name>").
func (k osKeyring) Names() ([]string, error) {
	creds, err := wincred.FilteredList(k.service + ":*")
	if err != nil {
		return nil, err
	}
	targets := make([]string, 0, len(creds))
	for _, c := range creds {
		targets = append(targets, c.TargetName)
	}
	return namesUnder(k.service, targets), nil
}
