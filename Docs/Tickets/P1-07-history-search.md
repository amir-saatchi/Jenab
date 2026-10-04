# P1-07 — History search
**Type:** Feature
**Status:** Done
**Gate:** 3 (Phase 1)
**Needs:** P1-06
**Requirements:** R-17, N-09

## Goal
Find anything said earlier in a chat or a project (SPEC 2.3, SPIKE-011).

## Scope
- `messages_fts` with the tokenizer and options from 2.3.
- The Go normalization, used for indexing and for queries.
- The query builder: quoted terms with `*`, joined with `AND`, the ZWNJ rule. No other text reaches `MATCH`.
- Snippets built in Go from the stored text.
- `substring: true`: a `LIKE` scan that stops at 500 ms and says so.
- Scope: one chat or every chat of the project.
- The functions behind `search_history` and `read_messages`; the tools are registered in P1-09.

## Done when
- SPIKE-011's English, German and Persian cases pass as permanent tests.
- The query builder has a fuzz test (Q34).
- p95 is under 50 ms at 100,000 messages on the dev machine (N-09).

## Result
- **`store/search.go`:** `ChatsDB.Search` with `SearchReq`, `Hit` and `SearchResult`, and `Normalize`. `read_messages` uses `Messages` by turn (P1-06).
- **Index:** chats.db format step 2 adds `messages_fts` and a trigger that removes a part's tokens with the part, so *Clear* and deleting a chat clean the index. Text parts written before the step are indexed by it. `AppendMessage` and `AppendPart` index text parts in the same transaction.
- **Changes from the plan:**
  - **Chat column:** the chat ID is a second column in the index. Filtering by a join made one-chat searches slow.
  - **Ranking:** bm25 orders only the newest 2,000 matches. Ranking every match took 100–160 ms for a common word, since it costs about 5 µs a match.
  - **Inside words:** a scan in Go, not `LIKE`, so it uses the same normalization (ي and ی, ß and ss, case beyond ASCII). It does a full pass over 100,000 messages in about 0.65 s, so large projects reach the 500 ms stop. It reads only `message_parts` and looks up messages only for hits.
- **Query builder:** control characters also split words; a NUL in a query broke FTS5's parser, and the fuzz seeds found it. At most 32 words (`ErrBadQuery`).
- **Speed, 100,000 messages (`JENAB_PERF_TEST=1`):** whole words 8 ms p50 and 17 ms p95; one chat 28 ms p95. Inside words 22 ms p50; 7 of 60 queries reach the stop.
- **Tests:**
  - SPIKE-011's 26 queries plus two with several words, in both modes. Each case lists what each mode finds. Word search misses only words inside words (Bitcoinpreis, Hauptstraße, getUserName) and "ran" for "run". Inside words finds those but not "ran", and also finds "brunch" for "run".
  - Hostile inputs, `FuzzFTSQuery` (147,000 runs, no FTS5 error), scope, limits, ranking and its bound, snippets, the scan's stop, *Clear* and delete, and the format step on an old file.
  - Mutation checks: 54 on the index, normalization, query builder, ranking, scan, fold and snippets. 40 made the tests fail at first; 11 missed ones got tests (ranking cuts, the 100-hit cap, the scan's scope, dedupe and snippets, fold ranges, snippet cuts, non-text parts kept out of the index). The other three changed nothing, so that code was removed or kept as it is: a ranking weight for the chat column, a mark filter that a later step repeats, and a check in the fast JSON path.
