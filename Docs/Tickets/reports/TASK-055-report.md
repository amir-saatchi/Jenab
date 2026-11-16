# TASK-055 report: Settings in the native app

**Base:** `main` at 1f581f4, with MyGo v0.9.1 pinned.
**MyGo:** not moved.
**Model requests:** only the live check: one *Connect* to Google Gemini and its model list, with `gemini-3.5-flash-lite`.
**Downloads:** none.
**Commit:** one, with the code, the tests, the Result, this report and the README row.

## 1. The screen

`internal/native/settings` is the Settings view, ported from `frontend/src/settings`, `shell/settings-layout.tsx` and `shell/settings-view.tsx`. It imports only the kit, the stores and Go-side types; `TestOnlyTheKitImportsMyGoUI` passes.
- **The layout:** a page list on the left and one scrolling column on the right, as in the Wails app.
  - A click on a page scrolls to its section and marks it.
  - *Back* returns to where Settings was opened from.
  - When a project is open, its settings are in the list as a second group. TASK-057 owns that screen, so here they open the project's place.
- **Where it starts:** each visit to Settings starts at the place it was opened for. A visit is found through a keyed `Local` on the screen's root, so going back to the same place counts too.
- **The wiring:**
  - `shell.Shell` has a `Settings` field (a `SettingsScreen` interface), shown for `state.ViewSettings`; the edit is additive.
  - `cmd/desktop-native` sets it, and `Pages.PickFolder` opens MyGo's folder dialog for the data folder.

## 2. The pages

- **General:**
  - **Data folder:** the folder, *Change*, and a dialog that checks the folder as it is typed, *Browse…*, and *Save*, which says the change applies at the next start. The default folder is saved as "".
  - **Approval level** for new projects: Strict, Standard and Auto, as a segmented control.
  - **Other apps' windows:** a switch, on Windows only.
  - **About:** the version and commit, with *Copy*, which shows a toast.
- **Memory:** the user's memory sections in a list, each with an editor, and a meter of the memory used.
  - **The editor:** the section's name and text; *Save*, *Cancel* and *Delete section*.
  - **A conflict:** when the file changed meanwhile, the editor keeps the draft, shows theirs, and offers *Use theirs*.
  - **A section deleted elsewhere** while it is edited stays open as a new section, so the draft can still be saved.
  - **A memory chip's section:** the page opens the section the chip asks for (`Shell.MemoryFocus`).
  - **The scope line** links to the open project's memory, *Project settings › Memory*.
  - **Exported:** `MemoryEditor` and `MemoryScope`, for TASK-057's project memory.
- **Appearance:** Light, Dark and System, with the sun, moon and monitor icons. The change applies at once.
- **Models:**
  - **Providers:** each row has its icon, name, kind badge and second line, the base URL, and *Background calls at once* as a number input. The row's buttons are *Models of X*, *Replace the key of X* and *Remove X*.
  - **Add provider:** the TASK-052 form, `ConnectForm`, with the presets grouped as in the frontend. It has fields for the base-URL placeholders and the API key, the no-key hint, and the toast after *Connect*.
  - **A provider's models:** a dialog with a switch per model. Typed-in models come after the listed ones, each with a context window, and *Add a model* is there when the provider allows it.
  - **Remove:** an alert dialog, *Remove X?*.
  - **The default and fast model:** pickers with the models that are on.
  - **Limits:** number inputs, with the `config.yaml` note.
- **Usage:**
  - **The view:** the totals, the 7, 30 and 90 days choice, and the chart of tokens per day, with input and output stacked.
  - **The tables:** *By model* and *By chat*.
  - **Notes:** the skipped chats alert and the hint for unpriced models.
- **Desktop** (Windows only): macros and recipes the agents saved.
  - **Macros:** edit the description, the parameters and the steps; delete.
  - **Recipes:** edit and delete. A recipe's text is capped, with a count.
  - **New recipe:** app, site, name and text.
  - **Removed items:** the list of what was removed.
- **Developer:** the *Developer tools* switch, with "Applies at the next start" when it differs from the running app. While developer tools are on, there are buttons to the turn inspector and the runtime panel.

## 3. The kit

Each piece is in its own new file, with headless tests in light and dark, for the keyboard, the label and right-to-left text.
- **`number.go`:** `NumberInput`, `NumberSpec` and `ReadNumber`, after `number-input.tsx`.
  - Enter or leaving the field commits, and Escape puts the value back.
  - A value below the least goes back, and the field is marked "At least N" until the next commit.
  - `Failed` puts the value back when the save fails.
- **`chart.go`:** `BarChart` and `Series`, on MyGo's `ui/chart`, after `usage-chart.tsx`. It draws stacked bars with few round ticks, a legend, and a label that names the series' values.
- **`switch.go`:** `Switch`, with the role *switch*.
- **`segmented.go`:** `Segmented` and `Choice`. It is a radio group, so the arrow keys move the choice.
- **`meter.go`:** `Meter`, for the memory used.
- **`table.go`:** `Table` and `TableColumn`: a header row, and numeric columns aligned to the end.
- **`settings.go`:**
  - the layout pieces: `SettingsLayout`, `SettingsNav`, `PageItem`, `SettingsTitle`, `SettingsSection`, `SettingsDivider`;
  - the rows: `FactRow`, `SettingRow`, `Hint`;
  - text helpers: `Subheading` and `LeftToRight`;
  - the pages' icons.
- **`link.go`:** `Link`, a button that looks like a link.
- **`tester_settings.go`:** more keys and roles for the tests.
- **Icons:** arrow-left, key-round, server, folder-open, folder-input, activity, sun, moon and monitor, from Lucide like the rest.

## 4. The stores

- **`state.Pages`** (`state/pages.go`): what the pages read and write beyond `state.Settings`.
  - **Reads:** usage per range, desktop items, presets, a provider's models, the user's memory, the folder check and the version.
  - **Writes:** macro and recipe edits, the user's memory, and the folder picker.
  - **How calls run:** off the UI thread, with the result posted back.
- **`state.API`:** gets `Pages` and `Version`; when they are nil, the Settings and System services provide them, as the app's services do. The edit to `stores.go` is additive.
- **`statetest`:** the fake has these calls too, with seeded usage, desktop items, presets and memory.

## 5. Tests

- **Ported from the frontend:** in `settings/logic`, `format.test.ts`, `provider-choice.test.ts` and `memory.test.ts`. In `settings`, `settings-view.test.ts` (the open project in the page list) and the scope-link test of `memory.test.ts`.
- **The pages:** 19 tests in `settings_test.go`, most in light and dark. They cover:
  - the page list and *Back*;
  - the memory editor, its conflict and a new section;
  - Appearance and Developer;
  - the approval level and the data folder dialog;
  - *Copy* in About;
  - the provider rows, the models dialog, *Remove* and *Add provider*;
  - the alias pickers and the limits;
  - Usage, Desktop and a macro edit.
- **The kit:** `TestNumberInput`, `TestReadNumber`, `TestBarChart`, `TestFewTicks`, `TestSwitch`, `TestSegmented`, `TestMeter`, `TestTable` and `TestSettingsLayout`.
- **Checks:** `go vet ./...` and `go test ./...` pass.

## 6. Screenshots

`TestShots` saves every page in light and dark, plus the models dialog and *Add provider*, when `JENAB_SETTINGS_SHOTS` names a folder. The shots of this run are outside the repo.

## 7. The live check

`TestLiveConnectAndPickAModel` runs only with `JENAB_LIVE_SETTINGS=1` and `GEMINI_API_KEY`.
- **The setup:** the real services, with the key in an in-memory keyring as the bench does, and a temporary config and registry.
- **The steps:** the test drives the native Settings: *Add provider*, *Google Gemini*, the API key, *Connect*. Then it picks `gemini-3.5-flash-lite` as the default and the fast model.
- **The result:**
  - the window said "Google Gemini connected · 2 models on";
  - `config.yaml` has `default` and `fast` set to `gemini/gemini-3.5-flash-lite`;
  - the key was never printed or written.

## 8. Notes and follow-ups

- **Merging with the other native tickets:**
  - `IconRuntime` uses `activity.svg`, the same file as TASK-058's `IconActivity`, with the same bytes. After both merge, one of the two names can go.
  - TASK-057 adds its own `KeyBackspace`, so this ticket doesn't.
  - After TASK-057 merges, its screens can use `FactRow`, `Segmented`, the settings layout and `MemoryEditor`.
  - SPEC 5.12's "What it has now" still says Settings comes with TASK-055. TASK-056 to 058 change the same line, so it is best updated once, after the merges.
- **The chat:**
  - The native chat's Connect card (`chat/cards.go`) can use `settings.ConnectForm`.
  - The chat's `logic.FirstPick` is older than TASK-052; the settings' `logic.FirstPick` has the new rule.
- **The chart:** day labels are "Jan 2" in English, not in the user's region format, and the chart is drawn left to right in every language.
- **MyGo:** `MaxHeightPercent` on a scroll inside a dialog left the dialog empty, so the models dialog uses a fixed `MaxHeight(440)`. This may be worth a MyGo issue.
- **Project settings:** the test that its title is the project's name belongs to TASK-057.
