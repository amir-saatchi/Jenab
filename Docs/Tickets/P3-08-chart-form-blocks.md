# P3-08 — Kit blocks: chart and form
**Type:** Feature
**Status:** Done
**Gate:** 3 (Phase 3)
**Needs:** –
**Requirements:** R-41, R-43, R-116

## Goal
The kit components for the chart and form blocks (SPEC 5.4, 5.5, 5.11), on MyGo's chart package and the kit's inputs. They take plain Go data in the kit's own types, so this needs no other Phase 3 ticket; P3-09 maps view configs onto them.

## Scope
- **Chart:** `line`, `bar`, `scatter` and `pie` on MyGo's `ui/chart`, with axes and their formats, series in the tones' fixed order, a legend, a keyboard-reachable point readout (`chart.Select`), and the notices from P3-05. The existing `kit.BarChart` is a start.
- **Form:** each control (`text`, `textarea`, `number`, `date`, `select`, `checkbox`, `file` with the upload's progress), required fields, errors on their field, submit with a busy state, and the update form filled from `load`.
- **States:** loading, empty, an error on the block.
- **Speed:** a 2,000-point line and a 1,000-bar chart draw within the frame budget; the report gives the numbers for P3-05's size rules.

## Rules
- As P3-07: only the kit imports MyGo's `ui`, headless tests in light and dark, by keyboard and label, with right-to-left text.

## Done when
- Headless tests for each chart type, each control and each state.
- Screenshots in light and dark, kept outside the repo.

## Result
Done on 2026-11-21. Report: [P3-08-report](reports/P3-08-report.md).

- **Chart:** `ChartBlock` draws line, bar, scatter and pie charts on MyGo's `ui/chart`. It has formatted axes, a legend and series in a fixed tone order.
  - The keyboard moves a readout over every point, and the numbers are in the user's locale.
  - A line breaks at missing values, and a pie draws at most 12 slices, "Other" among them.
  - A time axis follows 5.3's date rule: days in UTC, other times in the computer's zone.
- **Form:** `FormBlock` has text, textarea, number, date, select, checkbox and file controls. File uploads take a progress callback.
  - It has required fields, errors on their field (others above the button), a busy state and update forms filled from values.
  - A whole number is kept exactly (`FormValue.Int`).
- **States:** both blocks use P3-07's `BlockState` and `BlockBody`; the 10,000-row notice is `BlockState.Notice`.
- **Speed:** every chart stays inside a 60 fps frame, so SPEC 5.4's size rules are rewritten from these numbers. There is no downsampling, and the 1,000-bar cap stays.
- **Review:** two Medium and ten Low points are fixed, plus the time zone and the series order. The pie draws 12 slices, and a submitted date is a copy.
