# SPIKE-036 — A plan the agent keeps, when the task needs one
**Type:** Spike
**Status:** Open (later)
**Gate:** 3
**Needs:** SPIKE-035

## Question
SPIKE-035 tested a plan as an approval check and dropped it for v1. This spike asks the other question: does a written plan help the work itself?

The agent decides when a task needs a plan (a request with several steps, a long job) and keeps it with a `plan` tool. The user sees it in the chat with each step's state, and the agent works from it across requests and cuts. A one-step question gets no plan.

1. **When:** does the agent plan when it should, and skip it when it shouldn't, with the same tool description for every model (SPEC 1)?
2. **Help:** do multi-step tasks succeed more often with a plan (the `multi-part` set, longer Viso-style or schema tasks), and with fewer requests?
3. **Across cuts:** does a kept plan carry the work past a cut (3.6) better than session notes (3.4) alone?
4. **The user:** is a plan with step states clearer in the chat than today's tool chips? Can the user edit or approve it before work starts?
5. **From SPIKE-028** (moved on 2026-10-27): a request list. `plan(requests)` gives each request `id`, `ask`, `status` (`open`, `running`, `done`, `blocked`, `dropped`), `note`, `after` and `result` (a ref), and the list goes back into every call. The app won't end a turn while a request is `open` or `running`; `blocked` and `dropped` need a reason; if the model stops early, the app sends one more request naming the open items. A simple question gets no `plan` call. Also a `Plan` message part (P1-19), and a plan that shows early when a step has no tool behind it (TASK-014). SPIKE-023's 88% with the prompt rule (8.3) is the bar.
6. **Overlap:** how does it relate to session notes' "the plan" (3.4), SPIKE-028's request list, SPIKE-029 subjects, SPIKE-031's thoughts graph, and SPIKE-034 topics? One place for a plan, not three.

## Done when
Results on the reference models (gemma4:31b and gemini-3.5-flash-lite) with at least 3 reps per cell, and a decision: a `plan` tool and chat part in SPEC 8.1 and 8.3, merged into session notes, or not in v1.

## Result
_Pending._

## Decision
_Pending._
