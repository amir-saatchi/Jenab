# P3-12 — Page speed: six blocks in under 1.5 s
**Type:** Task
**Status:** Open
**Gate:** 3 (Phase 3)
**Needs:** P3-09
**Requirements:** N-03, N-04, N-05

## Goal
Measure Phase 3's numbers in the native app and keep them in tests: a page with six blocks, including a 10,000-row table and a chart, paints in under 1.5 s (Gate 2, PROPOSAL 9).

## Scope
- **The page:** six blocks on a fixture project: a 10,000-row table, a line chart over 10,000 rows, a stat, a bar chart, a form and a text block, with a page filter.
- **Measures:** from the click to the first frame with every block filled, on the SPIKE-022/SPIKE-048 PC (N-05); scrolling the 10,000-row table at 60 fps and a 500-row page from Go in under 50 ms (N-03); the 10,000-row chart in under 1 s (N-04); changing the filter; resizing the window; memory.
- **A benchmark** in the repo (like the chat's) that a later change can rerun, and a short test with a looser limit for CI.
- **SPEC:** the numbers for SPEC 5.3's transfer and rendering lines, which still describe React. 5.4's size rules were rewritten from P3-08's chart numbers; check them on the whole page. Main-Session rewrites from the report.

## Done when
- The six-block page paints in under 1.5 s, or the report shows what takes the time and a fix is filed.
