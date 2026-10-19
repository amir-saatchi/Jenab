# SPIKE-033 results

Written 2026-10-19 11:59 by the harness. Per-query ranks and top 5: `queries.jsonl`.

## Corpus

- Messages: 300, 38012 bytes of text.
- messages queries: 60 (15 same words, 15 paraphrase, 15 cross language, 15 no answer).
- files/fixed: 392 chunks over 242.2 pages (1800 characters a page).
- files/fixed queries: 30 (8 same words, 8 paraphrase, 7 cross language, 7 no answer).
- files/heading: 687 chunks over 242.2 pages (1800 characters a page).
- files/heading queries: 30 (8 same words, 8 paraphrase, 7 cross language, 7 no answer).

## Models

- granite: builtin `granite-embedding-107m-multilingual`, 384 wide, Matryoshka false, prefixes false.
- granite-reftok: cached `granite-embedding-107m-multilingual`, 384 wide, Matryoshka false, prefixes false.

## Recall@5 and MRR

No-answer queries are left out. Recall@5 is the share of queries with a target in the top 5; MRR counts a target past rank 50 as 0.

| Set | Model | Form | Condition | n | R@5 | MRR | en | de | fa | same words | paraphrase | cross-language |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| messages | — | — | fts | 45 | 33% | 0.32 | 33% | 33% | 33% | 100% | 0% | 0% |
| messages | granite | full | vector | 45 | 82% | 0.66 | 87% | 87% | 73% | 100% | 73% | 73% |
| messages | granite | full | hybrid | 45 | 82% | 0.69 | 87% | 87% | 73% | 100% | 73% | 73% |
| messages | granite | int8 | vector | 45 | 87% | 0.66 | 100% | 87% | 73% | 100% | 73% | 87% |
| messages | granite | int8 | hybrid | 45 | 87% | 0.70 | 100% | 87% | 73% | 100% | 73% | 87% |
| messages | granite-reftok | full | vector | 45 | 84% | 0.68 | 93% | 87% | 73% | 100% | 73% | 80% |
| messages | granite-reftok | full | hybrid | 45 | 84% | 0.72 | 93% | 87% | 73% | 100% | 73% | 80% |
| messages | granite-reftok | int8 | vector | 45 | 84% | 0.68 | 93% | 87% | 73% | 100% | 73% | 80% |
| messages | granite-reftok | int8 | hybrid | 45 | 84% | 0.72 | 93% | 87% | 73% | 100% | 73% | 80% |
| files/fixed | — | — | fts | 23 | 26% | 0.26 | 38% | 25% | 14% | 75% | 0% | 0% |
| files/fixed | granite | full | vector | 23 | 65% | 0.54 | 88% | 62% | 43% | 100% | 38% | 57% |
| files/fixed | granite | full | hybrid | 23 | 65% | 0.56 | 88% | 62% | 43% | 100% | 38% | 57% |
| files/fixed | granite | int8 | vector | 23 | 65% | 0.54 | 88% | 62% | 43% | 100% | 38% | 57% |
| files/fixed | granite | int8 | hybrid | 23 | 65% | 0.56 | 88% | 62% | 43% | 100% | 38% | 57% |
| files/fixed | granite-reftok | full | vector | 23 | 70% | 0.57 | 88% | 62% | 57% | 100% | 50% | 57% |
| files/fixed | granite-reftok | full | hybrid | 23 | 70% | 0.61 | 88% | 62% | 57% | 100% | 50% | 57% |
| files/fixed | granite-reftok | int8 | vector | 23 | 70% | 0.57 | 88% | 62% | 57% | 100% | 50% | 57% |
| files/fixed | granite-reftok | int8 | hybrid | 23 | 70% | 0.61 | 88% | 62% | 57% | 100% | 50% | 57% |
| files/heading | — | — | fts | 23 | 30% | 0.30 | 38% | 38% | 14% | 88% | 0% | 0% |
| files/heading | granite | full | vector | 23 | 74% | 0.60 | 88% | 75% | 57% | 100% | 50% | 71% |
| files/heading | granite | full | hybrid | 23 | 74% | 0.67 | 88% | 75% | 57% | 100% | 50% | 71% |
| files/heading | granite | int8 | vector | 23 | 74% | 0.59 | 88% | 75% | 57% | 100% | 50% | 71% |
| files/heading | granite | int8 | hybrid | 23 | 74% | 0.67 | 88% | 75% | 57% | 100% | 50% | 71% |
| files/heading | granite-reftok | full | vector | 23 | 74% | 0.67 | 88% | 75% | 57% | 100% | 62% | 57% |
| files/heading | granite-reftok | full | hybrid | 23 | 74% | 0.70 | 88% | 75% | 57% | 100% | 62% | 57% |
| files/heading | granite-reftok | int8 | vector | 23 | 74% | 0.67 | 88% | 75% | 57% | 100% | 62% | 57% |
| files/heading | granite-reftok | int8 | hybrid | 23 | 74% | 0.70 | 88% | 75% | 57% | 100% | 62% | 57% |

## Same-words queries hybrid ranks lower than FTS

| Set | Model | Form | Query | FTS rank | hybrid rank |
|---|---|---|---|---|---|
| files/heading | granite | full | qf003 | 1 | 2 |
| files/heading | granite | int8 | qf003 | 1 | 2 |
| files/heading | granite-reftok | full | qf003 | 1 | 2 |
| files/heading | granite-reftok | int8 | qf003 | 1 | 2 |

## No-answer queries

FTS: no-answer queries with no result at all.

| Set | empty | of |
|---|---|---|
| messages | 15 | 15 |
| files/fixed | 7 | 7 |
| files/heading | 7 | 7 |

Vector search: the top cosine of no-answer queries, and the cosine of real hits (the target ranked first). *Above* is the share of real hits above the highest no-answer score, which a cutoff there would keep.

| Set | Model | Form | no-answer max | no-answer median | hits min | hits median | above |
|---|---|---|---|---|---|---|---|
| messages | granite | full | 0.769 | 0.674 | 0.640 | 0.755 | 48% |
| messages | granite | int8 | 0.769 | 0.674 | 0.640 | 0.755 | 48% |
| messages | granite-reftok | full | 0.765 | 0.672 | 0.647 | 0.754 | 44% |
| messages | granite-reftok | int8 | 0.765 | 0.673 | 0.648 | 0.755 | 44% |
| files/fixed | granite | full | 0.750 | 0.727 | 0.731 | 0.769 | 67% |
| files/fixed | granite | int8 | 0.750 | 0.727 | 0.731 | 0.769 | 67% |
| files/fixed | granite-reftok | full | 0.761 | 0.727 | 0.723 | 0.738 | 50% |
| files/fixed | granite-reftok | int8 | 0.761 | 0.726 | 0.723 | 0.737 | 50% |
| files/heading | granite | full | 0.735 | 0.713 | 0.712 | 0.777 | 82% |
| files/heading | granite | int8 | 0.734 | 0.713 | 0.711 | 0.777 | 82% |
| files/heading | granite-reftok | full | 0.749 | 0.711 | 0.702 | 0.790 | 71% |
| files/heading | granite-reftok | int8 | 0.749 | 0.712 | 0.702 | 0.790 | 71% |

## Embedding time

From each vector's share of its batch's time, cached vectors too (*fresh* were embedded in this run). Queries are embedded one at a time.

| Model | Set | items | fresh | per 1,000 messages | per 100 pages | query p50 | query p95 |
|---|---|---|---|---|---|---|---|
| granite | messages | 300 | 0 | 2m30.93s | — | 78.4ms | 768.4ms |
| granite | files/fixed | 392 | 0 | — | 2m30.37s | 98.6ms | 1.31s |
| granite | files/heading | 687 | 0 | — | 4m9.15s | 98.6ms | 1.31s |
| granite-reftok | messages | 300 | 0 | 22.28s | — | 11.2ms | 23.2ms |
| granite-reftok | files/fixed | 392 | 0 | — | 22.22s | 12.4ms | 26.9ms |
| granite-reftok | files/heading | 687 | 0 | — | 25.93s | 12.4ms | 26.9ms |

## Storage

Bytes per vector, per message (one vector each) and per page (the chunks a page has). FTS is 0.4 × the text size (SPIKE-011).

| Model | Form | bytes/vector | messages per message | files/fixed per page | files/heading per page |
|---|---|---|---|---|---|
| granite | full | 1536 | 1536 | 2486 | 4357 |
| granite | int8 | 388 | 388 | 628 | 1101 |
| granite-reftok | full | 1536 | 1536 | 2486 | 4357 |
| granite-reftok | int8 | 388 | 388 | 628 | 1101 |
| FTS | — | — | 51 | 967 | 1040 |
