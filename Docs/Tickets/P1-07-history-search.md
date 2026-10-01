# P1-07 — History search
**Type:** Feature
**Status:** Open
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
