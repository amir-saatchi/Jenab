# TASK-054 report: the native chat's gaps

**Base:** `main` at 35481bf, with MyGo v0.9.1 pinned.
**MyGo:** not moved. [egoist/mygo#1929](https://github.com/egoist/mygo/pull/1929) is not merged, and v0.9.2 doesn't have it.
**Model requests:** none.
**Downloads:** none.

**Commits:**
- b7a42ee: items 1 and 2;
- 5f48095: item 4;
- e07786d: item 5;
- fbe48db: item 6;
- this report's commit: the Result, the report and the README row.

## 1. The *Approvals* picker

The composer has an *Approvals: Standard* menu after the model picker, as in the Wails composer (SPEC 8.3).
- **The menu:** a disabled title, "Approvals for this project", then *Strict*, *Standard* and *Auto* as radio items. Each item has its hint as a subtitle, with the Wails app's words.
- **The level:** the menu reads the opened project's level, *standard* when none is set. A choice calls `Actions.SetLevel`, through the same service as the Wails app. When the call succeeds, the store's project takes the new level.
- **Pieces:**
  - `state.ActionsAPI.SetLevel` and `Actions.SetLevel`;
  - `logic.Levels` and `logic.LevelName`;
  - the fake's `SetLevel`, which records strict, standard and auto and rejects anything else.
- **Tests:**
  - `TestTheApprovalsPickerSetsTheLevel` runs in light and dark. A click chooses *Auto*. Then the keyboard reaches *Strict*: Tab to the picker, Enter, Home, Enter. The menu opens at the checked item, so Up from there would choose *Auto* again.
  - `TestLevelName` covers the names.

## 2. Times in the user's format

`kit/time.go` writes times as Windows is set, through MyGo's `Context.Region`, which reads the user's time format. The app's language, English, is kept.
- **The functions:**
  - `kit.Clock`: "01:11 PM" or "13:11", for a message's time, a card's answer and a rewind mark. It matches the Wails app's `toLocaleTimeString` with hour and minute "2-digit".
  - `kit.ShortTime`: "1:11 PM" or "13:11", for "Last active" and the notes line.
  - `kit.MediumDate`: "Nov 16, 2026", or "16 Nov 2026" with British formats.
- **The cache:** the locale is cached for the language and region it was found for, since every frame writes times. When the system says nothing of the hour cycle, as for a Tester's zero Region, the language's CLDR defaults apply.
- **Screens:** they pass formatters into `logic.CardTime`, `logic.NotesLine` and `shell.Subline`, so `logic` stays free of the kit.
- **Tests:**
  - `kit.TestTimesFollowTheRegion` covers en-US 12-hour, en-GB 24-hour, en-US set to 24-hour, fa-IR and the zero region, in light and dark.
  - The chat's `TestTimesFollowTheRegion` checks a message's time in 12 and 24 hours.

## 4. Part A's open items, on MyGo v0.9.1

- **The sidebar's selected item:** MyGo v0.9.1 still fills a focused, chosen item with `Theme.Accent`, where the web app keeps sidebar-accent gray.
  - **The fix, in the kit:** `SidebarItem` paints the chosen item sidebar-accent, with its text sidebar-accent-foreground, and the theme's `AccentText` matches.
  - **The test:** `TestSidebar` checks the pixels: an item not chosen is the sidebar's colour, and the chosen, focused item is sidebar-accent. Without the fix the test fails, checked twice.
  - **Upstream:** not filed. It is MyGo's design choice, not a bug.
- **Text-field values in `Tester.Texts`:** v0.9.1 still leaves them out, by design. A field's text is in `AccessInfo.Value` and its placeholder in `AccessInfo.Placeholder`.
  - **The kit:** `kit.Shows` looks in `Find`, `HasText`, then every node's Value and Placeholder. The kit's, chat's and shell's `shows` helpers use it.
  - **The test:** `TestTextInputAndTextArea` now checks the placeholder and the typed Persian and tab text. It logs a line if `Texts` ever starts to hold them.
  - **Upstream:** nothing to file.

## 5. The Markdown look

In `kit.markdownTheme`, which `markdown.View` and `markdown.Stream` both use:
- **Quote:** the text is the foreground colour, not muted, with a 2 px bar, as in the frontend.
- **Code block:** no language label. The accessible label is still "Code, go", and *Copy* stays.
- **Table:**
  - the header has no fill, which needs a transparent colour, as MyGo treats the zero colour as its default;
  - cell padding is `1em × 0.75em`.
- **Links:** weight 500. They stay blue.

**MyGo gaps, with no setting in v0.9.1:**
- **Links** are underlined only on hover. The Wails app underlines them in the text colour. A foreground link with no underline wouldn't read as a link, so the kit keeps them blue.
- **Tables** get a frame and column lines in `TableBorder`. The Wails app draws row lines only.

**Rejected:**
- **A kit Grid in place of MyGo's table:** it would lose the row semantics Narrator reads.
- **Rewriting links as `<u>`:** it would break streaming, selection and copying.

**Upstream:** two candidate requests, an underline setting for links and a row-lines-only table border. They are not filed: filing posts to a public repository, which needs the user's OK.

**The test:** `TestMarkdownLooksLikeTheFrontend` runs in light and dark, with a Persian quote, a link, a table and Go code. It checks:
- the theme's quote colour, bar width and link weight;
- the header cell's pixel is the background, which fails with the old fill;
- there is no language label, while "Code, go" and *Copy code* are still found;
- Tab reaches the link.

## 6. The kit's contrast test

`kit/contrast_test.go` ports `frontend/src/contrast.test.ts`.
- **The pairs:** the same pairs, from foreground on background to each tone on the background, its soft fill and the sidebar.
- **The threshold:** WCAG AA, 4.5:1.
- **The colours:** the kit's own colours, in light and dark, 8-bit sRGB as the kit draws them. The web test computes them from oklch.
- **`TestContrastRatio`** checks the formula: black on white is 21, white on white is 1, and `oklch(0.708 0 0)` on white is under 4.5.
- **Results:**
  - Every pair passes.
  - The lowest are both in light: muted-foreground on muted (4.82) and destructive on the background (4.77). Raising the threshold to 5 makes the test fail and name those two.
- **SPEC 5.11** now names the test, in place of "no contrast check of its own yet".

## 3. MyGo with the harakat fix: open

Waits for a MyGo release that has [#1929](https://github.com/egoist/mygo/pull/1929). Then:
- move to it, after asking, since the update downloads;
- picture sample 19 again.

## The 20 samples

**The run:**
- Pictured on 2026-11-16 with `jenab-bench -samples -native`, at 1280×800, 125 % scale, in the dark theme.
- The native app was built from this branch.
- The Wails app is SPIKE-048 Part B's bench build. The frontend has changed since then only in tool rows, settings and the provider card, none of which the samples show.
- The screenshots stay in `bin/`, uncommitted: they hold a user-name path.

**Changes in every sample, compared with Part B's run:**
- the times now read "06:11 PM" and "Last active 6:11 PM", as in the Wails app;
- the native composer has *Approvals: Standard*.

**Per sample, native against Chromium:**

| # | Sample | Result |
|---|---|---|
| 1 | Persian | Same |
| 2 | English | Same |
| 3 | Persian with English | Same |
| 4 | English with Persian | Same |
| 5 | Digits and punctuation | Same |
| 6 | ZWNJ and joining | Same |
| 7 | Brackets in RTL | Same |
| 8 | Heading, Persian | Same |
| 9 | Persian quote | Same. The text is no longer muted. |
| 10 | English quote with Persian | Same. The text is no longer muted. |
| 11 | Go code with tabs | Same text. The language label is gone. The native block keeps its own *Copy* in the corner, and its syntax colours differ a little. |
| 12 | Inline code | Same |
| 13 | English table | Same cells. The header has no fill now. The frame and column lines remain (MyGo gap). |
| 14 | Persian table | Same cells, right to left. The header has no fill now, and the frame and column lines remain. The native table sits at the right edge; the Wails one at the left. |
| 15 | Persian list | Same |
| 16 | Mixed numbered list | Same |
| 17 | Nested list | Same |
| 18 | Link and email | Same text. Links are blue and not underlined; the Wails app underlines them in the text colour (MyGo gap). |
| 19 | Harakat | Still different: egoist/mygo#1925, item 3. |
| 20 | Emphasis and emoji | Same |

**Summary:**
- 19 of 20 samples have the same text as Chromium. Sample 19 waits for item 3.
- The style differences left:
  - links not underlined;
  - the table's frame and column lines;
  - the Persian table's place;
  - the code block's corner *Copy*.

## Checks

- `go vet ./...` and `go test ./...` pass, 38 packages. `frontend/dist` was copied from an earlier local build, because `go:embed` needs it and building the frontend would have meant installing packages.
- `TestOnlyTheKitImportsMyGoUI` passes: the screens import only the kit and the stores.

## Follow-ups

- **Item 3:** MyGo with #1929, when released.
- **Upstream requests to MyGo, if the user agrees:**
  - a link underline setting;
  - a table border with row lines only.
- **The Persian table's place** (sample 14). Chromium keeps the table at the left edge of an RTL message, because the table's `dir` is set but its block is not aligned. Either app could change; the SPEC doesn't say which is right.
