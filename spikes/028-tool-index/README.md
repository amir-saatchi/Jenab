# SPIKE-028 harness: the tool index and history cards

Runs scenario sets on the app's real orchestrator, through the scenario
runner (`internal/scenario`), in four conditions:

1. Today: every tool native with its full schema, today's window.
2. The tool index: the core tools (data, memory, `load_skill`, and the
   history tools `search_history` and `read_messages`) stay native;
   every other tool is one line in block 1, `describe_tool(name)` returns
   its schema as a tool result, and `call_tool(name, arguments)` runs it
   (SPEC 8.7's `mcp_describe`/`mcp_call`, SPIKE-024).
3. The index and history cards (long chats only): in a turn after a cut,
   Go adds the older turns that best match the user's message to that
   message, as cards; the recent turns stay as they are (SPEC 3.6).
4. Today's tools and history cards (long chats only): condition 1 with
   condition 3's cards.

The ticket is `Docs/Tickets/SPIKE-028-request-list.md`; the results are in
`results.md`.

```
go run . -sets rare-tools -conds 1 -reps 1 -model scripted:scenarios/rare-tools/_offline.yaml        # no keys
go run . -sets rare-tools -conds 2 -reps 1 -model scripted:scenarios/rare-tools/_offline_index.yaml  # no keys
go run . -long -conds 1,4 -long-reps 1 -model scripted:scenarios/long-chats/_offline.yaml           # no keys
go run . -long -conds 2,3 -long-reps 1 -model scripted:scenarios/long-chats/_offline_index.yaml     # no keys
go run . -sets multi-part,rare-tools -conds 1,2 -reps 3                                             # live
go run . -long -conds 1,2,3,4 -long-reps 5                                                           # live
go run . -sets skill-loading -conds 1,2 -reps 1 -first-rep 2 -out results/<stamp>                    # another day
go run . -report results/<a>,results/<b>                     # results.md again, in <b>; <b>'s runs win
```

The models are gemma4:31b (Ollama Cloud) and ministral-14b (Mistral), as
Main-Session chose: Gemini is left out to save its shared quota, and
Mistral is there because it reports cache reads. Keys come from `-env`
(default `../../.env`; from a worktree, pass `-env` with the main checkout's
`.env`) into memory. A quota error stops the runs of that
provider kind; the rest go on, and the stopped runs are marked. Gemini, if
run with `-model`, is also counted in `results/gemini-budget.json` per
Pacific day and stops at `-gemini-cap` (default 290). Each model runs one
run at a time, the models side by side.

## Hooks in the app (spike-only)

- `agent.Deps.ToolIndex`: text added to block 1 after the prompt rule, for
  a chat of the kind. Nil, or blank text, leaves block 1 as it is
  (`TestToolIndexHook`).
- `scenario.Options.Deps`: changes a run's orchestrator deps before it
  starts: here the registry with the index, `ToolIndex`, and
  `Deps.NoHistoryCards` in conditions 1 and 2.
- `scenario.Options.Call`: maps a call to the call the checks read, so
  `call_tool(X, a)` is checked as a call of X; the transcript shows both
  (`> call call_tool→X {a}`).
- `scenario.Options.Setup`: changes the chat under test before the first
  message: here a long chat's ten earlier turns.

Nil leaves each as it was (`TestDepsAndCallHooks`, `TestSetupHook`).
`spikes/` is outside `./...`; build and test this one by its path:
`go test ./spikes/028-tool-index/`.

## The prototype

- `index.go`: the index. `call_tool` takes the timeout, background flag
  and areas of the tools behind it, so block 1's prompt rule and areas are
  the same as in condition 1; it carries the inner tool's preflight
  (approvals, effects), checks argument names before the tool's own schema
  check (refusing other names only where the schema does), and loads the
  skills whose `load_with` names the inner tool, as the orchestrator does
  after a native call. Mother-only tools are listed and run only in the
  Mother chat. `keepSkills` keeps the skills condition 1 has: the app
  hides a skill whose `load_with` tools are all missing from the registry
  (`Skills.Usable`), and the index's tools are not in it
  (`TestSkillsWithIndex`).
- `redirect.go`: a native call of an index tool gets the app's "there is
  no tool named X"; an index of outside tools would point to `call_tool`.
  With no app hook for that result, the spike rewrites it in the requests
  it sends (the store and transcripts keep the app's text). These calls
  count as "index tool called natively" (`TestRedirect`).
- The cards: since TASK-027 they are the app's own (`internal/agent/cards.go`,
  SPEC 3.6), on unless `Deps.NoHistoryCards` is set; this spike's
  `cards.go`, which gave them through `Deps.Earlier`, was removed. The
  harness records the carded turns from `agent.CardScores` and
  `agent.PickCards` after seeding, as the app finds them at the
  follow-up's first request. Two things differ from the runs in
  `results/`: the app's rule for small words and its thresholds, and a
  card's calls of non-core tools show as `call_tool` in condition 3.
- `history.go`: a long chat's earlier turns, written into the store as the
  app writes them (calls of non-core tools as `call_tool` in conditions 2
  and 3).

## What is measured

- `runs.jsonl`: one line a run. `task` passes the checks but the request
  and prompt-size ones; `done` also needs every tool the checks name to
  have run once without an error (the runner's `tool_called` counts a
  refused call). Call counts come from the transcript, the same way in each
  condition: calls of non-core tools, through `call_tool`, `describe_tool`
  calls, blind calls (`call_tool` before describing), argument errors,
  unknown names (an index tool called natively), and tools that failed and
  later worked.
- `sizes.json`: the core and other tools' definitions against the index,
  as the app estimates tokens (4 bytes a token), on each set's real
  registry.
- Long chats: whether condition 3's card had the turn the follow-up
  needs, and how many cards were added.

## What is faked

The sets' tools are the runner's fakes, with schemas the arguments are
checked against; `read_feed`, `connect_api` and the rest of `rare-tools`
never reach the network. The long chats' fixture (`scenarios/long-chats/`)
has BTC and ETH tables; their earlier turns are in
`scenarios/long-history/`. Every prompt is synthetic.
