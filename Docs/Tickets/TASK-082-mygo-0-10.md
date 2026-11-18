# TASK-082 — Move to MyGo v0.10.0
**Type:** Task
**Status:** Done
**Gate:** 3
**Needs:** TASK-054

## Goal
TASK-054 item 3. MyGo v0.10.0 (released 2026-11-17) has [egoist/mygo#1929](https://github.com/egoist/mygo/pull/1929), the fix for [#1925](https://github.com/egoist/mygo/issues/1925): Arabic and Persian marks are drawn where shaping places them. Jenab is on v0.9.1. The user approved the download on 2026-11-18.

## Scope
1. **The update.** Move `github.com/egoist/mygo` in `go.mod` from v0.9.1 to v0.10.0, the `tool` line with it, and tidy. Record in the report what else `go.sum` pulls in. Per DEVELOPMENT.md, the move should touch only `internal/native/kit` and `cmd/desktop`; say if anything else had to change.
2. **Sample 19 (harakat).** Picture the 20 samples again with `jenab-bench -samples -native`, as in TASK-054's report: 1280×800, 125 % scale, dark theme. Sample 19 should now match Chromium: the fatha sits on ک and the kasra under ر shows. Say for each sample whether it still matches. Keep the screenshots in `bin/`, uncommitted, since they hold a user-name path.
3. **v0.10.0's changes in behavior.** Check each against the kit and the app, and fix or note it:
   - A triple click selects a paragraph, not one wrapped line: selectable text and the composer.
   - Attached popups never grow taller than the window: the kit's selects, comboboxes, popovers and menus. A long list should scroll inside.
   - A finger drag scrolls on Windows touchscreens. Note it; no change expected.
   - A pasted lone `\r` breaks the line in a multi-line input.
4. **The workarounds for v0.9.1.** The code names MyGo gaps "still so in v0.9.1" (`grep -rn "v0\.9\." internal cmd`): the sidebar's accent fill, the split, the tester's labels, the updater's file swap, audio and video in the file view. Check each on v0.10.0. Drop a workaround MyGo no longer needs, and update the version in the comments of those it still needs.
5. TASK-054 item 4's open points (the sidebar's accent, links underlined only on hover, the table's frame and column lines): say which v0.10.0 still has.

## Rules
- No other downloads without asking. No model requests.
- Don't edit Docs/. Put the version line for DEVELOPMENT.md (it pins v0.9.1 in two places) and anything for the SPEC in the report.
- Live checks use a scratch profile, with no system dialogs.

## Done when
- `go build ./...`, `go vet ./...`, `go vet -tags bench ./cmd/desktop` and `go test ./...` pass on v0.10.0, the kit's pixel tests too.
- Sample 19 matches Chromium, or the report says exactly how it still differs.
- The report covers items 3 to 5.

## Result
Done. MyGo is v0.10.0; `go.sum` changed only MyGo's lines, and the code only in comments.
- **Sample 19** matches Chromium, in messages and the composer; all 20 samples have the same text. The other 19 are pixel-identical to v0.9.1's run.
- **v0.10.0's changes in behavior** (paragraph triple click, popups that fit the window, finger scrolling, a lone `\r` on paste) needed no change; the kit's pickers now fit short windows.
- **The five workarounds** are all still needed; their comments say v0.10.0. The file view's reason was wrong: MyGo plays audio and video but has no player controls. An in-app player is TASK-083.
- **TASK-054 item 4's points** remain: links not underlined, the table's frame and column lines, the sidebar's accent.

DEVELOPMENT.md, Gate 2 and SPEC 5.12 now say v0.10.0. Report: [reports/TASK-082-report.md](reports/TASK-082-report.md).
