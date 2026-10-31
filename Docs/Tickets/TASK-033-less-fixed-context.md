# TASK-033 — Less fixed context per request
**Type:** Task
**Status:** Open
**Gate:** 3
**Needs:** P2-13

## Goal
In P2-13's benchmark, Jenab used 1.9-3.8 times the prompt tokens per turn of a plain chat harness, with the same accuracy on the reference models, so N-42 doesn't hold. Most of the gap is fixed: about 4,900 tokens of tool definitions and block 1 in every request (plain: 900), and 2.9-4.1 requests a turn (plain: 1.5-1.6). Cut it without losing accuracy, the same for every model.

## Scope
Measure each change on `scenarios/bench-bitcoin` (`-harness jenab,plain`) and keep what helps:
1. **Tools when needed:** the data tools only once the project has tables, or another rule that shrinks the tool list. The list changes only at a cut, so requests stay append-only (3.1).
2. **Skills:** a loaded skill's text leaves block 1 at a cut once it is no longer used.
3. **Fewer rounds** in a data turn, for example a write that returns what the answer needs.
4. **Standing instructions:** the session notes take a change to one ("add Ethereum to the usual"); in P2-13 they missed it twice.
5. **A longer set:** 3-4 weeks of the same project, written and committed before any scored run, to find the turn where plain gets dearer than Jenab.

## Done when
- On the reference models, 3 reps: Jenab's prompt tokens per turn on `bench-bitcoin` are at least 30 % lower than in P2-13, with checks as good.
- The longer set shows where Jenab gets cheaper than plain, or that it doesn't within 4 weeks.

## Result
