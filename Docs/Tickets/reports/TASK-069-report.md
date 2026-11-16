# TASK-069 report: small native fixes

**Base:** local `main` at eb1031d, with MyGo v0.9.1 pinned.
**Model requests:** none.
**Downloads:** none. `golang.org/x/text` was already a dependency.

## 1. Persian and Arabic digits

`kit.ReadNumber` now reads Persian digits (۰ to ۹) and Arabic-Indic digits (٠ to ٩) as 0 to 9, before it compares the text with the value shown. So `۱۲` and `١٢` are 12, and `۴` in a field that shows 4 is no change. A decimal point is still refused, as before. This works in every `NumberInput`.

Tests: `TestReadNumber` has new cases for both digit sets, for mixed digits, for zero below the least, and for a Persian decimal point. `TestNumberInputCommitsOnce` types `۸` and `٩` into the field.

## 2. Text direction

`kit.RightToLeft` now finds the first strong character by its Unicode bidi class (`golang.org/x/text/unicode/bidi`).

- R and AL (Hebrew, Arabic, Persian…) mean right to left.
- L means left to right. L covers every left-to-right script, such as Latin Extended Additional (Vietnamese), Chinese, Japanese, Korean, Thai, Devanagari, Georgian and Armenian.
- Digits, punctuation and signs are weak or neutral, so they are passed over. This includes Persian digits (EN), Arabic digits (AN), × (U+00D7) and ÷ (U+00F7).

This is how `dir="auto"` decides, and it is used by the inputs, the composer and Markdown blocks.

Test: `TestRightToLeft` keeps the old cases. It adds Hebrew, an empty text, and the eight scripts above, each followed by Persian. It also adds × and ÷ before Persian, a sum, and Persian digits in brackets.

## 3. Table tooltips

`kit.Table` now gives a tooltip to any cut cell, in any column. A cell counts as cut when its text, measured at 14 px (`Context.MeasureText`), is wider than the cell's box in the last frame. If that answer changes, for example after a resize, the frame is built again. A cell that fits gets no tooltip, so a short value or a number has none. Before, only the first column had tooltips, and every cell in it had one, cut or not. The doc comment now says this.

Test: `TestTableTooltips` checks a 420 px table. A long path in the first column and a long model name in a 96 px column have their text as their tooltip. Short cells and numbers have none.

## 4. Old text

These comments described Wails or its frontend. They now describe the native app:

- `internal/app`: `bucket.go`, `upload.go`, `bucket_read.go`, `events.go`, `errors.go`, `services.go`, `system.go`, `chat.go`, `errors_test.go`, `events_test.go`, `app_test.go`, `bucket_test.go`, `chat_test.go`, `services_test.go`.
- `internal/bucket/serve.go`.
- `internal/project/events.go`.

Only comments changed. "The frontend" became "the UI". Wails' route mounting, binding, marshalling and asset server are no longer mentioned.

**README.md** is not edited, because Main writes it. It should say:

- **Status line**, in place of "in design. There is no app to use yet…". Something like: "**Status:** in development, not released yet. Phase 1 (the core chat loop) is done and Phase 2 (structured memory) is nearly done, in the native app. Pages and pipelines (Phases 3 and 4) are next. There are no release builds yet: build it from source with [Development](Docs/DEVELOPMENT.md)."
- **The mockups line** says "the high-fidelity UI, built with React and shadcn/ui". Keep it, but say these are design mockups, not the app. The app's UI is native, on MyGo.
- **Needs:** the app is built and tested on Windows 11 only so far (macOS and Linux aren't checked yet). Either say "Windows 11 for now", or keep the list and mark macOS and Linux as planned.
- **Read more:** add "Run it: `go run ./cmd/desktop` (Go 1.27.3)", or point to DEVELOPMENT.md for that.

## Robustness

- **`kit.BarChart` with a nil `format`** now writes values as plain numbers (`strconv.FormatFloat`, 'f', -1) and doesn't panic. Test: `TestBarChartWithoutFormat` chooses a bar, and the chart reads "Tue: Calls 7".
- **`NumberInput` committed the same value twice** when the save was still on its way: once on Enter, and again as the focus left. It now remembers the value it committed until the value shown changes or `Failed` is called, and doesn't commit that value again. A different value still commits. After a failed save, the same value commits again. Test: `TestNumberInputCommitsOnce` keeps the value unchanged, as a slow save does, and covers Enter then Tab, a new value, a failed save, and the value arriving.

## Not done, and notes

- `app.ObjectHandler` and `bucket.Handler` serve `/objects/` over HTTP, but since TASK-061 only tests use them. The app reads files in its own process (`app.NativeBucket`). The ticket allowed only comment changes in the services, so the code stays. Removing it, or keeping it for later pages, is a separate choice.
- `UIError.MarshalJSON` is in the same position: nothing in the app marshals errors to JSON now, but its test keeps the shape.
- The table measures without the `tnum` feature that numeric cells use. A number that is cut by less than the width difference between tabular and proportional digits may show no tooltip. Numbers in a 96 px column are rarely that long.

## Checks

`go build ./...`, `go vet ./...` and `go test ./...` pass. The changed Go files are gofmt-clean. The ticket asks for tests only, so there was no live check.
