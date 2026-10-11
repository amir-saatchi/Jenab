# TASK-011 — Prompts that match what the app can do
**Type:** Task
**Status:** Done
**Gate:** 3

## Goal
Block 1 tells the model only about what it can do in this build, and gives it today's date. Found in the harness review of 2026-10-10.

## Today
Phase 1 has 12 tools: `search_history`, `read_messages`, `list_chats`, `read_ref`, `search_ref`, `fetch_page`, `bucket_list`, `bucket_read`, `bucket_put`, `bucket_delete`, `ask_user` and `load_skill` (`cmd/desktop/main.go:121-123`).

- **`prompts/rule.md`** says "Start slow work in the background. Say what is still running. Give the rest in the finish turn." No tool starts background work, and there is no finish turn. This invites the model to say work is running when nothing is, which is what SPIKE-023's `no_false_running` check catches.
- **`prompts/mother.md`** says Mother "directs the other chats". Mother has only `list_chats`; `create_chat` and `send_to_chat` come in Phase 5.
- **`prompts/base.md`** names "tables in a local database, pipelines that fetch data, and views". No tool reads or makes them yet, and there is no project card.
- **The `config-guide` skill** is listed in every chat. It says "Load before save_view or save_page" and points to `read_config`, `describe_table` and `query`, none of which exist. Its `load_with` can never fire.
- **No date.** The model doesn't know today's date, so "this week" and "the latest" are guesses. Pipelines have `today()`; chats don't.

## Scope
- **Tie prompt text to the tools that provide it**, so it can't drift again. The proposal:
  - `rule.md` goes in only when a background tool is registered.
  - Mother's delegation lines go in only with `create_chat`.
  - `base.md` says what the app is without listing what it can't do yet.
  - The text is the same for every model (SPEC 1).
- **Skills.** A skill whose `load_with` tools are all missing is left out of the skill list.
- **Date.** Today's date and the user's time zone go in block 1, built at each cut.
  - A cut comes after 5 minutes idle, so a new day almost always starts with one, and the cache isn't hurt.
  - It holds the date only, not the time, which changes every turn.
  - SPEC 3.1 is updated.
- **Language.** `base.md` doesn't say to answer in the user's language; the title prompt does. First run a Persian scenario on the reference models. Add a rule only if they don't answer in Persian.

## Done when
- A test shows block 1 with the Phase 1 tools has no `rule.md` lines, no delegation lines, and no `config-guide`.
- A test registers a background tool and shows the lines come back.
- Block 1 has the date, and a test checks it changes at a cut on a new day.
- multi-part t4 and the skill-loading set on the reference models are no worse than the last report.
- `go test ./...` passes.

## Result
Block 1 now has only lines its tools back up, plus the date (SPEC 3.1).
- **Prompt lines follow the tools.**
  - `tool.Spec` has `Background`. `rule.md` goes in only when a tool has it.
  - Mother's delegation lines (SPEC 8.6, v1) are in `prompts/delegate.md` and go in only with `create_chat`. `mother.md` no longer says Mother directs the other chats.
  - The scenario runner marks its background tools, so the multi-part set still gets the rule.
- **Skills.** `skill.Set.Usable` leaves out a skill whose `load_with` tools are all missing. The agent uses it everywhere, so `config-guide` is gone from Phase 1 chats (SPEC 8.9).
- **Date.** Block 1 has "Today is Sunday, 2026-10-11, in the user's time zone (UTC+02:00)." It is built at each cut. The time zone is an offset, as Windows has no IANA name for it. `agent.Deps.Now` is the clock. Scenario sets give their fixture's day with `today:`, since the cards say "Today is 2026-09-29". Without it, gemma backfilled the "missing" days in t4_b and t4_d.
- **base.md kept as it was.** Rewording it without tables, pipelines and views cut gemma's L02 from 12/12 (old code) to 10/21. With the old line it was 14/16. The line says what the app is for; the tools say what it can do now.
- **Language.** A Persian set (a knowledge question, a data question with a tool call, Mother) passed 3/3 on both reference models, all answered in Persian. No rule was added.
- **Live runs,** 1 rep:

  | Set | gemini-3.5-flash-lite | gemma4:31b | gemma4:31b, last report |
  |---|---|---|---|
  | multi-part t4 | 4/4 | 4/4 | 3/4 |
  | skill-loading | 11/12 | 12/12 | 12/12 |

  Gemini's miss is L04, which answers "why does this SQL fail" without sql-queries. It failed 3/4 on the old code too, like glm in SPIKE-025.
- **Tests:** `TestBlockOnePhase1` (no rule, delegation lines or config-guide with the Phase 1 tools), `TestBlockOneFollowsTools` (they come back with a background tool, `create_chat` and `save_view`), `TestDateAtCut` (the date changes at a cut on a new day, and not before it), and the set's day in `TestRunSet`.
