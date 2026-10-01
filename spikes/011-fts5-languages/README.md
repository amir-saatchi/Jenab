# SPIKE-011 — FTS5 for German and Persian text

Throwaway code for [SPIKE-011](../../Docs/Tickets/SPIKE-011-fts5-languages.md).

```bash
go run . > results.md    # about 35 seconds
```

- `corpus.go`: 33 hand-labelled messages (German, English, Persian, mixed, plus some "noise" messages that should not match), 26 queries with the messages each should find, and 17 hostile query inputs
- `norm.go`: the Go normalization, the query builder `ftsQuery`, and `ftsQueryZWNJ` (a term with a ZWNJ also matches its split form)
- `main.go`: part 1 runs every tokenizer × normalization on the labelled set; part 2 runs the hostile inputs; part 3 builds 100,000 synthetic messages and measures build time, index size and query time
- `gen.go`: the synthetic message generator for part 3 (Zipf word frequencies)
- `clock.go`: high-resolution timer
- `results.md`: output of the last run

In part 2, the "FTS5 query" column shows the `ftsQuery` output; the `ftsQueryZWNJ` column runs that builder's own output.
