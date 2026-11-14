# TASK-047 — Keep the start of each request the same
**Type:** Task
**Status:** Done
**Gate:** 3
**Needs:** TASK-046

## Goal
Every request starts with the system blocks and the tools, then the chat. Jenab rewrites that start often:
- at each cut, which rebuilds the date, memory, notes and "Earlier turns";
- when a migration changes the card or the tool list;
- when a long turn's tool results are stubbed;
- in the plain harness, with each new date.

Two things are lost each time:
- **Reasoning.** Newer Claude models drop every thinking block after the change (TASK-046). In the bench runs nearly every request dropped all earlier thinking, because the date changed each turn ([reports/TASK-046-report.md](reports/TASK-046-report.md), section 3).
- **The prompt cache,** on every provider. A rewritten start is written to the cache again, and cache writes were the biggest part of Haiku 5.5's prompt cost ([reports/HAIKU-PASS-report.md](reports/HAIKU-PASS-report.md)).

Change the start as little as possible, the same way for every model.

## Scope
1. **Measure first.** On one reference model and Haiku 5.5, with the runner's `-usage-log`: cache read against cache write, and dropped thinking blocks per run, on bench-bitcoin, acceptance and session-notes.
2. **Changes as messages.** Keep the system blocks the same between cuts. Send card, tool-list and date changes as a notice message at the end, the way memory notices work (3.1). The plain harness sends the date change as a message too.
3. **Tools.** Keep the full tool list from the cut. A tool that needs a missing table says so when it's called, instead of being offered later.
4. **Fewer cuts.** A cut caused only by idle time keeps the window start and the system blocks when nothing in them changed.
5. **At a real cut,** leave out the thinking of the turns before it, in a model-neutral way. Stale blocks then aren't sent and dropped again in every later request.
6. **Count breaks, not entries.** Record the first dropped block's path per request, so one break isn't counted again in each later request.
7. **Measure again,** the same runs as step 1, and give the pass rates before and after.
8. **SPEC:** 3.1, 3.6 and 3.8 as they change.

## Rules
- The same prompts, loop and settings for every model (SPEC 1).
- No pass rate may drop on the reference models; a drop needs a reason in the report.
- Live runs on the paid Anthropic key need Main-Session's OK and a cap.

## Done when
- Go tests show the system blocks staying the same between cuts, with card, tool and date changes as messages.
- The before/after numbers for steps 1 and 7, in the report.
- SPEC updated.

## Result
Done on 2026-11-14. SPEC 3.1, 3.6, 3.8 and 8.10 have the rules.

- **The change:** blocks 1–5 and the tool list stay as the cut built them. A new card, a new date and the last-request line come as notes at the end, and stay in place until the next cut. An idle return cuts only when something in the blocks or the tool list changed. At a cut, the thinking of earlier turns is left out, for every model. The runner counts breaks, not only dropped blocks. All of it is in `internal/agent`; the provider layer only reports the first dropped block's path.
- **Kept:** `query` and `describe_table` still join at the project's first table. Offered from the start, `query` made mistral build a notes table instead of calling `bucket_put` (missing-features F6).
- **Haiku 5.5:** read share in the bench sets went from about 70 % to 84–96 %. Dropped thinking went from 235–661 blocks (8–14 breaks) to 0–4 blocks (0–1 break). Bench cost fell 10–57 %. All 6 sets passed after. Spend $0.461 of the $1.00 cap.
- **mistral-large-4 (NVIDIA):** read share in the bench sets went from 67–72 % to 85–95 %, and uncached input fell 39–78 %. Every set passes on the final code except schema-design (1/3; the before binary failed the same column naming) and refresh-memory R1 (0/2, as in the baseline). bench-bitcoin needed a rerun after an NVIDIA outage.
- **Open:**
  - In-turn trim and series stubs (3.7) still change earlier messages of a very long turn.
  - The first table changes the tool list once: 1 break, 2–4 dropped blocks on Haiku.
  - The plain harness now takes a new day as a note too; it only matters in runs that cross midnight.
  - The card's row counts can be older than before, until the next cut.
  - Most numbers come from one run per binary.

The session's report: [reports/TASK-047-report.md](reports/TASK-047-report.md).
