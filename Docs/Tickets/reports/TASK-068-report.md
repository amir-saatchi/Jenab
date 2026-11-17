# TASK-068 report: Settings forms, keys, dialogs and the data folder

**Base:** `main` at eb1031d, with MyGo v0.9.1 pinned.
**Model requests:** none. The tests use `statetest`'s fake services. The live check connected a stand-in OpenAI-compatible server on 127.0.0.1 that only lists one model.
**Keys:** none were used or read. The live check typed dummy values (`acct-test`, `sk-test`), which were never sent anywhere, and the bench keyring is in memory.
**Real data:** the live check used a scratch profile outside the repo. No system dialogs were opened.

## 1. A typed key is dropped

`ConnectForm` has a new method, `Clear`. It empties the key and the base URL's placeholder values, such as Cloudflare's account ID. It is called:
- when another provider is chosen in the form's menu, so a key or account ID typed for one provider never goes to another one's URL;
- after a connect that worked;
- when the dialog closes, whether by Cancel, Escape or the close button. `models.go`'s `closeForms` runs after each frame's dialogs. It clears the form of a closed *Add provider* or *Replace key* dialog and drops it (`p.add`, `p.replace` = nil), so a new form is made the next time the dialog opens.

`Clear` empties the placeholder map in place rather than setting it to nil. The live check found the nil map crashed the app: the menu's pick and the field loop run in the same frame, and the loop then wrote into a nil map. The test now picks Cloudflare, with its account ID field, to cover this.

Go strings can't be wiped from memory. The form drops its references, so the key is no longer reachable from the UI and goes with the next garbage collection.

## 2. Presets that can't be read

When `Presets` fails, the form shows a red alert, "The providers couldn't be read", with the error and a *Try again* button, instead of "Loading the providers" for good. *Try again* asks again. The error no longer goes to the global error banner, since the form shows it in place, and the chat's Connect card will too.

## 3. A late connect

The callback a dialog's form gets closes the dialog only if that form is still the one shown (`p.add == form`, `p.replace == form`). A connect that ends after its dialog was closed still saves the provider and shows its toast, but leaves a dialog opened since then alone.

## 4. The data folder

- **Form:** *Save* is off until the folder check passes for the folder now typed. The check runs 300 ms after typing pauses, and the dialog stores the folder it passed (`passed`). Typing again turns *Save* off until the new check answers.
- **Backend:** `project.DataFolderProblem` says why a folder can't be the data folder:
  - it isn't a full path;
  - it is inside the projects folder in use now (`<data folder at start>\projects`);
  - it is inside a project's folder, for each project in the registry.

  The comparison ignores case on Windows and macOS. `SettingsService.CheckFolder` returns that problem as an invalid-input error, and `Save` refuses such a folder with the same error, instead of resetting it to the default. A save that keeps the data folder as it is isn't checked again.

## 5. A provider on this computer

- `provider.LocalURL` exports the registry's rule: a provider whose base URL is this computer (a loopback host, or no URL for a kind whose default URL is this computer, such as Ollama) needs no key. Settings uses it, so `Provider.Local()` is true for LM Studio and any OpenAI-compatible server on this computer, not only for the Ollama kind.
- The row says "On this computer · no key needed", and *Replace key* is hidden.
- *Add provider* shows an optional key field, "API key (optional)", with the hint "Only if the server asks for a token · stored in the OS keychain". It shows for a preset with no key (LM Studio, Ollama) and for *OpenAI-compatible…* once its base URL is on this computer. *Connect* works with the field empty. A key typed there goes with the connect, and from there to the keychain, as any other key.

## Embedding `ConnectForm` (for TASK-065's Connect card)

The form has no dialog of its own, so the chat's Connect card can show it as it is:
1. Make a new `&settings.ConnectForm{Connected: fn}` each time the card opens. Leave `Replace` nil to choose a provider, or set it to a `logic.Replace` to save a new key for a connected one.
2. Call `form.View(c, stores)` in each frame the card shows. It loads the presets itself, shows the error with *Try again* if they can't be read, and connects through `stores.Settings.Connect`.
3. Call `form.Clear()` when the card closes or is cancelled, then drop the form.
4. `Connected` may arrive after the card was closed, for a connect already sent. Before acting on it, check that the form is still the one shown, as `openAdd` in `models.go` does.

The doc comment on `ConnectForm` says the same.

## Tests (headless, light and dark)

- `internal/native/settings/forms_test.go`:
  - `TestAKeyIsDroppedWhenTheProviderChanges`: Cloudflare's account ID and key, then Anthropic (*Connect* off), back to Cloudflare (the account ID field is empty), Anthropic to Gemini, and a dialog closed with Escape and opened again. No connect is sent.
  - `TestThePresetsErrorOffersTryAgain`
  - `TestALateConnectLeavesANewDialogOpen`: a connect held until after its dialog was closed and a new one opened.
  - `TestTheDataFolderWaitsForItsCheck`: *Save* off before the check and on after it, a relative path and a folder in the projects folder refused, then a good folder saved.
  - `TestAProviderOnThisComputerNeedsNoKey`: LM Studio's row, connects with no key and with a token, and an OpenAI-compatible URL on 127.0.0.1.
- `internal/native/settings/forms_internal_test.go`: `TestClosedDialogsDropTheirForms`, `TestConnectFormClear`.
- `internal/native/settings/logic/local_test.go`: `Local()` and the form's shape for local and remote URLs.
- `internal/project/datafolder_test.go`: `DataFolderProblem` and `within`.
- `internal/app/datafolder_test.go`: `CheckFolder` and `Save` refuse a relative path, the projects folder, and folders inside two projects. A good folder saves, and a save that keeps the folder isn't checked.

With each fix undone in turn, the late-connect, data-folder and Cloudflare tests failed, the last one with the nil-map crash.

## Live check (Windows 11, bench build, scratch profile)

The bench build ran with `-hold`, and the forms were driven through UI Automation.

| Check | Result |
|---|---|
| Settings › Models, the bench's Ollama | "On this computer · no key needed · 1 model on", no *Replace key* |
| Cloudflare: account ID and key typed | *Connect* on |
| Then Anthropic | *Connect* off, the key field empty |
| Back to Cloudflare | the account ID field empty, *Connect* off |
| LM Studio | "API key (optional)" with its hint, *Connect* on with no key |
| OpenAI-compatible "studio" at a 127.0.0.1 URL | optional key field. Connected with no key, and the server saw no Authorization header. The row says "no key needed", with no *Replace key* |
| Data folder in the projects folder | *Save* off right after typing and after the check, with "The data folder can't be inside the projects folder, …" |
| Relative "Jenab" | *Save* off, with "\"Jenab\" is not a full path. Choose a folder." |
| A good folder | *Save* off right after typing, on after the check. Then Cancel |

The first run found the nil-map crash in section 1, and the second run was after the fix.

A Jenab already running on the same account blocks the bench build: MyGo's single-instance lock is keyed by the app's name, so the bench app hands its command line over and exits with code 0 and no output. The lock is a socket file in `XDG_RUNTIME_DIR` when that is set, so setting it to a short scratch folder for the bench run kept the two apart.

## Not done, and gaps

- A folder inside a project's own folder was refused in the backend tests, but not tried live, since the bench project's folder is under the scratch profile's data folder anyway.
- The optional key field shows for an OpenAI-compatible URL on this computer only once the URL is typed. Before that, the field is the required *API key*.

## Follow-ups

- `cmd/jenab-bench` could set `XDG_RUNTIME_DIR` to its scratch home itself, so a running Jenab doesn't stop a bench run (see the live check above).

## Checks

`go build ./...`, `go vet ./...` and `go test ./...` pass. The changed Go files are gofmt-clean.
