# SPIKE-011 results

modernc.org/sqlite v1.59.0 (SQLite 3.53.4), Go go1.26.0, 2026-09-28.

## 1. Labelled set

33 messages (German, English, Persian, mixed; some only there to catch false hits) and 26 queries with the messages a user would want. The query is normalized like the text, split on spaces, and every term quoted (`"term"`, or `"term"*` for prefix).

| Setup | Normalization | Wanted hits found | False hits | Queries fully right |
|---|---|---|---|---|
| unicode61 | none | 31 of 50 | 0 | 13 of 26 |
| unicode61, prefix query | none | 36 of 50 | 0 | 15 of 26 |
| porter unicode61 | none | 32 of 50 | 0 | 13 of 26 |
| trigram | none | 38 of 50 | 1 | 16 of 26 |
| unicode61 prefix ∪ trigram | none | 39 of 50 | 1 | 17 of 26 |
| unicode61 | Go, ZWNJ → space | 38 of 50 | 0 | 17 of 26 |
| unicode61, prefix query | Go, ZWNJ → space | 43 of 50 | 0 | 19 of 26 |
| porter unicode61 | Go, ZWNJ → space | 39 of 50 | 0 | 17 of 26 |
| trigram | Go, ZWNJ → space | 48 of 50 | 1 | 24 of 26 |
| unicode61 prefix ∪ trigram | Go, ZWNJ → space | 48 of 50 | 1 | 24 of 26 |
| unicode61 | Go, ZWNJ removed | 37 of 50 | 0 | 16 of 26 |
| unicode61, prefix query | Go, ZWNJ removed | 44 of 50 | 0 | 20 of 26 |
| porter unicode61 | Go, ZWNJ removed | 38 of 50 | 0 | 16 of 26 |
| trigram | Go, ZWNJ removed | 48 of 50 | 1 | 24 of 26 |
| unicode61 prefix ∪ trigram | Go, ZWNJ removed | 48 of 50 | 1 | 24 of 26 |
| unicode61 | Go, ZWNJ removed; query term with ZWNJ also tries the split form | 38 of 50 | 0 | 17 of 26 |
| unicode61, prefix query | Go, ZWNJ removed; query term with ZWNJ also tries the split form | 45 of 50 | 0 | 21 of 26 |
| porter unicode61 | Go, ZWNJ removed; query term with ZWNJ also tries the split form | 39 of 50 | 0 | 17 of 26 |
| trigram | Go, ZWNJ removed; query term with ZWNJ also tries the split form | 49 of 50 | 1 | 25 of 26 |
| unicode61 prefix ∪ trigram | Go, ZWNJ removed; query term with ZWNJ also tries the split form | 49 of 50 | 1 | 25 of 26 |

### Per query, with Go normalization (ZWNJ → space)

Cell: wanted hits found / wanted, then false hits as `+n`. **Bold** = fully right.

| Group | Query | Wanted | unicode61 | unicode61, prefix query | porter unicode61 | trigram | unicode61 prefix ∪ trigram |
|---|---|---|---|---|---|---|---|
| German | `Preis` | d1 d2 d8 d9 | 1/4 | 3/4 | 1/4 | **4/4** | **4/4** |
| German | `Übersicht` | d2 d3 | **2/2** | **2/2** | **2/2** | **2/2** | **2/2** |
| German | `Ubersicht` | d2 d3 | **2/2** | **2/2** | **2/2** | **2/2** | **2/2** |
| German | `Straße` | d4 d5 | 1/2 | 1/2 | 1/2 | **2/2** | **2/2** |
| German | `Strasse` | d4 d5 | 1/2 | 1/2 | 1/2 | **2/2** | **2/2** |
| German | `Haus` | d6 d7 | 1/2 | **2/2** | 1/2 | **2/2** | **2/2** |
| German | `Bitcoin` | d1 | 0/1 | **1/1** | 0/1 | **1/1** | **1/1** |
| German | `coin_prices` | d10 | **1/1** | **1/1** | **1/1** | **1/1** | **1/1** |
| English | `run` | e1 e2 e3 | 1/3 | 2/3 | 2/3 | 2/3 +1 (e8) | 2/3 +1 (e8) |
| English | `search_history` | e4 p9 | **2/2** | **2/2** | **2/2** | **2/2** | **2/2** |
| English | `coindata` | e5 m1 | **2/2** | **2/2** | **2/2** | **2/2** | **2/2** |
| English | `UserName` | e6 | 0/1 | 0/1 | 0/1 | **1/1** | **1/1** |
| English | `SQLITE_BUSY` | e7 | **1/1** | **1/1** | **1/1** | **1/1** | **1/1** |
| English | `locked` | e7 | **1/1** | **1/1** | **1/1** | **1/1** | **1/1** |
| Persian | `میخواهم` | p1 p2 | 1/2 | 1/2 | 1/2 | 1/2 | 1/2 |
| Persian | `می‌خواهم` | p1 p2 | 1/2 | 1/2 | 1/2 | **2/2** | **2/2** |
| Persian | `قیمت` | p1 p5 p8 m2 | **4/4** | **4/4** | **4/4** | **4/4** | **4/4** |
| Persian | `كتاب` | p3 p4 p7 | **3/3** | **3/3** | **3/3** | **3/3** | **3/3** |
| Persian | `کتاب` | p3 p4 p7 | **3/3** | **3/3** | **3/3** | **3/3** | **3/3** |
| Persian | `بیت‌کوین` | p1 p10 | **2/2** | **2/2** | **2/2** | **2/2** | **2/2** |
| Persian | `۲۰۲۶` | p6 | **1/1** | **1/1** | **1/1** | **1/1** | **1/1** |
| Persian | `2026` | p6 | **1/1** | **1/1** | **1/1** | **1/1** | **1/1** |
| Persian | `طلا` | p8 | **1/1** | **1/1** | **1/1** | **1/1** | **1/1** |
| Mixed | `BTC` | e5 m2 m3 | **3/3** | **3/3** | **3/3** | **3/3** | **3/3** |
| Mixed | `avg` | m3 | **1/1** | **1/1** | **1/1** | **1/1** | **1/1** |
| Mixed | `API-Key` | m1 | **1/1** | **1/1** | **1/1** | **1/1** | **1/1** |

### Persian queries by normalization (unicode61, prefix query)

| Query | none | ZWNJ → space | ZWNJ removed | ZWNJ removed, query also tries split |
|---|---|---|---|---|
| `میخواهم` | 1/2 | 1/2 | 2/2 | 2/2 |
| `می‌خواهم` | 1/2 | 1/2 | 2/2 | 2/2 |
| `قیمت` | 3/4 | 4/4 | 4/4 | 4/4 |
| `كتاب` | 1/3 | 3/3 | 3/3 | 3/3 |
| `کتاب` | 1/3 | 3/3 | 3/3 | 3/3 |
| `بیت‌کوین` | 2/2 | 2/2 | 1/2 | 2/2 |
| `۲۰۲۶` | 1/1 | 1/1 | 1/1 | 1/1 |
| `2026` | 0/1 | 1/1 | 1/1 | 1/1 |
| `طلا` | 1/1 | 1/1 | 1/1 | 1/1 |

## 2. Query builder with hostile input

Each input goes through `ftsQuery` and then `MATCH`, on unicode61 (prefix), trigram, and unicode61 with `ftsQueryZWNJ` (plus the Persian inputs `می‌خواهم "` and `بیت‌کوین OR`).

| Input | FTS5 query | unicode61 | trigram | ftsQueryZWNJ |
|---|---|---|---|---|
| `"` | `""""*` | ok, 0 rows | ok, 0 rows | ok, 0 rows |
| `AND` | `"AND"*` | ok, 1 rows | ok, 1 rows | ok, 1 rows |
| `NOT` | `"NOT"*` | ok, 1 rows | ok, 1 rows | ok, 1 rows |
| `x OR` | `"x"* "OR"*` | ok, 1 rows | ok, 0 rows | ok, 1 rows |
| `a*b` | `"a*b"*` | ok, 1 rows | ok, 0 rows | ok, 1 rows |
| `(` | `"("*` | ok, 0 rows | ok, 0 rows | ok, 0 rows |
| `NEAR(a b` | `"NEAR(a"* "b"*` | ok, 1 rows | ok, 0 rows | ok, 1 rows |
| `-btc` | `"-btc"*` | ok, 1 rows | ok, 0 rows | ok, 1 rows |
| `^preis` | `"^preis"*` | ok, 1 rows | ok, 0 rows | ok, 1 rows |
| `col:btc` | `"col:btc"*` | ok, 0 rows | ok, 0 rows | ok, 0 rows |
| `` | `` | not run (empty query) | not run (empty query) | not run (empty query) |
| `   ` | `` | not run (empty query) | not run (empty query) | not run (empty query) |
| `'` | `"'"*` | ok, 0 rows | ok, 0 rows | ok, 0 rows |
| `\` | `"\"*` | ok, 0 rows | ok, 0 rows | ok, 0 rows |
| `{}` | `"{}"*` | ok, 0 rows | ok, 0 rows | ok, 0 rows |
| `می‌خواهم "` | `"می‌خواهم"* """"*` | ok, 0 rows | ok, 0 rows | ok, 0 rows |
| `بیت‌کوین OR` | `"بیت‌کوین"* "OR"*` | ok, 0 rows | ok, 0 rows | ok, 0 rows |

## 3. 100000 messages

Synthetic messages: 50% English-like, 25% German-like (umlauts, compounds), 25% Persian-like (ZWNJ joins), about 5% with code or URLs. Word frequencies follow a Zipf curve. Average 508 bytes of text per message, 48.5 MB in total. Normalized with ZWNJ → space. Contentless tables (`content = ''`, `contentless_delete = 1`), so the size is the index alone; the text itself stays in the messages table. Top 20 by `bm25`, run on a warm cache.

| Tokenizer | Build (500-row transactions) | Index size | Index / text | Query p50 | p95 | max | Queries with no hit |
|---|---|---|---|---|---|---|---|
| unicode61 remove_diacritics 2 | 3.1 s | 20.3 MB | 0.42 | 1.7 ms | 25.3 ms | 42.1 ms | 2 of 60 |
| porter unicode61 remove_diacritics 2 | 3.4 s | 20.2 MB | 0.42 | 2.2 ms | 27.2 ms | 42.1 ms | 2 of 60 |
| trigram remove_diacritics 1 | 21.2 s | 131.4 MB | 2.71 | 2.5 ms | 36.0 ms | 133.8 ms | 16 of 60 |
| no index: `body LIKE '%term%'` over all rows (count) | — | 0 | 0 | 113.7 ms | 158.0 ms | 165.9 ms | — |

Queries: 60 terms drawn from the three vocabularies (frequent, middle and rare words, some two-word queries, and some two-letter Persian words).
