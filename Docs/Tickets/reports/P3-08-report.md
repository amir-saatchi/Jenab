# P3-08 report: kit blocks for chart and form

## What I built

Five new files in `internal/native/kit`. No existing file changed. The blocks use P3-07's `BlockState` and `BlockBody` (`blockstates.go`) and its percent format (`cellformat.go`).

| File | What it holds |
|---|---|
| `chartblock.go` | `ChartBlock(c, label, *ChartData, BlockState) Element` and its types: `ChartType` (`line`, `bar`, `scatter`, `pie`), `ChartAxis{Label, Format, Time}`, `ChartSeries{Name, X, Y}`, `ChartData{Type, Categories, X, XAxis, YAxis, Series}`, `PieSlices = 12`. |
| `formblock.go` | `FormBlock(c, label, *FormSpec, FormState) FormResult` and its types: `FormControl` (`text`, `textarea`, `number`, `date`, `select`, `checkbox`, `file`), `FormField`, `FormValue`, `FormSpec`, `FormState`, `FormResult`. |
| `chartblock_test.go`, `formblock_test.go` | 21 headless tests (16 before the review, 5 added with its fixes). |
| `chartblock_bench_test.go` | `BenchmarkChartBlock`, the size-rule charts, kept so the numbers can be measured again. |

### Chart block

- **Line:**
  - The x axis can be numbers, times (`ChartAxis.Time`, Unix seconds; days at midnight UTC stay in UTC, other instants are in the computer's zone; ticks from `chart.Time`) or categories (`ChartData.Categories`, a band).
  - A series may carry its own `X`, so series with different x values share one chart.
  - Each series is sorted by x. A point with no x is dropped. A missing or infinite y is kept as a gap: the line breaks there instead of joining its neighbours, and a point with gaps on both sides gets a dot.
  - A time axis takes the years 1 to 9999. An x outside them, such as epoch milliseconds given as seconds, is an error on the block that names the value.
  - The y axis is `Nice(5)` and doesn't force 0.
  - Lines are reduced to the pixel width with `chart.MinMax` and drawn with one `StrokePolyline`. Dots are drawn only when there is room (12 px a point).
- **Scatter:**
  - Uses the same axes as line, with `Nice(6)` on x so the edge dots aren't clipped.
  - Dots are 5 px, drawn in one `Draw` function, one per pixel cell (a bitset), so 10,000 points don't paint 10,000 shapes.
- **Bar:**
  - A band axis over `Categories`; several series are grouped side by side.
  - The y axis always includes 0, so negative bars go down from it.
  - All bars are painted in one `Draw` function, not one element each (see the numbers below).
- **Pie:**
  - The largest slices come first. At most 12 slices are drawn, "Other" included: with more than 12 values, the 11 largest and "Other" with the rest.
  - Zero and NaN values are skipped.
  - A negative value is an error on the block: "A pie can't show negative values: X is v."
  - The pointer picks a slice through `chart.SliceAt`, and the chosen slice is drawn 5 px larger.
- **Readout:**
  - `chart.Select` handles the keyboard: arrows, Home, End and Escape. It announces the choice and puts it in the plot's accessible value.
  - In a line chart, the choice moves over the union of all series' x values where some series has a value.
  - In a scatter chart, the choice moves over every point, by x and then by y, so points at the same x are each reached. The pointer chooses the point nearest it in both directions.
  - The chosen point gets a rule and a ringed dot, and the bar band is shaded in `Muted`.
  - Under the chart, a status row shows "x: Series v, Series v". A pie shows "Name: v (p%)".
  - Each part is its own text, so Persian names and Latin numbers keep their own direction.
- **Axes and formats:**
  - `ChartAxis.Format` formats a value for both the ticks and the readout.
  - Without one, numbers are written in the locale's format (`i18n.FormatNumber`, with grouping: "62,000"), on the ticks and in the readout alike. The readout keeps up to 2 decimals, or 3 significant digits below 1; a pie's share is written by P3-07's `formatValue` with `FormatPercent` ("13.3%", "25%"), as a table's percent is.
  - Times show as a medium date, with the time added when it isn't midnight.
  - The axis title is drawn under the chart.
- **Legend:** shown for two or more series and for every pie, as a wrapping row of swatches.
- **Colours:** series take the tones in their own fixed order (`seriesTones`): blue, green, amber, red, purple, then neutral, so a one-series chart is blue. From the 7th series on, the tones repeat mixed 45% toward the background. Grid lines use `Border`, and the rule uses `MutedForeground`.
- **States:**
  - These come from P3-07's `BlockBody`: loading ("Loading <label>"), an error ("Can't show <label>"), and `BlockState.Notice` above the chart.
  - An error: "Can't show <label>".
  - nil data: "No data".
  - Data with no points: "No data / The query returned no rows."
  - The chart's own error, as a pie of negative values, uses the same "Can't show <label>" alert.
- The model is worked out once for each `*ChartData` pointer and kept. Each frame reuses its buffers.

### Form block

- **Controls:**
  - `text` and `textarea` use the kit inputs.
  - `number` is a text input in tabular, left-to-right digits. It is read on submit, and it accepts Persian digits and separators: "۱٬۲۳۴٫۵" is read as 1234.5.
    - `FormValue.Number` is a float64. A whole number that fits in int64 is also given exactly as `FormValue.Int`, since a float64 rounds 9007199254740993. `Text` is always the number exactly as typed.
    - The form is filled from `Text`, `Int` or `Number`, in that order.
  - `select` is a `Picker`. An optional select gets a "None" choice first, so it can go back to empty; a required one doesn't.
  - `checkbox` is a `CheckField`.
  - An empty `date` shows a button named "Choose a date, <label>" that sets today. After that it is a `ui.DateInput`, and a Clear button is added when the field is optional. The submitted date is a copy, since the date control edits its own in place.
  - `file` takes a pick (`FormSpec.Pick`, a button named "Choose file, <label>") or a dropped file. It shows the name, a progress bar with the percentage while uploading, a Remove button and the error in red. A drop while the form is busy is ignored.
- **The upload is the caller's job:**
  - `FormSpec.Upload(field, path, progress, done)` is called with callbacks that must run on the UI thread.
  - A callback from an upload that was replaced or removed is ignored.
  - With no `Upload` function, the field says "Files can't be uploaded here."
- **Checks on submit:**
  - "Required" on an empty required field. An unchecked required checkbox counts as empty, as in HTML.
  - "Enter a number" on a number field that can't be read.
  - "Wait for the upload to finish" while a file is uploading.
  - The first field with a problem gets focus, and nothing is submitted.
  - Enter in a text or number field submits.
- **Errors from the write:**
  - `FormState.Errors` puts a message on its field, and it stays until that field is edited.
  - `FormState.Problem` is a "Not saved" alert above the button. An error under a name that is no field's goes in the same alert, as "owner: no such user", rather than being lost.
  - While the form is busy, the last write's errors are hidden, so an old error doesn't show again during a new submit.
- **Busy:** `FormState.Busy` disables every control and shows a disabled, busy "Saving…" button with a spinner.
- **Update form:**
  - `FormSpec.Values` fills the fields.
  - A new `*FormSpec` pointer fills the form again, so P3-09 passes a new spec when `load` returns.
  - `FormState.Missing` shows "Nothing to edit / This row is no longer there."
- **States:** P3-07's `BlockBody` with `FormState.BlockState`: loading, an error ("Can't show <label>", which was "Can't open" before) and a notice above the form; then missing, as above.
- `FormResult` returns the element, `Submitted` and the `Values` on the frame the form is submitted.

## Decisions and deviations

1. **The kit doesn't downsample or cap. The runtime does.**
   - SPEC 5.4's 10,000-row limit and its notices belong to the runtime (P3-05/P3-09). The kit shows the notice it is given in `BlockState.Notice`.
   - The kit doesn't need LTTB: `MinMax` per pixel column keeps a 10,000-point line about as fast to draw as a 2,000-point one (numbers below).
2. **Pie rules are in the kit too.** The 12 slices plus "Other" and the negative-value error are checked in the kit, so any caller gets them. P3-09 doesn't need to repeat them.
3. **Several series in a bar chart are grouped, not stacked.** SPEC doesn't say which. Stacking could come later as a config field.
4. **Tone order: blue, green, amber, red, purple, then neutral** (`seriesTones`, not `kit.Tones`' order, which starts at neutral and would make a one-series chart gray). Main-Session agreed after the first review. `TestSeriesColor` checks the order and the faded repeat in both themes.
5. **Numbers are typed as text**, not with a spinner control, so Persian digits and separators work. They are read on submit.
6. **"Required" is the field's description** when the field has none, so screen readers say it, rather than an asterisk.
7. **The upload is a callback, not a goroutine in the kit,** so the kit stays free of I/O. The caller runs the upload and reports progress on the UI thread (`win.Update`).
8. **Shared states:** after P3-07 reached main, this branch merged main, dropped its own `blockstate.go`, and uses P3-07's `BlockState{Loading, Error, Notice}` and `BlockBody` in both blocks.
   - **`ChartData.Notices` is gone.** A chart's notice is `BlockState.Notice`, as for tables and stats, so P3-09 puts the 10,000-row notice in one place for every block. (Only one notice per block; a second one could join it with a space.)
   - The loading state is now P3-07's spinner (at least 96 px high), not a 220 px placeholder, so a chart's page can move a little as it loads.
   - The form's error title is now "Can't show <label>", as for every block.
9. **`cellformat.go`:** no names clash. The pie's percent now uses P3-07's `formatValue` with `FormatPercent`. `chartValue` stays: it writes at most 2 decimals, or 3 significant digits below 1, which `FormatOptions.Decimals` (a fixed count) can't. A time axis follows P3-07's `TimeOf` rule (SPEC 5.3), so a view's table and chart show the same hour. If every x is at midnight UTC, the axis is days and stays in UTC, so no day moves to the one before. Otherwise its ticks and readout are in the computer's zone (`time.Local`), which MyGo's `chart.Time` honours when rounding ticks. Tests pin `time.Local` to UTC−5, as `TestTimeOf` does. P3-09 can map a view's `format` onto `ChartAxis.Format` with `FormatValue`.

## Test results

- `go build ./...` and `go vet ./...`: pass.
- `go test -p 2 -short -count=1 ./internal/native/...`: every package passes, including `TestOnlyTheKitImportsMyGoUI`. The kit took 6.6–16 s; after the review fixes, 9.8 s.
- `gofmt -l` on the six files: clean.
- Every test checks light and dark, keyboard use, the accessible label and right-to-left (Persian) text:
  - `TestChartBlockLine`: time x, a per-series X, a custom format, the Home, Right and End readouts, the value, Escape and Tab.
  - `TestChartBlockLineOfCategories`: Persian months, a NaN, and no legend for one series.
  - `TestChartBlockBar`: three series, a negative value and a Persian category.
  - `TestChartBlockScatter`.
  - `TestChartBlockPie`: 14 categories become 12 slices plus "Other", with readouts.
  - `TestChartBlockPiePointer`.
  - `TestChartBlockStates`: loading, error, a notice (read out), empty with a notice, negative pie and nil data.
  - `TestChartBlockFewerPoints`.
  - `TestChartBlockScatterSameX`: four points at one x reached by the arrows, and by the pointer up and down.
  - `TestChartBlockLineGaps`: a NaN and an Inf break the line (checked on the pixels), the point between them gets a dot, and the readout passes over the gaps.
  - `TestChartBlockEdgeCases`: a single point (line and scatter), all-equal values, all-negative bars, 10,000 bars and a pie of zeros.
  - `TestChartBlockTimeZones` and `TestTimeZone`, in UTC−5: days stay on their day; instants get a local-midnight tick and read out 22:30 on the day before (it fails with the zone forced to UTC).
  - `TestChartValue`: English, German and Persian number formats; `TestUnixTime`: huge and epoch-millisecond values held to the years 1 to 9999.
  - `TestChartBlockStates` also covers epoch milliseconds on a time axis.
  - `TestFormBlockControls`: every control by keyboard.
  - `TestFormBlockRequired`.
  - `TestFormBlockWriteErrorsAndBusy`.
  - `TestFormBlockUpdate`.
  - `TestFormBlockFile`: pick, progress, the upload holding the submit, an error, a drop, a stale callback and Remove.
  - `TestFormBlockStates`: loading, error, a notice above the form, and missing.
  - `TestFormBlockExactNumberAndNoChoice`: 9007199254740993 kept exactly, Persian digits, an optional select back to "None" and a required one without it.
  - `TestReadDecimal`, now with `readWhole`.
  - `TestFormBlockControls` checks that the submitted date doesn't change as the field does. With the copy taken out, this test fails.
  - `TestFormBlockWriteErrorsAndBusy` checks an error on no field and that old errors hide while busy; `TestFormBlockFile` checks that a drop while busy is ignored.
- **Screenshots** of each chart type and the form (normal and busy), in light and dark, were taken headless. They are kept outside the repo.

## Chart numbers

The PC has an Intel i5-1235U with integrated graphics, and the window is 1,000 × 600. Each chart has one series.

### Headless (CPU painter, `go test -bench BenchmarkChartBlock`), ms per frame

- **first:** a new `ChartData` (model worked out, built, laid out, painted)
- **frame:** an arrow key moving the choice
- **resize:** the width switching between 1,000 and 800

The two runs differ because other sessions were using the CPU; the second run was the busier one.

| Chart | first | frame | resize |
|---|---|---|---|
| line, 2,000 points | 0.38–0.46 | 0.72–0.77 | 1.6–2.0 |
| line, 10,000 points | 1.7 | 0.88–0.92 | 2.3–2.4 |
| scatter, 10,000 points | 4.4–5.6 | 5.9–9.5 | 6.2–10.6 |
| bar, 1,000 bars | 0.36–0.55 | 0.97–1.4 | 2.9–4.3 |
| the old `kit.BarChart`, 1,000 bars as elements | | | 5.1–6.1 |

**10,000 bars**, over SPEC's cap (after the review fixes, on a busier machine: the old `BarChart`, whose code didn't change, measured 8.8–9.3 ms against 5.1–6.1 above, so divide by about 1.7 for a quiet machine):

| Chart | first | frame | resize |
|---|---|---|---|
| bar, 1,000 bars | 0.68–0.77 | 1.4–1.5 | 3.9–4.0 |
| bar, 10,000 bars | 6.5–7.6 | 11.5–12.7 | 33.8–34.2 |

10,000 bars draw correctly, and the choice and readout work. But each frame paints 10,000 one-pixel bars, so a resize takes about two 60 fps frames. The 1,000-bar cap therefore matters for speed as well as legibility. If the cap is ever lifted, bars narrower than a pixel should be merged per pixel column, as lines are.

In the same busy run, the scatter plot's first frame went from 4.4–5.6 ms to 8.2–13 ms. About 1.7× of that is the machine; the rest is sorting every point for the new per-point selection.

### Real window on the GPU

I used a scratch probe that was not committed, with `MYGO_FRAME_STATS=all` and `MYGO_UNCAPPED=1`, and summed the frames of a 4-second run:

- **frame:** a new picture every frame, switching between two datasets, so the model is worked out again on every frame. This is the worst case.
- **resize:** the window switching between 1,000 and 800 px wide every 60 ms.

| Chart | mode | frames | total, median / p95 / max (ms) | build | paint | present (median) |
|---|---|---|---|---|---|---|
| line, 2,000 | frame | 1,311 | 2.7 / 5.0 / 20.6 | 0.5 | 0.5 | 1.1 |
| line, 10,000 | frame | 799 | 4.1 / 7.5 / 59.8 | 1.2 | 0.5 | 1.6 |
| scatter, 10,000 | frame | 361 | 10.6 / 14.1 / 17.0 | 3.5 | 1.0 | 5.4 |
| bar, 1,000 | frame | 1,817 | 1.3 / 3.8 / 48.7 | 0.0 | 0.0 | 1.0 |
| line, 2,000 | resize | 50 | 19.5 / 22.9 / 34.6 | 0.0 | 0.0 | 18.7 |
| line, 10,000 | resize | 46 | 19.8 / 28.9 / 35.2 | 0.0 | 0.6 | 18.5 |
| scatter, 10,000 | resize | 48 | 20.4 / 21.7 / 33.7 | 0.0 | 1.1 | 19.3 |
| bar, 1,000 | resize | 48 | 18.5 / 22.2 / 28.2 | 0.0 | 0.0 | 17.5 |

What the numbers show:

- **Speed is no longer a reason for the size rules.** Every chart, with new data on every frame, stays inside a 60 fps frame (16.7 ms) at p95.
- **Resizing costs the same for every chart.** About 18 ms of present goes to resizing the window's surface, and build, layout and paint stay under 2 ms at p95.
- **Frames that don't change cost almost nothing.** When nothing changes, MyGo reuses the last picture: an unchanged frame took about 0.5 ms.
- **The rare high maximums are one-off spikes:** 60 ms on the 10,000-point line and 49 ms on the bars. One early run also had a 900 ms present on a cold start. They aren't steady costs.
- **SPEC's old numbers were on SVG:** Recharts took 1.0–2.2 s to draw 10,000 points and up to 4.7 s to resize. Here the same chart takes 1.7 ms to draw and about 20 ms to resize.

## Proposed size rules (to replace SPEC 5.4's "Size rules")

- **Keep the 10,000-row limit** and its notice. It holds back memory and query time, not drawing.
- **No downsampling.** Lines are drawn exactly up to 10,000 points: the kit keeps each pixel column's minimum and maximum, so the shape and the peaks are exact at any size. Scatter is drawn exactly up to 10,000 points too. Drop the LTTB rule.
- **No animation rule.** Native charts don't animate. Dots on a line are shown only when there is room for them (about 12 px a point), which the kit decides at each width, not from a data size.
- **Bars:** keep the first-1,000 limit with its notice. 1,000 bars is about one pixel a bar at 1,000 px wide and takes about 1 ms a frame; 10,000 bars take about 20 ms to resize on a quiet machine. A lower limit, such as 200, would also be reasonable for legibility.
- **Pie:** unchanged (at most 12 slices, "Other" among them).
- **Remove the uPlot line.**

## MyGo bugs found

None. I filed nothing upstream. Two ideas could be raised with the user's OK:

- A scatter mark that skips duplicate points (the bitset in `drawDots`) could live in `ui/chart`.
- Bars drawn by a `Draw` function with `p.Fill` were about 1.7× faster to resize than bars as elements. A `chart.Bars` mark that paints instead of building elements would help every user of `ui/chart`.

## SPEC lines Main-Session should change

1. **5.4, "Size rules":** replace the whole list with the proposed rules above, citing P3-08's numbers instead of SPIKE-022's Recharts numbers.
2. **5.4:** add that several series in a bar chart are grouped. Stacking, if wanted, would be a later field.
3. **5.4, Pie:** add that zero and empty values are skipped, and that the negative-value error names the slice. Say "at most 12 slices, Other among them" so the count is clear.
4. **5.4:** add that a line breaks at a missing value instead of joining across it, and that a time x outside the years 1 to 9999 is an error on the block.
5. **5.11, "Charts: series take the tones in a fixed order":** name the order. Blue, green, amber, red, purple, then neutral, so a one-series chart isn't gray; after the sixth series the tones repeat, faded toward the background. The kit does this now.
6. **5.5:** say that `number` accepts Persian digits and separators and that a whole number is kept exactly (for `INTEGER` columns past 2^53), that a required checkbox must be checked, that an optional select has a "None" choice, and that the date control starts empty with a "Choose a date" button.

## Review fixes (Main-Session's review)

All twelve points of the review, and the rename, are in the commit after the first:

- B. `formatValue` is now `chartValue`, so it doesn't clash with P3-07's.
- 1. A pie draws at most 12 slices, "Other" among them.
- 2. The submitted date is a copy.
- 3. `FormValue.Int` holds a whole number exactly; `Text` is the number as typed.
- 4. Drops are ignored while the form is busy.
- 5. Errors on unknown names show above the button, and the last write's errors hide while busy.
- 6. "Choose file, <label>" and "Choose a date, <label>" keep the button's own name.
- 7. An optional select has a "None" choice.
- 8. Every scatter point is reached, by the arrows (by x, then y) and by the pointer (nearest in both directions).
- 9. Readout values, percentages and tick labels are in the locale's number format.
- 10. A missing or infinite y breaks the line.
- 11. A time x outside the years 1 to 9999 is an error on the block, and `unixTime` clamps instead of overflowing.
- 12. Edge-case tests, and the 10,000-bar numbers above.

- A. Done after P3-07 reached main: main is merged in (no conflicts), `blockstate.go` is gone, and both blocks use `BlockBody`. See decisions 8 and 9.
