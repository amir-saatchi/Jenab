package secret

import (
	"errors"

	"github.com/zalando/go-keyring"
)

// OSKeyring is the OS keychain under one service name: Credential Manager
// on Windows, Keychain on macOS, Secret Service on Linux. A name such as
// "provider:anthropic" is stored as "<service>:provider:anthropic".
func OSKeyring(service string) Keyring {
	return osKeyring{service}
}

type osKeyring struct{ service string }

func (k osKeyring) Get(name string) (string, error) {
	v, err := keyring.Get(k.service, name)
	return v, mapErr(err)
}

func (k osKeyring) Set(name, value string) error {
	return mapErr(keyring.Set(k.service, name, value))
}

func (k osKeyring) Delete(name string) error {
	return mapErr(keyring.Delete(k.service, name))
}

func mapErr(err error) error {
	if errors.Is(err, keyring.ErrNotFound) {
		return ErrNotFound
	}
	return err
}
