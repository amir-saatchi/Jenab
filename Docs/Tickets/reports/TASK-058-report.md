# TASK-058 report: the developer tools in the native app

**Base:** `main` at 1f581f4, with MyGo v0.9.1 pinned.
**Model requests:** none. The live check used the bench's fake model.
**Downloads:** none. The two new icons (wrench, activity) are Lucide's paths, copied from a lucide-react already on this machine. A git-ignored placeholder `frontend/dist/index.html` stands in for the embed, as in TASK-051.

## What was built

The turn inspector and *Runtime* (SPEC 8.4), ported from `frontend/src/dev`, in the new package `internal/native/dev`.
- **Where they open:** the Wails app shows them as wide pages of Settings. The native app has no Settings screens yet (TASK-055), so they open from the rail: while the developer tools are on at this start, the rail has *Developer tools* (a wrench), above Settings. It opens one page with a bar holding the two tools, *Turn inspector* and *Runtime*, and the badge "Read-only · secrets redacted". The page's heading, text and pickers are the Wails page's.
- **Inspect:** with the tools on, each turn's footer has *Inspect* after *Undo turn*, as in the Wails footer. It opens the inspector at that turn.
- **Off:** with the tools off, neither shows, and the page (if reached) says "The developer tools are off", as `dev-page.tsx` does.

### The turn inspector (`dev/inspector.go`)

As `inspector.tsx`:
- **Pickers:** a chat picker (each chat once, newest first, "Project · Chat") and a turn picker ("14:05 · turn 3 · 2 requests", the time in the user's format through `kit.Clock`). Choosing a chat shows its newest turn.
- **Timeline:** the time axis (at most six round ticks), then the Model, Tools and Parts lanes, and the "Tool calls:" line with the names in the code font. A running turn says "so far, still running".
- **Requests:** prompt, cached, output, time and tools per request, "–" before the provider's counts, and a request's error in a row under it. A click on a row, or Enter on its number, chooses it.
- **Tool calls:** time, result size (red with "· error"), and the ref.
- **Context blocks:** of the chosen request: the share bar, tokens, the cache badge (hit green, part amber, miss neutral), "· cache point", and the total with the share read from the cache. A block's name opens its text, unless the texts were dropped.
- **The block's text:** a dialog with "Request N · about T tokens · secrets redacted" and the text in the kit's new `LinesView`, a virtual list of lines, so a long body opens at once. "(empty)" for an empty block; the error for a block not recorded. Escape closes it.
- **States:** "No turns recorded yet", "Can't show this turn" with the reason, a spinner while it loads.

### *Runtime* (`dev/runtime.go`)

As `runtime.tsx`: the stuck warning, the LLM-call slots and each provider's state and limit, then each open project with its work (state, progress, running for, last moved, limit, *Stuck*) and its database writers (queue, now, done, last error, *Stuck*). A failed read shows "Can't read the runtime" over the last runtime read.

### The store (`state/dev.go`)

`state.Dev` holds the turns, the pick, the turn read for it, the runtime and the open block. It is not in `Stores`: `main` makes it with `state.NewDev(b.Dev, mailbox.Post)`.
- **Reads repeat as the screen builds.** The screen calls `PollTurn` or `PollRuntime` each frame; each reads only when a read is due, and a read that is due later wakes the window with an empty post. So another screen, or a hidden window, reads nothing.
- **Timing, as the React views:** a running turn every second until it ends, then the list once more; after a failed read 2 s, 4 s … up to 30 s, keeping a turn already shown; the runtime a second after each read.
- **A new pick never shows the old turn:** a read for another pick is dropped.

### The formatting helpers (`dev/format.go`)

`format.ts` ported: `Num`, `Duration`, `Size`, `NewScale` and `Tick`, `ChatOptions`, `Parts`, `Share`. The helpers inline in `inspector.tsx` and `runtime.tsx` are here too: `TurnLabel`, `Prompt`, `Counted`, `ProviderState`, `ProviderLimit`, `StuckTitle`, `ProjectLine`.

### New kit pieces

Each in its own file, each with a headless test in light and dark, with the keyboard, an accessible label and Persian text:
- `DataTable`, `DataRow`, `DataSpan`, `DataFoot` (`datatable.go`): shadcn's Table, with tabular digits, a selected row and a footer.
- `Picker` (`picker.go`): shadcn's Select, size sm, on MyGo's `SelectBase`.
- `TimeAxis`, `Lane`, `LaneBar` (`lanes.go`): the timeline's pieces.
- `TitledCard` (`titledcard.go`), `ShareBar` (`sharebar.go`).
- `LinesView` (`linesview.go`): a long text as a virtual list of lines, focusable, with Up, Down, Page Up, Page Down, Home and End. The test builds 20,000 lines and checks that only the lines in view are built.
- `LinkButton` (`linkbutton.go`): shadcn's link button.
- `IconWrench`, `IconActivity` (`icons_dev.go`).

### Shared files

Small and additive, for the sessions working beside this one:
- `shell/shell.go`: the `DevScreen` interface, a `Dev` field, one case in the main area, and `Settings.LoadStarted()` in `Start`.
- `shell/rail.go`: the one *Developer tools* entry; Settings is not marked current on the developer tools' places.
- `chat/rows.go`: *Inspect* in the footer.
- `cmd/desktop-native/main.go`: the store and the screen.
- `cmd/jenab-bench`: `-dev-tools` (see the live check).

## Tests

- **Ported from `format.test.ts`:** `TestTimesAndSizesReadShort`, `TestTheTimelineHasAtMostSixRoundTicks`, `TestEachChatIsPickedOnceNewestFirst`. `TestTheInspectorAndRuntimeLines` covers the other helpers.
- **The store** (`state/dev_test.go`): the newest turn is picked; a running turn is read every second until it ends; a failed read waits longer each time and keeps the turn shown; a new pick never shows the old turn; the runtime is read a second after each read; a block's text or why not.
- **The screens** (`dev/screen_test.go`, headless against `statetest.Dev`, a chat with a Persian title): the inspector at the newest turn and at a turn inspected; choosing request 2; the turn picker and the keyboard; the block's dialog, Escape, an empty block and a block not recorded; the tools off and nothing recorded; *Runtime* with every column, a failed read and the bar back to the inspector; no project open. Light and dark.
- **The frame** (`shell/dev_test.go`): the rail's *Developer tools* opens the inspector, and *Inspect* opens it at that turn; with the tools off, neither shows.
- **Kit:** the tests above, and `TestOnlyTheKitImportsMyGoUI` still passes: `dev` imports only the kit and the stores.
- `go vet ./...` and `go test ./...` pass, 39 packages.
  - In the first full run, `internal/desktop`'s `TestCLIKill` failed once while the bench app was building beside it. It passed alone three times and in the second full run.
  - `TestAFailedReadWaitsLongerEachTime` failed in that run too, which was the test's fault: it read again without moving the clock past the wait. Fixed; it now also checks that no read starts before the wait is over.

## The live check

With the fake model and no keys:

```bash
go build -tags bench -o bin/jenab-native-bench.exe ./cmd/desktop-native
```

```bash
go run ./cmd/jenab-bench -native bin/jenab-native-bench.exe -hold 20m -dev-tools -tokens 300
```

- **`-dev-tools`** (new, with `-hold`): the settings turn the developer tools on, and the fake model answers a message with "use a tool" in it with two `query` calls first, `SELECT 1 AS one` and a query of a table that doesn't exist, then the reply.
- **Steps:** the bench's `/send` sent "Please use a tool to count the rows" to *History 200*. *Inspect* in the turn's footer was pressed through UI Automation (`InvokePattern`), which reached MyGo's button by its name.
- **The inspector showed the turn:** `bench/bench-model · 8.3 s`; two requests (4,978 and 5,069 prompt tokens, 20 and 829 output); the two `query` calls in the Tools lane and the tool-calls table (13 B, and the failed one); the six context blocks of request 1 (Tools, System prompt…, Project card, Earlier turns with its cache point, History window, This turn), 3,743 tokens in all, every one a miss, as the fake model reports no cache.
- ***Runtime*,** opened from the bar during a second turn, showed 1 of 8 slots in use, the provider *bench* OK, the project *Bench* with the turn running ("turn 1, request 2") and its two writers.
- **Screenshots** are in `bin/dev-shots`, uncommitted: the live ones (dark, as the system is set) show a user-name path. `JENAB_DEV_SHOTS=<folder> go test ./internal/native/dev/` writes the headless ones in light and dark: the inspector, request 2, a block's text and *Runtime*.

## Differences from the Wails app, and notes

- **The rail, not Settings,** opens the tools, as said above. When TASK-055 brings Settings, *Settings → Developer* can link to them too.
- **The block dialog** has no close button, as the kit's `Dialog` has none: Escape or a click outside closes it.
- **Bars on the timeline overlap** when two calls run at once or a request takes a few milliseconds, as in the Wails app: in the live turn, request 1 (3 ms) is under request 2's bar.
- **Long block names** end in "…" in the block table; the dialog's title shows the whole name.
- **The rail's current tile:** with the tools open, the wrench tile is marked current for assistive technology, as Settings' tile is, but the kit's `IconButton` draws no mark for it. That is the rail's look today, not this ticket's change.
