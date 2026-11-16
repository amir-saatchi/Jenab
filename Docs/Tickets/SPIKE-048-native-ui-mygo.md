# SPIKE-048 — A native UI with MyGo
**Type:** Spike
**Status:** Done (go, decided 2026-11-16)
**Gate:** 3
**Needs:** none

## Question
Jenab's window is a React app in WebView2, through Wails v3 (SPIKE-003). [MyGo](https://github.com/egoist/mygo) (MIT, pure Go, no cgo) draws a native UI on the GPU with no webview. Can Jenab's desktop UI move to MyGo, with MyGo's own widgets and no other UI library, and is the gain worth rewriting the frontend?

The spike answers this with one real slice, the chat, built in the repository next to the Wails app. The Wails app keeps working and shipping until a go decision, and after it until the native app has every screen.

## What we know already
From a separate test app on MyGo v0.7.4 with Go 1.27.3, on Windows 11:

- **Persian text works.** MyGo shapes text with `go-text/typesetting` and runs the bidi algorithm. Mixed Persian and English lines, a ZWNJ (می‌شود), Persian digits with `٫`, and an English paragraph that wraps onto Persian text rendered line for line as in Chromium (Edge). Code blocks stay left to right.
- **Tabs in code blocks are dropped.** Indentation with spaces renders; indentation with tabs shows nothing. Go code from a model is indented with tabs. To fix in the kit (expand tabs) and report upstream.
- **Streaming Markdown is built in.** `markdown.Stream` parses only the appended text and shows the unfinished end as it will most likely become; `ui.List` with `FollowEnd` keeps the newest message in view until the user scrolls up.
- **Memory and start-up.** One process, ~109 MB private memory and the first frame after 0.86–0.98 s. Jenab's Wails build measured the same way: 7 processes (6 of them WebView2), ~330 MB private. SPIKE-022 measured 245 MB idle and 338–402 MB with a chat open, and 0.83 s to first paint. The test app is small, so these numbers are not a like-for-like comparison yet.
- **Widget coverage.** MyGo's `ui` package has most of what the 41 shadcn components in `frontend/src/components/ui` do (table below). What it lacks is small or is Jenab's own chat UI, which is custom in React too.
- **Not production-ready yet.** MyGo is pre-v1 (v0.7.x) and changes fast; it needs Go 1.27.3, and Jenab is on Go 1.26.

## Why the move is smaller than the frontend
The Go side is already apart from the UI:

- The six services in `internal/app` (Project, Chat, Settings, Bucket, System, Dev) are 79 plain Go methods. A native UI calls them directly; there are no bindings and no generated TypeScript.
- Every live update goes through one `Publisher` with one `emit` function (`internal/app/events.go`): `chat:delta`, `chat:part`, `chat:status`, `project:notice`, `project:activity`, `project:export` and `app:open`. A native `Publisher` applies them to the UI state on the window's thread.
- Only `app.go`, `events.go` and `dark_windows.go` in `internal/app` import Wails.

So the work is `frontend/src`: about 15,500 lines without tests.

| Folder | Lines | Becomes |
|---|--:|---|
| `components/ui` | 4,900 | the kit (`internal/native/kit`) |
| `chat/` | 3,660 | the chat screen |
| `shell/` | 3,070 | the app frame: rail, sidebars, dock, files panel, dialogs |
| `settings/` | 2,000 | Settings |
| `state/` | 1,150 | Go stores (`internal/native/state`) |
| `dev/` | 940 | developer tools (8.4) |
| `lib/`, `hooks/` | 520 | plain Go functions, or nothing (below) |

Some React code has no native counterpart, because MyGo already does it: per-frame token buffering (`state/stream.ts`, SPEC 5.8), memoised Markdown blocks (`lib/blocks.ts`), `dir="auto"` handling (`lib/dir.ts`), and the `/objects/...` asset route (SPEC 4.5), since a native view loads bucket objects directly.

## Architecture
```
cmd/desktop            Wails app (unchanged)          cmd/desktop-native   MyGo app
        \                                                   /
         internal/app services (Project, Chat, Settings, Bucket, System, Dev)
                                      |
                       internal/native/state   Go stores; the native Publisher
                                      |         applies events here, on the UI thread
                       internal/native/kit     Jenab's components on MyGo's ui
                                      |
          internal/native/{shell,chat,settings,files,dev}   screens
```

- **Screens use only the kit and the stores.** They never import `github.com/egoist/mygo/ui` directly; a test checks the imports. The kit is where Jenab's theme (5.11), fonts, RTL defaults and fixes for MyGo bugs live, so each is done once, and a change in MyGo's API before v1 touches only the kit.
- **The stores are plain Go**, tested without a window, like `state/*.test.ts` today. Their tests are ported with them and are the spec.
- **Same repository, same `main`.** No long-lived migration branch: each ticket gets its own short branch or worktree, as now. Backend changes land once and both apps get them.

## Component inventory
Uses = files in `frontend/src` that import the component.

| shadcn component | Uses | Native source |
|---|--:|---|
| button | 28 | MyGo `Button`, `PrimaryButton` |
| spinner | 19 | MyGo `Spinner` |
| badge | 11 | MyGo `Badge` |
| alert | 11 | **kit**: a box with icon and tone |
| field, label | 11 | MyGo `Field`, `Fieldset`, `Form` |
| tooltip | 8 | MyGo `Element.Tooltip` |
| dialog | 8 | MyGo `Modal` |
| sidebar | 7 | MyGo `Sidebar`, `SidebarSection`, `SidebarItem` |
| input, textarea | 9 | MyGo `TextInput`, `TextArea` |
| empty | 7 | **kit**: icon, title, text |
| toggle-group, toggle, button-group | 7 | MyGo `ToggleGroup`, `Segmented`, `Toggle` |
| dropdown-menu | 6 | MyGo `MenuButton`, `PopoverMenuButton`, `Element.ContextMenu` |
| input-group | 5 | **kit**: a row of `TextInput` and add-ons |
| item, card | 8 | **kit**: styled boxes |
| alert-dialog | 4 | MyGo `AlertDialog` |
| table | 3 | MyGo `Table` |
| switch, checkbox | 4 | MyGo `Switch`, `Checkbox` |
| sheet | 3 | **kit**: `Overlay` sliding in from a side |
| separator | 3 | MyGo `Divider` |
| select | 3 | MyGo `Select`, `Combobox` |
| collapsible | 3 | MyGo `Collapsible` |
| skeleton | 2 | **kit**: a pulsing box |
| scroll-area | 2 | MyGo `Scroll` |
| progress | 2 | MyGo `Progress`, `Meter` |
| message-scroller | 2 | MyGo `List` with `FollowEnd` |
| marker, bubble, message, attachment | 6 | **kit (chat)**: thread rows, user bubble, attachment chip |
| tabs | 1 | MyGo `Tabs` |
| sonner | 1 | MyGo toasts (`ToastBase`, `ToastViewportBase`) |
| questionnaire | 1 | **kit (chat)**: the `ask_user` form, from `Field`, `RadioGroup`, `CheckboxGroup` |
| chart | 1 | MyGo `ui/chart` |
| breadcrumb | 1 | MyGo `Breadcrumbs` |
| kbd | 0 | **kit**: a small styled box |
| Markdown (`react-markdown`, `remark-gfm`, `rehype-highlight`, Typeset) | – | MyGo `ui/markdown` and `ui/editor` (highlighting); Typeset's look as a `markdown.Theme` |

Files and lists that React doesn't have as components but MyGo does: `Tree` (files panel), `Split` (dock, 5.12), `SearchField`, `TokenField`.

## Stores
| `frontend/src/state` | Go store | Note |
|---|---|---|
| `nav.ts` | `state.Nav` | the place in the main area (5.12) |
| `projects.ts`, `chats.ts`, `events.ts` | `state.Projects`, `state.Chats` | chat lists with *Waiting*; status events apply here; reload timers become one goroutine |
| `thread.ts` | `state.Thread` | sequence numbers and the missed-event reread (Q32) port as they are |
| `stream.ts` | part of `state.Thread` | no frame buffering; the Publisher coalesces deltas every ~16 ms as today (SPEC 5.8) |
| `settings.ts` | `state.Settings` | the save queue ports as it is |
| `dock.ts`, `ui.ts`, `files.ts` | `state.Shell` | |
| `quote.ts` | `state.Quote` | `QUOTE_MAX` already matches `chat.QuoteMax` |

## Scope
Windows 11 only. Everything is added next to the Wails app; nothing in its behaviour changes.

**Part A: foundation**
1. **Go 1.27.3.** Move `go.mod` and the toolchain. The Wails app still builds, all tests pass, and the bindings regenerate unchanged.
2. **Services without Wails.** Move the Wails wiring (`app.go`, the `RegisterEvent` calls in `events.go`, `dark_windows.go`) into its own package, so that the services and the `Publisher` build without importing Wails. Move the single-instance lock and shutdown from `cmd/desktop` into an internal package both apps use.
3. **`cmd/desktop-native`.** One window with saved size and place, close hides it and the app keeps running, a second launch shows the running window, and desktop notifications whose click opens the chat (`app:open`). The services are built as in the Wails app, and a native `Publisher` sends events to the stores through `Window.Update`.
4. **Stores.** Port the stores and their tests (table above).
5. **Kit, first part.** Theme tokens from `index.css` (light, dark, System; tones 5.11), the Vazirmatn and Geist fonts through `ui.RegisterFont`, and the components the chat needs. MyGo reads TrueType and OpenType, not the WOFF2 files in `node_modules`, so the fonts come as TTF from their upstream releases (both OFL). Check that MyGo handles their variable weights.

**Part B: the chat slice**

6. **Frame.** The rail, the chat sidebar and the main area (5.12), with no dock.
7. **Thread.** Messages, `markdown.Stream` while a part streams and `markdown.View` once it is done, thinking parts, tool and approval cards, the `ask_user` form, marker rows, and *Stop*, *Retry*, *Rewind* and *Undo* (8.3).
8. **Composer.** Text, the quote chip, attachments, the model picker, and *Send* and *Stop*.
9. **Checks.** Measure on the same laptop as SPIKE-022, with a fake provider streaming at a fixed rate, and record each value next to the Wails app's:
   - frame times while streaming at 100 and 1,000 tokens a second, and the time from a key press to the letter on screen in the composer;
   - opening a chat with 200 messages;
   - start-up to the first frame, cold and warm;
   - private memory idle, with a chat open, and after streaming 10 answers (all processes);
   - text: 20 fixed samples (Persian, English, mixed, quotes, code with tabs, tables, lists), rendered in the thread and the composer, next to Chromium's rendering;
   - Narrator: does it read the messages, the buttons and the composer, and announce a finished answer;
   - copying a selection across messages, as text and as Markdown.

   Then one live answer from a reference model.
10. **Cost.** Record the hours the slice took, the size of the kit (components and lines), and an estimate for the rest from that.

## Rules
- **MyGo only.** `github.com/egoist/mygo` at a pinned version is the one new dependency. No Onevra or other component library. Code from an MIT library may be copied into the kit with its notice in `NOTICE`.
- **Screens use only the kit and the stores** (above). A MyGo bug is fixed or worked around in the kit and reported upstream, with the issue linked in the code.
- **Each kit component has a headless test** (`ui.NewTester`): light and dark, keyboard focus, an accessible label, and right-to-left text in it.
- **Right-to-left text, not a mirrored layout.** The SPEC supports right-to-left text only, not a mirrored interface (5.8 and 11). The window stays left to right; paragraphs, inputs and quotes take the direction of their text. MyGo's layout mirroring stays off.
- **The Wails app is not changed** apart from the moves in items 1 and 2, which must not change its behaviour.
- **Ask before downloading** fonts or anything else, and record the version and SHA-256.
- **Models:** the fake provider for every measurement. One live check with `gemini-3.5-flash-lite` (`GEMINI_API_KEY`). Ask before any other key.

## Done when
- Part A and Part B work, with the checks in item 9 recorded, and what failed.
- A go or no-go. Go needs, measured against the Wails app on the same laptop:
  - Persian and mixed text match Chromium on all 20 samples, or the differences are fixed in the kit;
  - streaming at 1,000 tokens a second keeps frames under 33 ms and the key-to-screen time under 50 ms (the SPIKE-022 targets);
  - opening a 200-message chat and start-up are no slower than Wails;
  - private memory with a chat open is at most half of the Wails app's;
  - Narrator can use the chat;
  - the estimate for the rest is one the user accepts.
- If go: tickets for the rest (below), and the SPEC and design changes: 2.8 (updater), 4.5 (the `/objects` route), 5.8, 5.10 (blocks as React components), 5.11 and 5.12, Gate-2's technical design, `DEVELOPMENT.md` and `CODE-OUTLINE.md`.
- If no-go: delete `cmd/desktop-native` and `internal/native`; keep items 1 and 2 if they are still useful.

## After a go (tickets, not part of this spike)
1. Settings: the 11 files in `frontend/src/settings`, the usage chart.
2. Files panel, file view and the dock (5.9, 5.12).
3. Projects: new project, project settings, the welcome page, the folder warning, private hosts, exports.
4. Developer tools: the turn inspector and *Runtime* (8.4).
5. Pages and pipelines (5.9, 6.10), built native only.
6. Updater: MyGo's signed updates instead of Wails `pkg/updater` (2.8), and the installer.
7. macOS and Linux builds, when CI runs again.
8. Remove the Wails app, `frontend/` and Bun from the build.

From the go decision on, new UI work goes into the native app first, so nothing is built twice for longer than needed.

## Result

**Part A** (2026-11-16, MyGo v0.7.4): items 1 to 5 are done, and the session report is [reports/SPIKE-048-partA-report.md](reports/SPIKE-048-partA-report.md). No model requests were spent.

- **Items 1 and 2:** Go 1.27.3; the services and the `Publisher` build without Wails. The Wails app and its bindings are unchanged.
- **Item 3:** `cmd/desktop-native` starts and keeps one instance. A smoke run checked both. Hiding on close, the saved size and place, and notification clicks are wired but not yet checked live.
- **Items 4 and 5:** the stores (54 tests) and the kit's first part (the components the chat needs, 18 headless tests; a test keeps `mygo/ui` inside the kit). Geist and Vazirmatn draw variable weights.
- **MyGo v0.7.4 draws tabs as nothing.** The kit expands them, and its code blocks copy the original code. A tab size came upstream in [#1217](https://github.com/egoist/mygo/pull/1217), after v0.7.4. Part B should decide on moving to v0.9.1 first.

**Part B** (2026-11-16, MyGo v0.9.1): items 6 to 10 are done, and the session report is [reports/SPIKE-048-partB-report.md](reports/SPIKE-048-partB-report.md).
- **What ran:** native and Wails were built from the same commit and measured on the SPIKE-022 PC, with the fake model and every data folder in a temporary folder.
- **Model requests:** one, the live check with `gemini-3.5-flash-lite`.
- **Checked by the user (2026-11-16):** Narrator read the chat correctly, and a notification click opened its chat.

| Measured | Native (MyGo) | Wails |
|---|---|---|
| Start to the shell drawn (3 starts) | 540, 177, 185 ms | 823, 760, 805 ms |
| Opening the 200-message chat, first rows / all rows (median of 5) | 14 / 37 ms | 56 / 196 ms |
| 1,000 tok/s: frames | 59.8 fps, work max 17.8 ms, 0 % over 33 ms | 59.9 fps, max 18.7 ms, 0 % over 33 ms |
| 1,000 tok/s: key to screen p50 / p95 / max | 11.7 / 18.2 / 24.1 ms | 23.0 / 35.4 / 51.1 ms |
| 100 tok/s: key to screen p50 / p95 / max | 12.0 / 17.1 / 76.9 ms | 15.5 / 26.6 / 33.9 ms |
| Private memory, all processes: idle | 177 MB | 351 MB |
| … with the 200-message chat open | 186 MB | 531 MB |
| … peak with 10 answers (100 / 1,000 tok/s) | 225 / 418 MB | 654 / 770 MB |
| Text samples like Chromium | 19 of 20 (16 before the direction fix) | — |

**Go criteria:**

| Criterion | Result |
|---|---|
| Persian and mixed text match Chromium on all 20 samples, or the kit fixes the differences | **Not fully met.** 19 of 20 match after the kit's fixes. Those fixes are code tab size 2, ten clipped icons, and direction for lists and tables. A Persian table's columns and Persian list markers now run right to left (14 to 16), through MyGo's `Options.Block`. Open: MyGo on Windows misplaces or drops some Arabic harakat inside words (19), which the kit can't fix: [egoist/mygo#1925](https://github.com/egoist/mygo/issues/1925). |
| At 1,000 tok/s, frames under 33 ms and key to screen under 50 ms | **Met.** Frame work max 17.8 ms; key to screen max 24.1 ms (Wails: 51.1 ms). At 100 tok/s one frame took 62.9 ms (0.1 %). |
| Opening a 200-message chat and start-up no slower than Wails | **Met.** About 4 times faster to open and 4 times faster to start. |
| Private memory with a chat open at most half of the Wails app's | **Met.** 186 MB against 531 MB, 35 %. |
| Narrator can use the chat | **Met.** The user listened on 2026-11-16: Narrator read the rail, the chat list, each message ("You said" / "Jenab said" with its text), its buttons and the composer, and said "Answer finished". |
| The user accepts the estimate | **The user's call.** About 4,900 frontend lines remain, the size of this slice: 5 to 8 agent hours for the screens. Pages and pipelines, the updater and installer, and macOS and Linux come on top. |

**Recommendation:** a conditional go.
- **Why:** performance and memory are clear wins. The text gap is narrow: the three kit gaps are fixed, and what's left is an upstream bug that rarely touches Persian.
- **Before deciding:**
  1. ~~The kit takes direction for lists and tables, and the samples pass again (19 of 20).~~ Done: 19 of 20 match.
  2. ~~The user spends ten minutes with Narrator in the chat and clicks one notification.~~ Done: both correct.
  3. ~~The harakat issue goes to MyGo, with the user's OK to post.~~ Done: [egoist/mygo#1925](https://github.com/egoist/mygo/issues/1925).
- **If any of these fails:** no-go, as the Done-when section says.

**Decision: go** (the user, 2026-11-16). Part B, with Part A, is merged into `main`. MyGo fixed the harakat bug in [#1929](https://github.com/egoist/mygo/pull/1929), tested on Windows before merging; TASK-054 moves to the release that has it.

The tickets for the rest:
- [TASK-053](TASK-053-native-ui-docs.md): the SPEC and design docs for the native UI (first);
- [TASK-054](TASK-054-native-chat-gaps.md): the native chat's gaps, and MyGo with the harakat fix;
- [TASK-055](TASK-055-native-settings.md): Settings;
- [TASK-056](TASK-056-native-files-and-dock.md): the files panel, the file view and the dock;
- [TASK-057](TASK-057-native-project-screens.md): the project screens;
- [TASK-058](TASK-058-native-developer-tools.md): developer tools;
- [TASK-059](TASK-059-native-updater-installer.md): updates and the installer;
- [TASK-060](TASK-060-native-macos-linux.md): macOS and Linux, when CI runs again;
- [TASK-061](TASK-061-remove-wails.md): the switch, and removing Wails, `frontend/` and Bun.

Pages and pipelines (5.9, 6.10) get no port ticket: they are built in the native app when their phases come. Until TASK-061, the React frontend gets bug fixes only.
