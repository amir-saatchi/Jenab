# TASK-043 — More scenario failures both reference models share
**Type:** Task
**Status:** Done
**Gate:** 3
**Needs:** TASK-042

## Goal
The reference suite of 2026-11-10 ran all 20 scenario sets on `gemini-3.5-flash-lite` and `mistral-large-4-1t-a52b` ([REFERENCE-SUITE-report](reports/REFERENCE-SUITE-report.md)). Some failures are the same on both, as in TASK-042. For each, decide whether the check, Jenab or the models are at fault, and fix what is Jenab's.

## The failures
1. **`role-skills` R2, R3, R5, R6:** Mother never gives `pipelines`, `config-guide` or `migrations` to a new chat. Both models 1/6, as was minimax on 2026-10-20. Pipelines don't exist yet (Phase 3): is the set asking for skills the chat can't use?
2. **`acceptance`:** turn 25 gets the right answer (0.35 BTC, €48,200, Bitstamp) without `search_history`. Where does the answer come from now (the window, session notes, memory), and what should the scenario check?
3. **`schema-widen-check`** (and `schema-widen` on mistral): in the Ethereum migration the news key stays `url` instead of `(coin, url)`, the new `prices` table loses `CHECK(price >= 0)`, and mistral gives `coin` a `'BTC'` default. The schema agent writes the migration: check its prompt and the `migrations` skill.
4. **`local-refresh` (info):** the "dates as YYYY-MM-DD" convention never reaches memory, in 5 of 5 runs.

Also look at:
- **`refresh-memory` R1 on mistral:** the refresh answer once had no JSON object, so memory stayed empty. Is Jenab's JSON request sent and honoured on `openai_compatible` (NVIDIA)?
- **TASK-042's open point:** the base prompt doesn't say to load `sql-queries` before writing SQL.

## Rules
- As TASK-042: the reference models decide; fixes are the same for every model; don't loosen a check without saying why the SPEC allows what the model did.
- Gemini: `GEMINI_API_KEY` only, under 300 requests for the ticket, the key under 400 a day (the day resets at 08:00 UTC). NVIDIA: about 40 requests a minute; mistral needs long run limits (25–40 minutes for refresh and acceptance).
- Keep the PC awake during live runs and wait actively.

## Done when
- Each failure has a verdict with the transcript lines that show it.
- The fixes are in, with Go tests where the change is in code, and `jenab-scenarios check scenarios` passes.
- Before and after pass shares on both models for the affected scenarios.
- The report proposes any SPEC text.

## Result
Done on 2026-11-11. SPEC 3.5, 8.1, 8.2 and 8.4 have the new rules. Before and after on both reference models:

- **role-skills** (no migrations, pipelines or config-guide for a new chat): the set. It faked no write tools, so the skill list hid the skills that load with them. The set now fakes `apply_migration`, `save_pipeline` and `save_view`, and `check` rejects a skill assert the chat's skill list can't meet. Gemini 3/18 → 11/18, mistral 2/18 → 7/18.
- **acceptance** (turn 25 right without `search_history`): the check. The history card (3.6) carries turn 3. A new `from_history` assert accepts a card or a history tool, and fails if the window already holds the fact. Gemini 0/1 → 1/1, mistral 0/2 → 1/1.
- **schema-widen** (news key, lost CHECK, `DEFAULT 'BTC'`): Jenab. The main agent designed the migration in its goal, and `describe_table` hid CHECKs. `request_schema_change` now says not to design; the schema agent's prompt and the migrations skill say the design is its own, when to widen and to keep checks; `describe_table` shows CHECKs. mistral swc 0/3 → 2/3, sw 1/3 → 3/3. Gemini swc 2/3 → 2/3: checks and columns are kept now, but the news key is still `url` in 1 of 3.
- **local-refresh** (date format): the check. "For all my chats" goes to user memory; a new `memory_contains` assert reads both memories. 5/5 re-scored, 3/3 after on each model.
- **refresh-memory R1 on mistral** ("no JSON object"): Jenab, and the model. JSON mode is sent and honoured; mistral thinks on NVIDIA even when asked not to, and used the whole 4,096-token cap. A cut-off answer is now named, and its retry gets 8,192 tokens. 0/1 → 1/1.
- **sql-queries before SQL:** the model. Gemini loaded it before the first query in 0 of 16 transcripts, mistral in 8 of 33; 3 of 135 queries without it failed, each fixed in the next call. Left open.
- **Open:**
  - Gemini swc and sw with the last migrations-skill line weren't run (budget).
  - R3: both models give a daily job web-research only, never pipelines; the delegation skill could say that work on a schedule needs pipelines.
  - mistral adds skills that don't fit (R2, R5).
  - A table-level CHECK hides the column checks in `describe_table`.
  - A proposal: `query`'s description could say to load sql-queries before anything beyond a simple SELECT; it needs its own measurement.

The session's report: [reports/TASK-043-report.md](reports/TASK-043-report.md).
