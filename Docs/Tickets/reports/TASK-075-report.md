# TASK-075 report: heading links in repeated text, and a cut number cell's tooltip

**Base:** `main` at 0e68d3c, with MyGo v0.9.1 pinned.
**Model requests:** none. **Downloads:** none. **New dependencies:** none.
**Docs/:** not edited, as asked. This report sits at the worktree's root.

## 1. A `#heading` click scrolled the wrong message

**Cause.** One `kit.Markdown` shows the whole thread. TASK-064 kept a clicked `#heading` link as pending and gave its anchor to the next view built with the same text. When two messages hold the same text, the first one built in the next frame took the anchor, even if the link was clicked in the second.

MyGo offers no way to tell views apart before they are built. Its view state is local to the root element, which `markdown.View` makes, so the kit can't key on it. The caller does know which view is which.

**Fix** (`internal/native/kit/markdown.go`):
- **New methods:** `Markdown.ViewAs(c, key, src)` and `StreamAs(c, key, src)` take a key that names the view among the others, the same from frame to frame.
- **The anchor:** the pending anchor holds the key of the view being built at the click, and goes only to the next view built with that key.
- **Without a key:** `View` and `Stream` are `ViewAs` and `StreamAs` with no key. They tell views apart by their text, as before, which is right for one view per `Markdown`, as in the file view.
- **The chat** (`internal/native/chat/rows.go`): its three calls pass the key the row already uses for the part:
  - `messageID:index` for a finished answer's text;
  - `stream:messageID:index` for a streaming one's.

**Test:** `TestAHeadingLinkScrollsItsOwnView`, in `kit/safelink_test.go`, light and dark.
- **Setup:** two scroll views share one `Markdown`, with the same long text and the keys `first` and `second`.
- **Before the click,** neither shows its last heading.
- **After a click** on *Go to the end* in the second view, the heading shows in the second view only.
- **Checked against the old code:** with the old match by text only put back, the test fails. The heading shows in the first view.

## 2. A cut number cell got no tooltip

**Cause.** `kit.Table` decides a cell is cut by measuring its text with `MeasureText`, then giving a cut cell a tooltip. A number cell is drawn in tabular figures (`FontFeatures("tnum")`), but it was measured in the font's own figures. Those are much narrower for some digits: in the kit's font, a "1" takes nearly twice the room in tabular figures.

Example: "11,111,111,111" measures 60 px in the font's own figures and 101 px in tabular figures. In the 96 px column it is cut, but it was measured as fitting, so it had no tooltip.

**Fix** (`internal/native/kit/table.go`): `cut` takes whether the column is numeric, and then measures with `Features: "tnum"`, as the cell is drawn.

**Test:** `TestTableTooltips`, in `kit/table_test.go`, gets a row whose number cell is "11,111,111,111". The test now asks that cell's tooltip to be the whole number. The cells that fit, "950" and "12.5K", still have none.
- **Checked against the old code:** with numeric measuring off, the test fails ("has the tooltip """).

## 3. Checks

- **Build and vet:** `go build ./...` and `go vet ./...` pass at the repo root.
- **Tests:** `go test ./...` passes, except two timing tests in packages this change doesn't touch: `internal/project` `TestCrashExport` and `internal/scenario` `TestRunTimeout`.
  - In the full run, those two packages took 235 s and 203 s, with other sessions loading the machine.
  - Run alone with `-count=1`, both pass, in 14 s and 9 s.
- **The new tests:** both fail on the old code and pass with the fix.
