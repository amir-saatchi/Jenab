# SPIKE-020: keyless web search

Throwaway code for SPIKE-020. The question: can Burrow's `web_search` tool / `web.search` step work out of the box with no API key, using APIs made for programmatic access? Results are in [results.md](results.md).

It uses only the Go standard library (Go 1.26, no cgo).

```bash
go run . -collect ddg,wikipedia,hn,marginalia,rss   # about 1 min
go run . -collect gdelt                              # 3-month and 7-day variants, 10 s apart, 1 retry on 429 (~13 min today)
go run . -collect gdelt-slow                         # 7-day variant, 30 s apart, no retry
go run . -collect marginalia -only q07,q08,q09 -tag -late   # -only picks queries, -tag keeps the earlier raw file
go run . -list-public                                # 1 request to searx.space, prints candidates
go run . -collect searxng-public -public https://a.example,https://b.example
go run . -collect searxng-local -local http://127.0.0.1:8888 -rounds 3   # needs the container below
go run . -ddg-control                                # 1 DDG request for "Bitcoin", to show what the API does answer
go run . -dump                                       # writes top5.md for hand judging
go run . -report > results.md                        # tables from raw/*.json + judgments.txt + notes.md
```

## Files

- `queries.go`: the 10 tracker-style queries (8 English, 1 German, 1 Persian). `Match` is only used for RSS, which has no server-side search.
- `http.go`: a polite client. It sends the User-Agent `Burrow-spike/0.1 (research; contact via github)` and waits at least 1.1 s between requests to the same host. Some hosts wait longer: GDELT 10 s, public SearXNG 4 s, local SearXNG 3 s. On a 429 it retries once after `Retry-After` (at most 60 s, 15 s by default). Public SearXNG never gets a retry.
- `sources.go`: one adapter per source, all mapped to Burrow's result shape (title, url, snippet, date).
  - SearXNG: `format=json`
  - DuckDuckGo Instant Answer
  - GDELT DOC 2.0 (`mode=artlist`): run with the default 3-month window and with `timespan=7d`
  - MediaWiki `list=search`: the wiki language comes from the query language
  - HN Algolia `/search?tags=story`
  - Marginalia `api2` with the documented `public` key
  - RSS/Atom
- `main.go`: collection. It writes `raw/<source>.json` (one record per request, with latency, status, results, and SearXNG `unresponsive_engines`). The two public-instance runs were renamed to `raw/searxng-public-batch1.json` and `raw/searxng-public-batch2.json`. `raw/ddg-control.txt` and `raw/searx-space-instances.json` are kept for reference and are not read by the report.
- `report.go`: builds the tables in `results.md`.
- `judgments.txt`: hand relevance judgments, 0/1 for each of the top 5 results, per source and query (written after reading `top5.md`).
- `notes.md`: the hand-written parts of `results.md`: terms, keyed providers, and the recommendation. The generated tables are inserted at `<!-- TABLES -->`.
- `searxng/settings.yml`: minimal settings for a local SearXNG. It enables `json` and turns off the limiter.

## Local SearXNG (not run in this spike)

Docker Desktop failed to start on the test machine, so local SearXNG was not measured. No container was created and no image was pulled. To run it:

```bash
docker pull searxng/searxng
docker run -d --name burrow-searxng -p 127.0.0.1:8888:8080 \
  -e SEARXNG_SECRET="$(openssl rand -hex 32)" \
  -v "$PWD/searxng:/etc/searxng" searxng/searxng
go run . -collect searxng-local -rounds 3        # 30 general + 10 news queries, 3 s apart
docker rm -f burrow-searxng && docker image rm searxng/searxng
```

`report.go` already has the per-engine table: which upstream engines answered, and how often SearXNG listed each one as unresponsive, with the reason (CAPTCHA, too many requests, timeout).

## Politeness

- Public SearXNG: at most 10 requests per instance, at least 4 s apart. The spike stops after two refusals in a row. In practice every instance got 2 requests.
- Google News RSS was not called. `news.google.com/robots.txt` disallows `/rss` (see results.md).
- DuckDuckGo `html.`/`lite.` pages were not called.
- No accounts and no keys were used. The keyed providers come from their public docs only.
