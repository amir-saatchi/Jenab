# TASK-074 report: Replace the token of a provider on this computer

**Base:** `main` at 0e68d3c. **Branch:** `claude/task-074-local-provider-token`.
**Model requests:** none. The tests use fake providers and in-memory keychains.
**Keys:** none were used or read. The tests use dummy tokens.

## What changed

**Settings › Models.** Every provider's row now has *Replace key*, including providers on this computer. For a local provider, the dialog's key field is optional:
- **Typed:** the new token is checked with the server and saved in the keychain, in place of the old one. The models stay as they are.
- **Left empty:** the button reads *Remove key*. The server is checked without a token, and the saved token is deleted from the keychain. If the server still asks for a token, the check fails and the saved token stays.
- **No token saved and nothing typed:** *Save key* stays off, since there is nothing to do.

The dialog's text says which case it is. The toasts are "…: the new key works and is saved" and "…: the key is removed; the server answers without one".

**The row's line.** A local provider with a token says "On this computer · key in the keychain". One with no token still says "On this computer · no key needed". A provider that isn't on this computer is unchanged: "Key in the keychain", with the key required.

## How

- **`provider.Registry.Connect`:** when it is called with no key, it now deletes the stored key once the check passes. Only a provider on this computer can get there, since others are refused without a key. This is what makes an empty field remove the token. Connecting a new local provider with no key does the same, which is a no-op when nothing is stored.
- **`app.SettingsView` has a new field, `LocalKeys`:** the providers on this computer that have a key in the keychain. `SettingsService` fills it in `Get`, `Save`, `Connect` and `Remove` (`internal/app/localkeys.go`). Only whether the key exists is checked; its value isn't used. It costs one keychain read per local provider for each view.
- **Settings logic:**
  - `logic.Provider.Key` drives the row's line.
  - `logic.Replace.Keyed` tells the form whether there is a token to remove.
  - `logic.Replace.Local()` makes `FormFor` give a local provider an optional key.
  - New text: `logic.KeyRemovedText`.
- **`ConnectForm`:** the connect flow is unchanged. Only *Replace key* uses the new parts: `removes()` (an empty optional key in *Replace key*), the button label, the hint, the toast, and the rule that keeps *Save key* off when there is nothing to remove. *Add provider*, and so the chat's Connect card (TASK-071), behaves as before.
- **`statetest.Settings.Save`** keeps `LocalKeys`, as the real service does.

## Tests

- **`internal/provider/localkey_test.go`, `TestALocalKeyIsReplacedAndRemoved`:**
  - two tokens in turn, each checked with the server and stored;
  - a failed check without a token keeps the stored one;
  - a check that passes without one removes it;
  - removing again is not an error.
- **`internal/app/localkeys_test.go`, `TestALocalProvidersKeyIsReplacedAndRemoved`:**
  - connected with no token, then two tokens, then none;
  - each time, the keychain and `LocalKeys` from `Connect`, `Get` and `Save` match;
  - the models stay;
  - a cloud provider isn't listed.
- **`internal/native/settings/localkey_test.go`** (headless, light and dark):
  - `TestALocalProvidersTokenIsReplacedAndRemoved` runs through the row's line, *Replace key*, a new token saved (the request carries it), then *Remove key* (the request carries no key), the row back to "no key needed", and *Save key* off with no token and nothing typed.
  - `TestARemoteKeyStaysNeeded` checks that *Replace key* for Gemini still needs a key, with no *Remove key*.
- **Updated:** `TestTheProviderRows` and `TestAProviderOnThisComputerNeedsNoKey` now expect *Replace key* on local rows. Before, they checked that it was hidden.

With the registry's delete undone, the provider and app tests fail.

## Not done

- **No live check on the bench build.** The change is covered by the headless and backend tests. A live run would need a local server that asks for a token.

## Checks

`go build ./...`, `go vet ./...` and `go test ./...` pass. The changed Go files are gofmt-clean.
