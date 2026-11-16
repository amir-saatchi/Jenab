// Package appinfo holds the names the app shows to the user and the
// model, so each is written in one place.
package appinfo

const (
	// Name is the app's name: the window title, the data folder and the
	// keychain service come from it.
	Name = "Jenab"
	// Mother is what the project's home chat is called. It stays the
	// term even if the user renames that chat.
	Mother = "Mother"
)

// Identifier is the app's reverse-DNS name, as mygo.json's identifier:
// the installer's Apps entry is named by it.
const Identifier = "io.github.amir-saatchi.jenab"
