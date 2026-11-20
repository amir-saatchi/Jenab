# P3-07 report: Kit blocks for table, stat and filters

**Base:** `main` at 9ba75b1. **Branch:** `claude/upbeat-sutherland-26b10c`.
**Model requests:** none. **Keys:** none were used or read.
**Dependencies:** none added; everything builds with `GOPROXY=off`.

## What I built

All of it is in `internal/native/kit`. The components take plain Go data in the kit's own types, so mapping view configs onto them is left to P3-09.

### Shared helpers (new files, so P3-08 can use them too)

- **`cellformat.go`: `CellFormat` and `FormatValue`.**
  - `CellFormat` lists the eleven 5.3 formats. `FormatOptions` holds `Currency`, `Decimals` and `Tones`.
  - `FormatValue(c, v, format, options)` writes a value in the user's regional formats through MyGo's `i18n`:
    - numbers with separators, amounts in their currency, a share as a percent with up to one decimal (0.25 is 25%, 0.004 is 0.4%, 0.256 is 25.6%) unless `Decimals` is set;
    - `date` in the medium style, and `datetime` with the short time added (a day alone shows no time);
    - booleans as Yes or No.
  - NULL and a zero `time.Time` are "". NaN and the infinities are a dash (`NoNumber`, "–") in every format. A value the format can't read is written as it is.
  - **Values it reads:**
    - **Numbers:** Go integers and floats, and numeric strings, which are kept exact. NaN and the infinities count as no number.
    - **Dates:** `TimeOf` reads `time.Time` and SQLite's text forms (`2026-11-20`, `2026-11-20 08:00:00`, RFC 3339). A `time.Time` and text are read by one rule:
      - **A day** (a date alone, or a time that is midnight in its own zone, as the SQLite driver reads a `DATE` column) is a day on the calendar, shown as it is in every zone.
      - **Any other time is an instant,** shown in the computer's zone: a `time.Time` in any zone, text with a zone, and text without one, which is read as **UTC**. That is how SQLite's `datetime('now')` and SPEC 1's timestamps (RFC 3339 in UTC) write it. A query that keeps local times should write them with their zone, or as a day.
      - The cost: an instant at exactly midnight UTC shows as its day, with no time.
    - **Booleans:** 1/0, true/false and yes/no.
- **`blockstates.go`: `BlockState` and `BlockBody`.**
  - `BlockState{Loading, Error, Notice}` holds a block's state.
  - `BlockBody(c, label, state, fn)` builds the block's body by that state:
    - **Loading:** a spinner, read as "Loading <label>".
    - **Error:** a red alert, "Can't show <label>", with the reason. It shows in place of the data.
    - **Otherwise:** a `BlockNotice` (an info icon and muted text, role status), if one is set, above what `fn` builds.
- **`icons_blocks.go`:** `IconChevronLeft`, `IconArrowUp` and `IconCalendar`, with their Lucide SVGs in `icons/`.

### Table block: `tableblock.go`

`TableBlock(c, *TableState, TableSpec) TableResult`.

- **Spec.**
  - `TableSpec` has `Label`, `Columns` (`BlockColumn{Field, Label, Format, Options, Align}`), `Rows [][]any` (one page), `Total` (or `ManyRows`), `PageSize` (default 50), `Actions` (`RowAction{Label, Bulk, Destructive}`) and `State`.
  - It also takes three callbacks: `Thumbnail(key) ([]byte, bool)`, `OpenFile(key)` and `OpenLink(url)`. Without `OpenLink`, a link opens through `c.OpenURL`.
- **Result.** `TableResult` reports `Sorted`, `Paged`, and `Action` with its `Rows`, where `Action` is -1 for none. `TableState` keeps `Sort`, `Page` and the chosen rows (`Chosen()`, `ClearChosen()`).
- **Rows.** The rows are virtual (`ui.Table` on `ListState`). The arrows, Home, End and PageDown move through them. Type-select goes by the first column.
- **Headings.** Each heading is the kit's own button:
  - a click, Enter or Space sorts by it, and doing it again reverses the order;
  - an arrow shows the sort, and assistive technology reads it (`Sort`);
  - sorting goes back to page 1.
  - Image and file columns aren't sortable.
- **Cells, by format:**
  - **Numbers:** in figures of one width (`tnum`), left to right, at the column's end.
  - **Badges:** in their tone (`Tones`, default Neutral).
  - **`url`:** a link only if `SafeLink` passes. Otherwise it is plain text.
  - **`image`:** a 28 px thumbnail from `Thumbnail`. It is decoded once and cached, with a placeholder icon while it has none. A click opens it in the file view.
  - **`file`:** the file's name with a file icon. A click opens the file view, and its tooltip is the bucket key.
  - **Booleans:** Yes or No, with No muted.
  - **Plain text:** takes the direction of its first strong character.
  - **Cut cells:** have a tooltip with the whole text.
- **Persian tables.** A table whose headings read right to left runs its columns right to left. Plain text still takes its own direction in each cell. Other formats line up with their heading, and numbers sit at the column's end.
- **Bulk actions.**
  - With any bulk action, a check box column appears ("Choose row N", plus "Choose all rows" in the heading, mixed while only some are chosen).
  - "Choose all rows" chooses the page's first 500 rows when none is chosen; when any is chosen (mixed or all), a click clears them. So on a page of more than 500 rows it chooses 500, shows mixed, and the next click clears.
  - A bar shows "N chosen" and one button per bulk action ("<Label> chosen rows").
  - The buttons are off with nothing chosen or more than 500 rows chosen. Past 500, a red note "Choose at most 500 rows" shows in the bar (role status), and each button's description says the same.
  - The chosen rows reset on sort and page.
- **Row actions:** Ghost buttons in a fixed end column, as wide as their labels. Destructive ones are in the red tone's text colour.
- **Pager:** "Rows a–b of N" (or "of many"), "Page x of n", and Previous and Next page buttons, as a navigation landmark.
- **States:**
  - **Empty:** "No rows".
  - **Loading, error and notices:** through `BlockBody`.

### Stat block: `statblock.go`

`StatBlock(c, StatSpec) Element`. `StatSpec` has `Label`, `Value`, `Format`, `Options`, `Change`, `ChangeFormat`, `Caption` and `State`.

- **Value:** 30 px with tabular figures. NULL, NaN and the infinities show "–".
- **Change:** signed. With no `ChangeFormat`, a number is written as a number (5 is "+5"). A change that is no number (a word, NaN, an infinity) is muted as it is (NaN and the infinities as "–"), with no arrow and no Up, Down or No change.
  - Above zero, it is green with an arrow up, read as "Up +2.5%".
  - Below zero, it is red with an arrow down.
  - At zero, it is muted, read as "No change".
- **Caption:** muted, in its own direction.
- **Layout:** the block is a group named by its label. It lines up from the side its label reads from, so a Persian stat starts at the right.

### Filter controls: `filterblock.go`

`Filter(c, FilterSpec, *any) FilterField{Element, Changed}`. The label sits above the control.

**`FilterSpec`:** `Label`, `Control` (text, number, date, select or checkbox), `Required` and `Options`.

**What each control binds:**

| Control | Binds | Cleared, optional | Cleared, required |
|---|---|---|---|
| text | `string` | `nil` | goes back, marked "Required" |
| number | `int64` or `float64` | `nil` | goes back, marked "Required" |
| date | ISO date text, `2026-11-20` | `nil` (a Clear button) | no Clear button |
| select | `string` | `nil` ("Any", listed first) | no "Any"; with no value, binds the first option |
| checkbox | `true` when checked | unchecked binds `nil` | unchecked binds `false`; with no value, binds `false` |

- **Text and number:**
  - commit on Enter or as the focus leaves;
  - Escape puts the value back;
  - an optional field shows "Any" as its placeholder;
  - why a commit went back ("Required", or a number it can't read) shows as red text under the field (`Field`'s error), is read with the field, and marks it invalid, until the next commit or Escape. No hover is needed.
- **Numbers** are read as the user's regional formats write them (`readNumber`):
  - the locale's decimal point, and its group separator **only where its grouping puts it**: by threes (1,234,567), or by twos before the last three in India (12,34,567). So "1,5" is refused in English, not read as 15, and "1.5" is refused in German.
  - also Persian and Arabic digits, decimal point and group separator; any space where the locale groups with one (French); a comma where the locale groups with neither a comma nor a point and its decimal point is no comma (Persian, Swiss German); and a "." where the locale's decimal point is another and its group is not ".".
  - a leading "-" or the minus "−", and the bidi marks i18n writes around a sign.
  - Anything else goes back, marked "Not a number; write it as 1,234.5" (the example in the user's formats).
- **Date:**
  - A button shows the date in the user's regional formats, or "Any date".
  - It opens a `ui.Calendar` in a popover, in Browse mode. The arrows only move, Enter or a click chooses and closes, and Escape closes without choosing.
- **Persian labels:** a filter with a Persian label lines up from the right.
- **`FilterBar(c, label, fn)`:** a wrapping row of filters, as a search landmark.
- **`RelativeDate(expr, today)`:** reads a date default (an ISO date, `today`, `today-Nd`, `today-Nm`, `today-Ny`, `start_of_month` or `start_of_year`). P3-09 can set a filter's first value with it, and the filter then shows it as a date.

## Decisions

- **`ui.Table`, not `kit.List`.** The ticket names `kit.List`. `ui.Table` is the same virtual list, with column headings, column widths and the table role for assistive technology, and it scrolled 10,000 × 12 rows by keyboard in the test.
- **The kit draws its own headings.**
  - MyGo's sortable headings sort only on a click, and Tab never reaches them (MyGo bug 1 below).
  - So the `ui.Table` columns have no `Title` and aren't `Sortable`. The kit puts its own heading button in `HeaderCell`.
- **`pressed()` registers Enter on buttons inside the table.** A table with a keyboard row claims Enter from the buttons and links in it (MyGo bug 2 below). So each of those buttons also takes Enter itself, with `Shortcut`.
- **The keyboard row shows muted, not in the primary colour,** as shadcn's selected row does. Cells go back to the page's theme for their check boxes, with a transparent background so the row's highlight shows through.
- **A table's direction comes from its headings.** If the column labels read right to left, the columns run right to left.
- **Dates bind ISO text.** SQLite compares dates as text, so a date filter binds `2026-11-20`, not a `time.Time`.
- **A percent shows up to one decimal** when it has no `Decimals` set, in tables and stats alike, so 0.025 reads "+2.5%" and 0.004 reads "0.4%", not "0%".
- **A number filter refuses what it can't read for sure,** rather than guessing: grouping must fit the locale's pattern. The field says why, with an example.
- **A required select or check box with no value takes a value in its first frame** (the first option, or false) and reports `Changed`, so what the query gets matches what the control shows.
- **The filter files are named `*block.go`, and the exported names start with `Filter`,** so they don't clash with the form controls P3-08 is building.

## Deviations

- **`kit.List` became `ui.Table`,** as explained under Decisions.
- **Select options are `[]string`.** Running an `{ query: ... }` options source is P3-09's job, which passes in the resulting list.
- **A stat's change takes only a format.** SPEC 5.10's `change` has no `format_options`, so `StatSpec` has no `ChangeOptions`.
- **A file cell opens the file view,** as the ticket says, not a download link as SPEC 5.3 says.

## Tests

All tests are headless (`ui.NewTester`) and run in light and dark.

- **`tableblock_test.go`:**
  - `TestTableBlockFormats`: every format, badge tone pixels, safe and unsafe URLs, the link opening, and Tab into and out of the table.
  - `TestTableBlockSorts`: a click, Enter, and Space on headings, with the sort read out.
  - `TestTableBlockPages`.
  - `TestTableBlockScrollsTenThousandRows`: 10,000 × 12, with End, Home and PageDown.
  - `TestTableBlockPersianColumns`: columns right to left, each cell in its own direction, and a number under its heading's end.
  - `TestTableBlockActions`:
    - a check box chosen by Space, the bulk action, the box mixed with one row chosen, a click clearing it, then choosing all and none;
    - Edit by Enter;
    - the thumbnail drawn, the placeholder for a picture with no bytes, the picture and the file opened, and the file's tooltip.
  - `TestTableBlockStates`: loading, error, empty, and a Persian notice.
  - `TestTableBlockCutCells`.
- **`statblock_test.go`:**
  - `TestStatBlock`: formats, the green rise and the red fall checked by pixels, the readouts, and NULL.
  - `TestStatBlockPersian`.
  - `TestStatBlockStates`.
- **`filterblock_test.go`:**
  - `TestFilterText`: Persian text, NULL, required, Escape, and Tab.
  - `TestFilterNumber`: grouping, Persian digits and decimal point, negative numbers, empty, and a word.
  - `TestFilterDate`:
    - the relative default shown as a date;
    - Enter opens the calendar, and an arrow alone doesn't choose;
    - Enter chooses, Clear binds NULL, a required date has no Clear, and Escape closes without choosing.
  - `TestFilterSelectAndCheckbox`.
  - `TestFilterPersian`.
  - `TestRelativeDate`.
- **`blockvalues_test.go`** (added for the review):
  - `TestFormatValueEdges`: percent 0.004, 0.256, 0.25, and with `Decimals` 0; NaN and both infinities in every number format and text; a zero `time.Time` blank; a signed number.
  - `TestTimeOf`: with the computer's zone set to UTC−5, a `time.Time` in Tehran's zone and the same instant as zoned text, RFC 3339 `Z` text and bytes all read and show the same; zoneless text read as UTC; a day (text, UTC-midnight `time.Time`, midnight text with and without a zone) shown as the same day in `date` and `datetime`; a zero time and words refused.
  - `TestFilterReadsNumbers`: 40 cases in en, de, fa, fr, en-IN and de-CH, among them "1,5" refused in English, "1.234,5" in German, both minus signs, Indian grouping, French narrow spaces and Swiss apostrophes.
  - `TestFilterRequiredFallsBack`: a required select with nil binds its first option and a required check box false, once; a required select with no options stays nil.
  - `TestFilterProblemShows`: "Required" and "Not a number; write it as 1,234.5" show as text, are read as the field's description with Invalid, and go on the next good commit or Escape.
  - `TestStatBlockOddValues`: NaN and the infinities as a dash for value and change, with no Up/Down word; a change of 5 with no format read as "Up +5"; a word change with no arrow or word.
  - `TestTableBlockOddValues`: NaN and infinities as a dash in cells, a zero time blank, 0.004 as "0.4%".
  - `TestTableBlockTenThousandRowsPaged`: 10,000 rows at the default page size, "Page 1 of 200", Next by the keyboard; then a page of 600: Choose all takes 500 and shows mixed, a click clears, 501 chosen shows "Choose at most 500 rows", disables Delete (with that description) so a click does nothing, and back at 500 it is on again.
- **`blocks_shots_test.go`, `TestBlockShots`:** saves screenshots only when `JENAB_SHOTS` is set, as the chat and projects tests do, and is skipped otherwise.
  - I rendered them headless into my scratchpad (`blocks-page-*.png` and `blocks-states-*.png`, light and dark). They aren't in the repo.
  - They show a filter bar, three stats (one Persian), and the formats table with one row chosen and the notice. They also show a loading stat, a failed stat, a Persian table, and the date filter's calendar open.

**Checks:**
- `go build ./...` and `go vet ./...` pass.
- `gofmt -l` is clean on the changed files, read with LF line endings. The checkout uses CRLF, so plain `gofmt -l` lists every file in the package.
- `go test -p 2 -short -count=1 ./internal/native/...` passes: all 9 packages with tests are ok. This includes `TestOnlyTheKitImportsMyGoUI`.

## MyGo v0.10.0 bugs found

With the user's OK, both are filed upstream, each with a reproduction that uses only `ui`. Both are still in MyGo's `main` (17c6727).

1. **A `Sortable` `ui.Table` column can't be sorted from the keyboard: [egoist/mygo#2146](https://github.com/egoist/mygo/issues/2146).** The headers are not in the Tab order, and no key sorts. A header gets `flagClickable` but not `flagFocusable`, and it sorts only on `Clicked()`.
2. **A `List` or `Table` with `Selected` set takes Enter from the buttons and links inside its rows: [egoist/mygo#2145](https://github.com/egoist/mygo/issues/2145).** `Clicked` doesn't fire on Enter for them; Space still works. The cause is that `ui/list.go` calls `Shortcut(0, KeyEnter)` on the list every frame, even with no row chosen.

**Not a bug: the `HeaderCell` element.** While debugging, I thought `Shortcut` on the element `HeaderCell` gets blocked Enter and Space window-wide. A minimal reproduction showed it doesn't. `Focused()` being false on it is expected, since a header can't take the focus. So it is not filed.

**A test pitfall, maybe not a bug:** `Tester.Focused(label)` is true for any labelled element inside the focused one. While a table has the focus, it reports every check box and button in its rows as focused. The kit's `tabTo` therefore stops on the table. The new tests use `focusOn`, which checks the focused element's own name.

## SPEC lines Main-Session should change

- **5.2 Filters:**
  - "A date filter binds its date as ISO text (`2026-11-20`)."
  - "An optional checkbox binds `NULL` when unchecked; a required one binds false."
  - "Relative dates go back only (`today-7d`); months and years run on as SQLite's do (March 31 less one month is March 3)."
  - Add: "A number filter reads the user's regional formats; grouping must fit their pattern ('1,5' is no number in English). What it can't read, or an emptied required filter, goes back with the reason shown under it."
  - Add: "A required select with no value takes its first option."
- **5.3 Table:**
  - "the user can re-sort by clicking a column" → "by clicking a column heading, or with Enter or Space on it".
  - Add: "A table whose column labels read right to left runs its columns right to left. Plain text takes its own direction in each cell; other formats line up with their heading, numbers at the column's end."
  - `image` and `file`: "a thumbnail or a download link" → "a thumbnail or a link, each opening the file view" (the ticket's wording).
- **5.3 Formats:**
  - `percent`: "shows up to one decimal unless `decimals` is set".
  - Add: "NULL, and a time that is zero, show blank; NaN and the infinities show a dash."
  - `date` / `datetime`: "A date alone, or a time at midnight in its own zone, is a calendar day, shown as it is. Any other time is shown in the computer's zone; a timestamp without a zone is read as UTC (as `datetime('now')` writes it)."
- **5.3 Table, bulk actions:** "Choose all chooses at most 500 rows; with any chosen, it clears them. Past 500 chosen, the bulk actions are off, and a note says why."
- **5.10 `stat`:**
  - Add: "A NULL, NaN or infinite value shows as a dash."
  - Add: "The change is signed, and muted at zero; with no format, it is a number; a change that is no number has no arrow."
- **Ticket P3-07, Scope:** "a virtual list (`kit.List`)" → "a virtual table (`ui.Table` in the kit)".
