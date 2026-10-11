# TASK-011 — Prompts that match what the app can do
**Type:** Task
**Status:** Open
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
