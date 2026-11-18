# TASK-082 report: MyGo v0.10.0

**Branch:** `claude/task-082-mygo-0-10-23006b`, commit bcbc8b4. Not merged, not pushed.

## 1. The update

- `go.mod`: `github.com/egoist/mygo` v0.9.1 → v0.10.0. The `tool github.com/egoist/mygo/cmd/mygo` line has no version of its own; it follows the `require`, so it now runs v0.10.0's CLI.
- `go mod tidy` changed nothing else.
- `go.sum` swaps MyGo's two lines and nothing more. v0.10.0's `go.mod` has the same hash as v0.9.1's (`h1:Hw9sxd…`): the same `go 1.27.3` and the same dependencies, so nothing else was downloaded.
- No Go API change touched the code. Besides `go.mod`/`go.sum`, the commit changes only comments: four in `internal/native/kit` and `cmd/desktop`, and one in `internal/native/files/view.go` (item 4). That file is outside the two folders DEVELOPMENT.md names, but only its comment changed.

## 2. The 20 samples

**The run:**
- `jenab-bench -samples`, run on 2026-11-18. The window was 1280×800 at 125 % scale (120 DPI), and Windows was in dark mode. (The bench has no `-native` flag: `-samples` pictures the native app.)
- The bench app was built from this branch.
- I made a second run with a v0.9.1 build of the same code, using the old `go.mod` as `-modfile`. Then I compared the two runs pixel by pixel. There is no Chromium screenshot left from TASK-054, so the comparison is with v0.9.1, which TASK-054 compared with Chromium.
- The screenshots are uncommitted, in `bin/bench/samples/` (v0.10.0) and `bin/bench-091/samples/` (v0.9.1).

**Chromium reference for sample 19:**
- I wrote a page with the kit's own Vazirmatn and Geist fonts, at 14 px × 1.25, and opened it in the app's Chromium pane, served from loopback. I also drew a large copy of the line headless on both MyGo versions.
- Chromium and v0.10.0 draw the same marks in the same places:
  - the fatha on ک's head in کَسرِه, and the kasra under its ر;
  - the fatha over ف in فَتحه;
  - the kasra under إ in إِنَّ;
  - the sukun and the final kasra in الْعُسْرِ.
- v0.9.1 got these wrong:
  - It drew the fatha into ک's stroke, so the word read as «گسره».
  - It put a damma-like mark on ف.
  - It dropped the kasra under ر and under إ.

**Per sample, v0.10.0 against v0.9.1 (which matched Chromium except 19):**

The comparison covers the chat, composer and side panel, x ≥ 390 px in the 1600×1000 shot.

| # | Sample | Result |
|---|---|---|
| 1 | Persian | Same: 1 px, the composer's caret |
| 2 | English | Same: identical pixels |
| 3 | Persian with English | Same. Besides the caret, 3 pixels of anti-aliasing differ slightly at the line's end. You can't see it. |
| 4 | English with Persian | Same: identical pixels |
| 5 | Digits and punctuation | Same: identical pixels |
| 6 | ZWNJ and joining | Same: identical pixels |
| 7 | Brackets in RTL | Same: identical pixels |
| 8 | Heading, Persian | Same: identical pixels |
| 9 | Persian quote | Same: identical pixels |
| 10 | English quote with Persian | Same: identical pixels |
| 11 | Go code with tabs | Same: only the caret |
| 12 | Inline code | Same: identical pixels |
| 13 | English table | Same: identical pixels (the frame and column lines remain, item 5) |
| 14 | Persian table | Same: only the caret (frame and column lines remain, item 5) |
| 15 | Persian list | Same: identical pixels |
| 16 | Mixed numbered list | Same: identical pixels |
| 17 | Nested list | Same: identical pixels |
| 18 | Link and email | Same: identical pixels (links still not underlined, item 5) |
| 19 | Harakat | **Now matches Chromium**, in the message and in the composer: 641 px changed, all at the marks |
| 20 | Emphasis and emoji | Same: identical pixels |

**Outside the chat area, two things differ between the runs; neither comes from MyGo:**
- **The sidebar's order of chats.** The seeded chats (Call 1–8, Sample 19/20) share the same last-active time, so their order changes from run to run.
- **The status bar's free memory.**

**Summary:** all 20 samples now have the same text as Chromium. The style differences left are the ones TASK-054 listed: links not underlined, the table's frame and column lines, the Persian table's place, and the code block's corner *Copy*.

## 3. v0.10.0's changes in behavior

I checked these with headless probes in the kit (`kit.NewTester`), on v0.10.0 and again on v0.9.1 through the old `go.mod`. The probes were temporary and are not committed.

| Change | Kit or app | v0.9.1 | v0.10.0 | Action |
|---|---|---|---|---|
| Triple click selects a paragraph | Selectable text (`kit.Selectable` around two paragraphs, 240 px wide so the first wraps): triple click on a middle line, Ctrl+C | Copied one wrapped line: "narrow column of text, so a triple " | Copies the whole first paragraph and not the second | None. The chat's selectable messages get it. |
| | Composer (`ComposerText`): triple click on the first paragraph's second wrapped line, type "X" | Replaced one wrapped line | Replaces the paragraph and its line break: "XSecond paragraph." | None |
| Attached popups fit in the window | `kit.Picker`, 40 options, windows 600, 260 and 160 px tall | Popup 330 px tall at every height: it ran past the bottom of a 260 px and a 160 px window, and End's option was off-window | Popup bottoms are 374 (600 px window), 256 (260) and 156 (160). End scrolls Option 39 into view inside the popup. | None. The kit's own cap, `MaxHeight(320)`, still applies in tall windows. |
| | `kit.MenuButton` (`PopoverMenuButton`), 30 items, windows 600 and 200 px | Already fit: 548 and 148 px tall | Same | None |
| | Comboboxes and popovers | — | — | The kit wraps neither. The screens get popups only through `Picker`, `MenuButton`/`MoreButton` and the title bar's menu (also a `PopoverMenuButton`). |
| Finger drag scrolls on Windows touchscreens | `kit.Scroll`, 60 lines: `FingerDown`/`FingerMove`/`FingerUp` up 200 px | `Tester` has no finger input | The list scrolls 190 px | None. No touchscreen here, so not checked on hardware. `internal/windows/pointer.go` now turns touch pointers into finger events. |
| A lone `\r` pasted breaks the line | Pasted "a\rb\r\nc" into `TextInput`, `TextArea` and `ComposerText` | Single line: "a b c". Multi-line: "a\rb\nc" (the `\r` kept) | Single line: "a b c". `TextArea` and composer: "a\nb\nc" | None. No app code reads `\r` in input. The only CR handling, in `state/dev.go`, turns `\r\n` into `\n`. |

## 4. The workarounds for v0.9.1

`grep -rn "v0\.9\." internal cmd` finds five. All five are still needed; their comments now say v0.10.0.

| Where | Gap | On v0.10.0 | Change |
|---|---|---|---|
| `kit/sidebar.go` | MyGo's sidebar fills the chosen, focused item with `Theme.Accent`, and has no setting for it | `ui/sidebar.go` is byte-identical to v0.9.1's: it still does `item.Background(t.Accent)` | "still so in v0.10.0" |
| `kit/split.go` | MyGo names the split's line "Divider", with no setting | `ui/split.go` is byte-identical: `Label(c.texts().Divider())` | "still so in v0.10.0" |
| `kit/tester.go` | `Tester.Texts`/`Find`/`HasText` don't see a text field's text | A probe shows `HasText` and `Find` still false for a `TextInput`'s value | "still so in v0.10.0" |
| `cmd/desktop/appsentry.go` | The updater swaps the files but leaves the Installed apps entry's `DisplayVersion` | Only the NSIS installer (`cmd/mygo/nsis.go`) writes the entry. Neither `updater.go` nor `internal/update` touches the registry. | "v0.9.1, still so in v0.10.0" |
| `files/view.go` | Audio and video open with the system app, because "v0.9.1 has no player" | **The premise was wrong.** MyGo plays media since at least v0.9.1: `ui.Video` with `ui/video` (Media Foundation on Windows, sound included), and the `audio` package (WAV, MP3, Vorbis, FLAC, AAC/ALAC). It has no player controls, though: no play/pause or seek bar. Building a player in the kit is new UI, not the removal of a workaround, so I left it. | Comment corrected: MyGo plays media but has no player controls, and the kit has no player yet (v0.10.0) |

**For the SPEC (not edited):**
- **5.12, line 768:** "MyGo has no audio or video player yet" is not accurate. Suggested wording: "MyGo plays audio and video but has no player controls, and the kit has no player yet, so the app offers *Open with the system app* for those too."
- **Line 1113:** "the kit can't rename it in v0.9.1" → "in v0.10.0".

A player in the file view (SPEC line 1138 says audio and video play) would be its own ticket, if wanted.

## 5. TASK-054 item 4's open points on v0.10.0

All three remain. Samples 13, 14 and 18 are pixel-identical to the v0.9.1 run in the chat area, and the MyGo code behind them is unchanged: `ui/markdown/theme.go`, `ui/markdown/table.go` and `ui/richtext.go` are byte-identical.

- **The sidebar's accent:** still filled with `Theme.Accent` on focus (see item 4). The kit's fix stays, and `TestSidebar` passes.
- **Links underlined only on hover:** still so. The markdown theme has `Link` and `LinkWeight` but no underline setting.
- **The table's frame and column lines:** still so. `markdown/table.go` draws `Border(1, TableBorder)` around the grid and a start border on each cell after the first; there is no row-lines-only option.

The two upstream requests from TASK-054 (a link underline setting, a row-lines-only table border) still apply. They are not filed.

## For DEVELOPMENT.md and other docs (not edited)

- `Docs/DEVELOPMENT.md` line 14: `- MyGo comes through \`go.mod\`, pinned (v0.10.0), and its CLI is a \`tool\` there, so \`go tool mygo\` needs no install.`
- `Docs/DEVELOPMENT.md` line 157: `- MyGo is pinned in \`go.mod\` (v0.10.0). It is pre-v1 …` (rest unchanged).
- `Docs/Gates/Gate-2-Technical-Design.md` line 72 also says v0.9.1. v0.10.0 still needs Go 1.27.3.

## Checks

On v0.10.0, with the probes removed:
- `go build ./...`: passes.
- `go vet ./...`: passes.
- `go vet -tags bench ./cmd/desktop`: passes.
- `go test -count=1 ./...`: passes, the kit's pixel tests included (43 packages ok).

**The rules:**
- No model requests.
- The only download was MyGo v0.10.0.
- The bench used its scratch profile, and nothing opened a system dialog.
- The Chromium reference page was served from 127.0.0.1 out of `bin/` and stopped afterwards. The ignored `.claude/launch.json` is back as it was.
