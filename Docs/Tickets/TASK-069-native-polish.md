# TASK-069 — Small native fixes: digits, text direction, tooltips, old comments
**Type:** Task
**Status:** Done
**Gate:** 3
**Needs:** TASK-061

## Goal
Low-severity findings from the review of the native app (2026-11-17).

## Scope
1. **Persian and Arabic digits:** number fields (`internal/native/kit/number.go`, `ReadNumber`) reject `۱۲` and `١٢`. Accept them as the same numbers.
2. **Text direction:** `RightToLeft` (`kit/dir.go`) counts only Latin up to U+024F, Greek and Cyrillic as strong left-to-right. Text that starts with Vietnamese (Latin Extended Additional), Chinese, Japanese, Korean, Thai, Devanagari, Georgian or Armenian and then has Persian is taken as right to left. Use the Unicode bidi class (`golang.org/x/text/unicode/bidi`, already a dependency) for the first strong character. U+00D7 and U+00F7 aren't letters.
3. **Table tooltips:** `kit.Table` shows a tooltip only for the first column, so cut values in the others (model names, paths) can't be read. Every cut cell gets one, as its doc comment says.
4. **Old text:**
   - Comments that still describe Wails ("Wails mounts…", "As Wails marshals…") in `internal/app` (`bucket.go`, `bucket_read.go`, `events.go`, `upload.go`, `errors_test.go`, `events_test.go`), `internal/bucket/serve.go` and `internal/project/events.go`.
   - `README.md` still says "There is no app to use yet". Main-Session writes the README; the session lists what it should say.
5. **Small robustness:** `kit.BarChart` with a nil `format` panics; `NumberInput` can commit the same value twice (Enter, then blur).

## Done when
- Tests for 1, 2, 3 and 5.

## Result
Done.
1. Number fields read Persian (۱۲) and Arabic (١٢) digits as the same numbers.
2. `RightToLeft` takes the first strong character by its Unicode bidi class (`x/text/unicode/bidi`), so Vietnamese, Chinese, Japanese, Korean, Thai, Devanagari, Georgian and Armenian are left to right. × and ÷ are passed over.
3. `kit.Table` gives a tooltip to any cell whose text is cut, in any column. A cell that fits has none.
4. Comments about Wails and its frontend in `internal/app`, `internal/bucket/serve.go` and `internal/project/events.go` now describe the native app. Only comments changed. The report lists what README.md should say; Main writes it.
5. `BarChart` with a nil `format` writes plain numbers. `NumberInput` commits a value once while its save is on its way.

Items 1, 2, 3 and 5 have tests. The view tests run in light and dark. One note: `app.ObjectHandler` and `bucket.Handler` are now used only by tests. The code stays, as the ticket allowed only comment changes in the services.

`go build ./...`, `go vet ./...` and `go test ./...` pass. Report: [reports/TASK-069-report.md](reports/TASK-069-report.md).
