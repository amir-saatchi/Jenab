# SPIKE-011 — FTS5 for German and Persian text
**Type:** Spike
**Status:** Done
**Gate:** 2

## Question
Which FTS5 tokenizer gives good `search_history` results for English, German and Persian text (SPEC 2.3)?

Compare `unicode61` (with `remove_diacritics 2`), `porter unicode61` and `trigram` on:
1. German compound words and umlauts (e.g. search `Preis` finds `Bitcoinpreis`; `Übersicht` vs `Ubersicht`)
2. Persian text: zero-width non-joiner, Arabic vs Persian `ی`/`ي` and `ک`/`ك`
3. Mixed-language messages with code and URLs
4. Index size and query time for 100,000 messages

## Done when
One tokenizer is chosen (or two FTS tables), with any text normalization Go must apply before indexing.

## Result
Run on 2026-09-28 with modernc.org/sqlite v1.59.0 (SQLite 3.53.4). Code and full output: `spikes/011-fts5-languages/` (`results.md`).

**Labelled set:** 33 messages and 26 queries, with 50 wanted hits in total. Each query was normalized like the text, then every term quoted.

| Setup | No normalization | Go normalization, ZWNJ removed, split form also tried |
|---|---|---|
| `unicode61` | 31 of 50 hits | 38 of 50 |
| `unicode61`, prefix query (`"term"*`) | 36 | **45**, 0 false hits, 21 of 26 queries fully right |
| `porter unicode61` | 32 | 39 |
| `trigram` | 38 | 49, 1 false hit (`run` → "truncated"), 25 of 26 |

- **Go normalization matters most** (+7 to +11 hits for every tokenizer). Without it, `ك`/`ي` never match `ک`/`ی`, `2026` misses `۲۰۲۶`, and `Straße` misses `Strasse`.
- **ZWNJ:**
  - ZWNJ → space: `بیت‌کوین` finds "بیت کوین", but `میخواهم` misses "می‌خواهم".
  - ZWNJ removed: the reverse.
  - ZWNJ removed, plus the query also tries the split form for a term with a ZWNJ: all Persian queries right.
- **Prefix queries** beat `porter`: they find `running`, `runs`, `Häuser`… `Haus`, `Bitcoin`… `Bitcoinpreis`. Porter is English-only and misses `ran` too.
- **The 5 misses left with `unicode61` + prefix** are all inside a word (`Preis` in "Bitcoinpreis", `Straße` in "Hauptstraße" ×2, `UserName` in "getUserName"), plus `ran` for `run`. Only `trigram` finds words inside words.
- **Hostile input:** the query builder handled all 17 inputs without an error: `"`, `AND`, `NOT`, `NEAR(`, `col:`, `^`, `-`, `*`, empty…
  - One bug was found and fixed: FTS5 has no implicit AND next to a `( … OR … )` group, so terms are joined with an explicit `AND`.

**100,000 messages** (synthetic; 48.5 MB of text, 508 bytes average; contentless tables; top 20 by `bm25`):

| | Build | Index size | Query p50 / p95 / max | Queries with no hit |
|---|---|---|---|---|
| `unicode61` | 3.1 s | 20 MB (0.42 × text) | 1.7 / 25 / 42 ms | 2 of 60 |
| `porter unicode61` | 3.4 s | 20 MB | 2.2 / 27 / 42 ms | 2 of 60 |
| `trigram` | 21 s | 131 MB (2.7 × text) | 2.5 / 36 / 134 ms | 16 of 60 (terms under 3 characters) |
| No index, `LIKE '%term%'` scan | — | 0 | 114 / 158 / 166 ms | — |

**Findings**
- `trigram` finds 4 more of the 50 wanted hits, but its index is 6.5 times larger and takes 7 times longer to build. It also finds nothing for terms under 3 characters, and many Persian words are that short (`می`, `از`).
- For words inside words, a plain `LIKE` scan costs about 0.1–0.17 s per 100,000 messages, and grows linearly with message count. That is fine as an explicit option, but too slow for every search.
- The index text is normalized, so it differs from the stored text. An external-content table would then give wrong snippets and deletes. A contentless table with `contentless_delete = 1` works, and Go builds the snippets from the original text.
- Not measured: over 100,000 messages, and real (not synthetic) chat text.

## Decision
**Decided (2026-09-28):**
- **One FTS table:** `messages_fts`, `unicode61 remove_diacritics 2`, contentless (`content = ''`, `contentless_delete = 1`), rowid = the text part's rowid.
- **Normalization in Go,** the same for indexed text and queries:
  - NFKC
  - `ي ى` → `ی`, `ك` → `ک`
  - remove tatweel and Arabic diacritics
  - Persian and Arabic-Indic digits → ASCII
  - `ß` → `ss`
  - remove ZWNJ
- **Query builder:**
  - Split on spaces, quote each term (doubling `"`) and add `*`; join with `AND`.
  - A term with a ZWNJ becomes `("joined"* OR "split form"*)`.
  - An empty query does no search.
  - Agent and user text never reaches `MATCH` any other way.
- **Snippets** are built in Go from the stored text.
- **"Inside words"** (`search_history` option `substring: true`, and a UI toggle) runs a `LIKE` scan within the scope instead, capped at 500 ms. After that it returns what it found with a note.
- **No porter and no trigram** for v1.
