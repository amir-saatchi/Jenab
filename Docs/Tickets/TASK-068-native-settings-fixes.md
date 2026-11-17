# TASK-068 — Settings forms: keys, dialogs and the data folder
**Type:** Task
**Status:** Done
**Gate:** 3
**Needs:** TASK-061

## Goal
The review of the native app (2026-11-17) found small faults in the Settings forms, plus TASK-052's leftover for LM Studio.

## Scope
1. **A typed key stays in memory:** `ConnectForm.key` and its placeholder values (Cloudflare's account ID) are cleared only after a successful connect (`internal/native/settings/connect.go`), and `p.add`/`p.replace` keep the form after the dialog closes (`models.go`). Clear them when the dialog closes, is cancelled, or the provider choice changes.
2. **A stuck spinner:** if `Presets` fails, `asked` is set and `loaded` stays false, so the form shows "Loading the providers" for good (the chat's Connect card may reuse the form). Show the error with *Try again*.
3. **The wrong dialog closes:** a connect that finishes after its dialog was dismissed sets `adding`/`replacing` to false and can close a dialog opened since. Tie the callback to its own dialog.
4. **The data folder:** *Save* doesn't wait for the folder check (debounced 300 ms), and a relative path is then reset to the default with no message. Disable *Save* until the check passes. Refuse a folder inside a project or inside the current projects folder, in the backend as well as the form.
5. **A keyless provider on this computer** (TASK-052): *Settings → Models* shows "Key in the keychain" and *Replace key* for LM Studio, because `logic.Local()` is true only for the Ollama kind. Use the provider's base URL being this computer, the same rule as `provider.localURL`, and say "No key needed". A local LM Studio or other server that does use a token can still be given one: *Add provider* shows an optional key field for a provider on this computer.

## Rules
- Keys go only to the keychain (6.7).

## Done when
- Tests for each of the five.

## Result
Done. Report: [reports/TASK-068-report.md](reports/TASK-068-report.md).
1. `ConnectForm.Clear` drops the key and the placeholder values when the provider choice changes, after a connect, and when the dialog closes. A closed dialog's form is cleared and dropped, so the next opening starts empty.
2. Presets that can't be read show the error and *Try again* in the form.
3. A connect's callback closes its dialog only if its form is still the one shown.
4. *Save* in the data folder dialog waits until the check passes for the folder typed. The backend (`CheckFolder` and `Save`) refuses a relative path, a folder inside the projects folder in use, and a folder inside a project's folder (`project.DataFolderProblem`).
5. Settings uses the registry's rule (`provider.LocalURL`): a provider whose base URL is this computer, LM Studio included, says "no key needed" and has no *Replace key*. *Add provider* shows an optional key field for such a provider.

Each item has headless tests in light and dark, and backend tests cover the data folder rule. A live check on the bench build covered all five. It also found a crash in the first version of item 1, which is fixed and tested. `ConnectForm` stays reusable for TASK-065's Connect card; the report says how to embed it.
