# SPIKE-032: catch made-up numbers

Can a check in the harness find numbers in an answer, or in what `bucket_put` saves, that no source gave, and get the model to correct itself, the same way for every model, without flagging honest answers? See `Docs/Tickets/SPIKE-032-made-up-numbers.md`. The findings are in `results.md`.

## The check

`internal/agent/grounding.go`, spike-only: `agent.Deps.Grounding`, which only the scenario runner sets. The app's zero value has no check.

- At the end of a turn, the numbers in the turn's answer text and in the arguments of its write calls (tools with the `WritesDB`, `Bucket` or `Memory` effect) are checked against the numbers of everything the model was given in the last request: the system blocks (date, card), the tool descriptions, the user's messages, notices, tool results, and the answers of earlier turns whose check didn't fire.
- Numbers are read with Persian and Arabic-Indic digits and separators, thousands separators, decimals, `k`/`M`/`B`/`T` and the words `thousand` to `trillion`. A number matches within the coarser rounding of the two, so 61,240 = 61.2k = 61,240.00 = ۶۱٬۲۴۰.
- Not checked: dates and times, integers under 1000 without a currency sign or a scale, years 1900-2100, list numbering, and numbers glued to a letter (`v3`, `24h`, `r_1`).
- The check fires at K or more numbers without a source.

Round 2 added:
- Numbers a write call saves without a source are kept for the chat. When a tool that reads the project (`ReadsDB`: `bucket_read`, `query`, ...) returns them, they are no source.
- Each value counts once, whatever its format (`2,520.50` in the answer and `2520.50` in the CSV).
- After the notice, numbers it named are left out of the answer's text (an admission quotes them), not out of what is saved.
- After a same_turn notice, what was saved before it and not saved again under the same `key` is checked again (`saved_left`). With numbers from a write call, the notice ends "..., and correct what you saved."
- A percentage matches its fraction: 5.2% = 0.052 (`pct` counts these matches).
- Switches for the ways to cut false alarms (set, scenario, or `-tell-k`, `-tell-saved`, `-compute`): `tell_k` tells the model only at that many numbers, `tell_saved` also for a number in what it saved; the mark stays at K. `compute` adds a line to block 1 asking the model to compute numbers with `query` (only with a `query` tool).

## The modes

- `mark`: the answer gets an `unsourced` notice for the user. `shape` drops it, so the model never reads it; a run is the same as one without the check, plus the verdicts. Any K can be computed afterwards from the counts.
- `same_turn`: the mark, plus an `app_check` notice and one more request in the same turn, past the request cap if need be, without tools if it is the cap's last. Once a turn.
- `next_turn`: the mark, plus the `app_check` notice at the end of the turn, with no extra request; the model reads it with the next message.

The notice, the same for every model:

```
[app check: 3 numbers in your answer appear in no tool result or message, e.g. 2,470.10, 2,530.55, 2,544.20. If no tool gave them, say so or get them with a tool.]
```

With numbers from a write call, "in your answer" is "in your answer or in what you saved".

## Run

The scenarios are in `scenarios/grounding` (G: the web gives part of what is asked; H: honest answers), plus `F2_table`, `F4_view`, `F5_what_is_ma` of `scenarios/missing-features` and `t3_a`, `t3_b`, `t3_d`, `t4_a`, `t4_b`, `t4_d` of `scenarios/multi-part`. The grounding set has `grounding: {mode: mark}`; `-grounding` and `-k` set the mode and K over the sets'.

```
go run ./cmd/jenab-scenarios run -models <models.yaml> -env .env -reps 3 -scale 0.5 -grounding mark -k 3 \
  -scenario G1_eth_week_table,... scenarios/grounding scenarios/missing-features scenarios/multi-part
go run ./cmd/jenab-scenarios run ... -grounding same_turn -k 3 -scenario <the ones where it fired> ...
go run ./cmd/jenab-scenarios run ... -grounding next_turn -k 3 -scenario G5_eth_two_turns,G9_eth_csv_then_chart scenarios/grounding
```

Each run's transcript has a "Number check" section, and the report's JSON has `grounding` (each verdict: unsourced numbers, fired, told, after the notice) and `grounding_extra` (the requests after the notice: count, tokens, seconds). The report's Markdown has a table of the unsourced counts.

The analyzer is its own module, as the other spikes are. From this folder, `go run . -dump <report>.json...` lists each answer's verdict for labelling; with `-labels labels.csv` and no `-dump`, it prints, for each model and K, made-up answers caught and honest answers flagged. `labels.csv` has the hand labels; runs without one get the label of the scenario's `made_up` checks (H scenarios: honest).

Round 2's honest set is `scenarios/grounding-honest` (real data tools on a synthetic September). `go run . -round2 <report>.json...` prints, per condition, the unsourced counts per case and model, the false alarms of each way of telling, and the runs where the model was told.

To re-score old runs with a newer check, without a model: `go run . -script <report folder> -script-out <dir>` writes each model and rep's transcripts as a script, and `go run ./cmd/jenab-scenarios run -model scripted:<script> -grounding <mode> -out <dir> <scenarios>` plays it through the real orchestrator and tools. Use a separate `-out` per script (report names are timestamps to the second). Calls the transcript clipped at 600 bytes are closed where they were cut.

`go test ./internal/agent -run 'Number|Ground|Source|CheckTexts'` checks the check; `go test ./internal/agent -bench CheckNumbers -run x` times it.
