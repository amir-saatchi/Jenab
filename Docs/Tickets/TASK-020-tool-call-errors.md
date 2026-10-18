# TASK-020 — Tool-call errors: measure them, then check more
**Type:** Task
**Status:** Open (later)
**Gate:** 3

## What there is
Every call is checked before its tool runs, and a wrong call goes back to the model as an error result it can fix (SPEC 8.3):
1. **Not a JSON object** (broken JSON, an array, a string): the parser's error and the byte where it broke (`invalidArgs` in `internal/agent/turn.go`). A call cut at MaxTokens is dropped.
2. **No such tool**, or a Mother-only tool in another chat: "there is no tool named X".
3. **Schema:** `tool.Func` checks the arguments against the tool's JSON Schema, then decodes them into its Go struct (`decodeArgs` in `internal/tool/tool.go`). Each error has its path: "wrong arguments: - at /rows/0/price: …".
4. **The tool's own mistakes** (a missing file, a blocked URL, a timeout) are a `tool.Error`. Other failures are logged and reported as "X failed: …".

Three requests in a row with only calls that couldn't run stop the turn. Providers' strict mode is off: the check is ours, the same for every model.

## Goal
Find out how often models send calls that can't run, and add checks only where they help.

## Scope
1. **Measure first.** From the scenario traces, count bad calls per model, tool and kind (JSON, name, schema, tool error), and how often the next call fixes it.
2. **Check what gets saved.** `bucket_put` by file extension: CSV rows with the header's column count, JSON and YAML that parse. The error says where: "line 14 has 5 columns, the header has 4".
3. **Checks across fields and against the project**, in each tool as a `tool.Error`: a start date after the end, a table or chat that doesn't exist, SQL that doesn't parse. Mostly Phase 2 tools.
4. **Better error text**, if the measure shows models failing to fix a call: add the expected type or the allowed values ("use one of: daily, weekly").

Not in scope: fixing slips silently ("5" for 5). It hides the mistake and changes what the model sent.

## Done when
There is a count of bad calls per model and kind, and each check added comes with a scenario that shows it helps.

## Result
