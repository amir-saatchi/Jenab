# P3-08 — Kit blocks: chart and form
**Type:** Feature
**Status:** Open
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
