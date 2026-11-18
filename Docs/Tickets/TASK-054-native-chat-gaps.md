# TASK-054 — Close the native chat's gaps
**Type:** Task
**Status:** Done
**Gate:** 3
**Needs:** SPIKE-048

## Goal
SPIKE-048 built the chat in the native app (`internal/native/chat`, the kit and the stores). Its checks left a few gaps. This ticket closes them, so the chat matches the Wails app before the other screens are ported.

## Scope
1. **The composer's *Approvals* picker**, as in the Wails composer (SPEC 8.3).
2. **Times in the user's locale format** (01:11 PM or 13:11, as Windows is set), as the Wails app shows them. Today the native app always shows 24-hour time.
3. **MyGo with the harakat fix.** [egoist/mygo#1929](https://github.com/egoist/mygo/pull/1929) fixes [#1925](https://github.com/egoist/mygo/issues/1925): marks were drawn on the baseline, so the fatha on ک sat on the letter and the kasra under ر was hidden. Tested on Windows before merging (Segoe UI, Tahoma, Vazirmatn match Edge). Move to the first MyGo release that has it, and picture sample 19 again with `jenab-bench -samples -native`: all 20 samples should then match Chromium.
4. **Part A's open items, on the new MyGo:**
   - the sidebar's selected-item colours;
   - text-field values missing from `Tester.Texts`.
   Fix each in the kit, or file it upstream if MyGo still has it.
5. **The style differences** the samples found, where the Wails look is the one to keep: the quote's muted text, links (underlined, not only blue), the code block's language label, table borders.
6. **A contrast check for the kit** (found in TASK-053). `frontend/src/contrast.test.ts` checks the theme's text and tone colours for WCAG AA. The kit copies those colours (`kit.Light`, `kit.Dark`, `kit.ToneColors`), but its test only checks that text and fill differ. Port the check to a Go test on the kit's colours, in light and dark, so it stays when TASK-061 removes `frontend/`.

## Rules
- Screens import only the kit and the stores (the test checks it).
- Ask before the MyGo update downloads, and record the version.
- Every kit change keeps its headless test: light and dark, keyboard, label, right-to-left text.

## Done when
- The picker and the time format work, with headless tests.
- The kit's contrast test passes for every text and tone colour, light and dark, with the same pairs and threshold as `contrast.test.ts`.
- The 20 samples are pictured again in both apps, with the result per sample.
- `go test ./...` passes.

## Result
Items 1, 2, 4, 5 and 6 are done, on MyGo v0.9.1; item 3 is done in TASK-082, on v0.10.0. The report: [reports/TASK-054-report.md](reports/TASK-054-report.md).

- **1. Approvals picker:** done. The composer's *Approvals* menu has *Strict*, *Standard* and *Auto*, each with its hint as a subtitle. It reads and sets the opened project's level through `Actions.SetLevel`. The headless test works by mouse and by keyboard, in light and dark.
- **2. Times:** done. `kit.Clock`, `kit.ShortTime` and `kit.MediumDate` follow Windows' regional formats: "01:11 PM" or "13:11", as in the Wails app. They are tested with US, British, Persian and 24-hour US formats.
- **3. MyGo with the harakat fix:** done in TASK-082. MyGo v0.10.0 has [egoist/mygo#1929](https://github.com/egoist/mygo/pull/1929); sample 19 now matches Chromium, and all 20 samples have the same text.
- **4. Part A's open items:** done in the kit, with nothing to file upstream.
  - **The sidebar:** MyGo v0.9.1 still fills a focused, chosen item with the accent. The kit paints it sidebar-accent, and a pixel test checks it.
  - **Text fields:** `Tester.Texts` still leaves out their text, by design; it is in `AccessInfo.Value` and `Placeholder`. `kit.Shows` reads it there.
- **5. Markdown look:** done where MyGo allows it.
  - **Done:** the quote's text is the foreground colour; the code block has no language label; the table header has no fill.
  - **MyGo gaps, with no setting:** links are underlined only on hover, so the kit keeps them blue. Tables keep a frame and column lines.
  - **Upstream:** the two requests are not filed; that needs the user's OK.
- **6. Contrast:** done. `kit/contrast_test.go` checks the kit's colours, in light and dark, with the pairs and the 4.5:1 threshold of `contrast.test.ts`. Every pair passes; the lowest is 4.77. SPEC 5.11 names the test.
- **The 20 samples**, pictured again in both apps:
  - **Same text as Chromium:** 19 of 20. Sample 19 still drops harakat, waiting for item 3.
  - **Style left:** links not underlined (18); the tables' frame and column lines (13, 14); the Persian table at the right edge, where Chromium has it at the left (14); the code block's corner *Copy* (11).
  - The per-sample table is in the report.
- `go vet ./...` and `go test ./...` pass.
