# Blind verdicts, round 2 (2026-10-10)

Ten readers (Claude subagents, one per pair) each read one pair file, without the key, the database or any other file. They were told length is not quality. After all ten had answered, the verdicts were matched to `key.json`. `index` is the graph's answer; the other side is the one-call answer the judges scored higher.

| Pair | Task | Builder | A | B | Winner | Score A / B | Longer | Length decided it? |
|---|---|---|---|---|---|---|---|---|
| 01 | db-move | gemini-3.5-flash-lite | index (2,645) | plain (4,905) | plain | 5 / 7 | B | no |
| 02 | bakery | gemini-3.5-flash-lite | index (2,912) | plain (4,253) | plain | 6 / 7 | B | no |
| 03 | umzug | gemini-3.5-flash-lite | index (3,091) | plain (4,947) | plain | 4 / 7 | B | no |
| 04 | b1-exam | gemma4:31b | thinking (4,471) | index (3,003) | thinking | 8 / 5 | A | no |
| 05 | db-move | gemma4:31b | thinking (4,340) | index (2,620) | thinking | 7 / 5 | A | no |
| 06 | report-page | gemma4:31b | index (2,358) | thinking (4,331) | thinking | 6 / 8 | B | no |
| 07 | umzug | gemma4:31b | thinking (4,131) | index (3,143) | thinking | 7 / 6 | A | no |
| 08 | report-page | gemini-3.5-flash-lite | index (2,273) | plain (4,877) | plain | 5 / 8 | B | no |
| 09 | bakery | gemma4:31b | index (2,042) | thinking (3,611) | thinking | 5 / 7 | B | no |
| 10 | b1-exam | gemini-3.5-flash-lite | index (3,249) | plain (5,050) | plain | 6 / 8 | B | no |

Lengths are in characters. **The graph lost 10 of 10, by a mean of 5.3 against 7.4.** The longer answer won every pair, but every reader said length didn't decide it. The reasons they gave are about content:

- **The graph's answer is an outline; the one-call answer is a procedure.**
  - db-move (01, 05): the graph's answers had no commands. The one-call answers had the pg_hba line, the `pg_basebackup -R -X stream` command, an LSN check, bandwidth math and the DNS-TTL step.
  - bakery (02): the graph's answer had no split of the 50 million toman budget.
  - report-page (06, 08): the graph's answers had no SQL or Go, and missed checking the API side (JSON encoding, N+1 queries).
- **The graph's answer is narrow where the task needs breadth.**
  - umzug (03): the graph's answer was almost only about booking movers. It left out registering the new address, school and Kita, no-parking zones and mail forwarding.
  - This fits how conclude works: it sees only the two strongest paths in full.
- **The graph's answer is usually more cautious.** It had fewer or no factual errors in 6 of 10 pairs (01, 02, 03, 06, 08, 09), and often the better risk section. It had the worse errors in 4 (04, 05, 07, 10).

The ten verdicts, with their reasons, are in the session transcript of 2026-10-10.
