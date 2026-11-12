# TASK-047 — Keep the start of each request the same
**Type:** Task
**Status:** Open
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
