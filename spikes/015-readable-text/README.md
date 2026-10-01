# SPIKE-015 — readable text from web pages

Throwaway code for [SPIKE-015](../../Docs/Tickets/SPIKE-015-readable-text.md).

```bash
go run . > results.md    # a few minutes, no network (use -nospeed to skip sections 7 and 8)
go run ./fetch           # refetch missing fixtures (-force refetches all, -only id1,id2)
```

- `corpus/pages.go`: the 35 pages (URL, group, language, derived legacy-encoding copies)
- `corpus/labels.go`: hand-written labels per page: expected title, 2–4 main-text phrases, 2–4 boilerplate phrases, and table rows
- `fetch/main.go`: the fetcher. It uses a plain test User-Agent, honors robots.txt, waits at least 2 s per host, and sends one request per page. It also builds the derived windows-1256/windows-1252 fixtures from their UTF-8 source
- `fixtures/`: `<id>.html` as served, plus `<id>.json` with URL, final URL, fetch date, content-type, header and meta charset
- `fixtures.go`: loads the fixtures, decodes them with `x/net/html/charset`, and normalizes text for matching (NFC, lower case, whitespace, Arabic ي/ك to Persian ی/ک)
- `extract.go`: one adapter per library (trafilatura, trafilatura+fallback, readeck v2, go-shiori, domdistiller, and whole page to Markdown as a baseline), plus the html-to-markdown setup and link collection
- `score.go`: phrase hits, leaks, title, links, table rows, RTL and mojibake checks
- `selector.go`: selector mode with goquery and cascadia (section 9)
- `main.go`: writes every section of `results.md`. Memory is measured in a child process (`-child`)
- `notes.go`: manual notes, written by hand after reading the extracted text (`-dump dir` writes it out)
- `results.md`: output of the last run

Fetch notes: fixtures were fetched on 2026-09-28. Stack Overflow was skipped because its robots.txt disallows `*`. The German news and government sites inject their cookie banner with JavaScript, so it is not in their fixtures. Every phrase is checked against the page's visible text on each run, so a wrong label shows up at the top of `results.md`.
