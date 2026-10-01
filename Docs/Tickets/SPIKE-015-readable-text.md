# SPIKE-015 — Readable text from web pages
**Type:** Spike
**Status:** Done
**Gate:** 2

## Question
Which library turns a web page into readable text for `fetch_page` and `html.extract` (SPEC 3.7, 6.5)?

1. The main text is kept, while menus, ads and cookie banners are removed
2. Title and links are returned
3. English, German and Persian pages (right-to-left, old encodings such as windows-1256)
4. Tables, such as price tables, stay readable
5. Speed and memory on large pages

Candidates: `markusmobius/go-trafilatura/v2`, `readeck/go-readability/v2`, `go-shiori/go-readability`, plus an HTML-to-Markdown converter as an extra step.

## Done when
Each candidate is scored on a saved set of about 30 real pages, and one is chosen.

## Result
Run on 2026-09-28 with no cgo. Code and full output: `spikes/015-readable-text/` (`results.md`). The saved pages in `fixtures/` are copies of third-party sites and are kept out of git.

**Test set: 35 pages**, 32 fetched once and 3 derived. They include:
- news and Wikipedia in English, German and Persian
- docs, blogs, a forum, a GitHub README
- 4 table pages, 2 JavaScript-heavy pages and a 14.9 MB page
- Shift_JIS, windows-1256 and windows-1252 pages

There are 109 main-text phrases that should appear and 118 boilerplate phrases that should not.

| Library | Version (date), license | Main text | Boilerplate leaks | Title right | p50 | Peak heap, 14.9 MB page |
|---|---|---|---|---|---|---|
| `readeck/go-readability/v2` | v2.1.2 (2026-06-18), MIT | 98 / 109 | 5 | 34 / 35 | 17 ms | 261 MB |
| `go-trafilatura/v2` + fallback | v2.2.2 (2026-09-21), Apache-2.0 | 100 / 109 | 5 | 33 / 35 | 31 ms | 681 MB |
| `go-trafilatura/v2` | same | 97 / 109 | 7 | 33 / 35 | 24 ms | 550 MB |
| `go-shiori/go-readability` (deprecated) | 2025-12-05, MIT | 93 / 109 | 6 | 35 / 35 | 19 ms | 447 MB |
| `go-domdistiller` | v1.0.0 (2024-09-26), MIT | 86 / 109 | 3 | 35 / 35 | 17 ms | 507 MB |
| Whole page → Markdown (baseline) | — | 109 / 109 | 112 | — | 11 ms | 261 MB |

**Findings**
- **readeck vs trafilatura:** readeck is almost as good on text as trafilatura with fallback (98 vs 100), and better on memory.
  - Its peak heap is half as large: 261 vs 681 MB on the 14.9 MB page, and 94 vs 230 MB on a 5 MB page.
  - trafilatura's first call takes about 380 ms and 186 MB, even for a 44 KB page.
  - trafilatura also adds a WebAssembly regex engine.
- **Tables:** no library is best everywhere.
  - Wikipedia: trafilatura's Markdown keeps the tables; readeck drops them.
  - gov.uk holidays: readeck keeps full rows; trafilatura drops the date column.
  - ECB rates: only domdistiller keeps the table.
- **html-to-markdown v2.5.2:** skips tables that have line breaks in cells unless set to keep newlines and mirror spans. It takes 2.2 ms per page.
- **Encoding:**
  - Raw windows-1256 bytes came out as garbage in every library (0 of 4), even with a `<meta>` charset.
  - Decoding in Go first gave 4 of 4, and made no UTF-8 page worse.
- **Persian:** right-to-left text and ZWNJ are kept in the body by all libraries. trafilatura's title loses the ZWNJ.
- **German:**
  - destatis is a hub page of teasers, and every library scores 0 of 3 on it.
  - The consent banners on spiegel, heise, destatis and bundesbank are added by JavaScript, so they are not in the fetched HTML.
- **JavaScript pages:** the Excalidraw page has 32 visible characters. On CoinMarketCap the live price is missing in every library.
- **Selector mode:**
  - goquery matched all 13 cases.
  - `cascadia.Compile` reports errors for all 6 bad selectors, while `goquery.Find` silently returns nothing.
- **Timings** varied about 2× between runs; compare them only relative to each other.
- **Not tested:**
  - JavaScript rendering
  - paywalled pages
  - consent banners inside the HTML
  - real (not derived) windows-1256 pages
  - PDFs
  - fuzzing with hostile HTML

## Decision
**Decided (2026-09-28):**
- **Library:** `codeberg.org/readeck/go-readability/v2` for readable mode, followed by `html-to-markdown/v2` with the table options, so the text is Markdown.
- **Rules Go adds** (SPEC 3.7):
  - decode the charset before extraction
  - cap the body at 5 MB
  - build the title in Go, keeping the ZWNJ
  - return `needs_javascript` for pages with under about 200 characters of text
  - in selector mode, compile with `cascadia.Compile` and return its error
- **Alternative:** trafilatura with fallback, if table-heavy pages turn out to matter more than memory.
