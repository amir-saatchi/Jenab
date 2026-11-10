# TASK-043 — More scenario failures both reference models share
**Type:** Task
**Status:** Open
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
