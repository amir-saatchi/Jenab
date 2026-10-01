# SPIKE-015 results

go1.26.0, windows/amd64, 12 CPUs, 2026-09-28. 35 fixtures (32 fetched, 3 derived). Input to every library is the page after Go decoded it to UTF-8, unless a section says otherwise.

Libraries: `github.com/markusmobius/go-trafilatura/v2` v2.2.2, `codeberg.org/readeck/go-readability/v2` v2.1.2, `github.com/go-shiori/go-readability` v0.0.0-20251205110129-5db1dc9836f0, `github.com/markusmobius/go-domdistiller` v1.0.0, `github.com/JohannesKaufmann/html-to-markdown/v2` v2.5.2, `github.com/PuerkitoBio/goquery` v1.13.0, `github.com/andybalholm/cascadia` v1.3.5, `golang.org/x/net` v0.59.0.

Labels: 109 main-text phrases, 118 boilerplate phrases. Every phrase was checked against the visible text of the whole page (script and style removed): 0 not found.

## 1. Summary

Main text = main-text phrases found. Boilerplate = boilerplate phrases that leaked into the text (lower is better). Title = extracted title contains the expected title. Links = pages where at least one link came back / pages scored. Empty = pages with fewer than 200 characters of text or an error. RTL = Persian pages where all phrases (including the ones with a ZWNJ) are found and no U+FFFD. Size = total text characters over all pages.

| Library | Main text | Boilerplate | Title | Links | Empty/error | RTL (fa) | Size (text) | Size (Markdown) |
|---|---|---|---|---|---|---|---|---|
| trafilatura | 97 of 109 | 7 of 118 | 33 of 35 | 30 of 35 | 1 | 6 of 6 | 4.97 M | 8.05 M |
| trafilatura+fallback | 100 of 109 | 5 of 118 | 33 of 35 | 31 of 35 | 1 | 6 of 6 | 5.11 M | 8.20 M |
| readeck v2 | 98 of 109 | 5 of 118 | 34 of 35 | 32 of 35 | 1 | 6 of 6 | 5.92 M | 10.21 M |
| go-shiori | 93 of 109 | 6 of 118 | 35 of 35 | 32 of 35 | 1 | 6 of 6 | 6.18 M | 10.21 M |
| domdistiller | 86 of 109 | 3 of 118 | 35 of 35 | 26 of 35 | 3 | 5 of 6 | 4.15 M | 6.03 M |
| no extraction (whole page → Markdown) | 109 of 109 | 112 of 118 | 0 of 35 | 34 of 35 | 1 | 6 of 6 | 13.03 M | 13.03 M |

## 2. Per page

Cell: main-text phrases found / total, then boilerplate leaks as `-n`, `T` if the title is right, `E` for an error, `∅` for under 200 characters. Columns: traf = trafilatura, traf+fb = trafilatura with fallback, readeck = readeck v2, shiori = go-shiori, distiller = domdistiller, whole = whole page to Markdown.

| Page | Group | Lang | KB | traf | traf+fb | readeck | shiori | distiller | whole |
|---|---|---|---|---|---|---|---|---|---|
| news-bbc-en | news | en | 412 | 3/3 T | 3/3 T | 3/3 T | 3/3 T | 3/3 T | 3/3 -4 |
| news-guardian-en | news | en | 342 | 3/3 T | 3/3 T | 3/3 T | 3/3 T | 3/3 T | 3/3 -4 |
| news-tagesschau-de | news | de | 577 | 3/3 T | 3/3 T | 3/3 T | 3/3 T | 3/3 T | 3/3 -4 |
| news-spiegel-de | news | de | 500 | 3/3 T | 3/3 T | 3/3 | 1/3 T | 3/3 T | 3/3 -4 |
| news-heise-de | news | de | 235 | 3/3 T | 3/3 T | 3/3 T | 3/3 T | 2/3 T | 3/3 -4 |
| news-bbc-fa | news | fa | 491 | 4/4 T | 4/4 T | 4/4 T | 4/4 T | 4/4 T | 4/4 -4 |
| news-dw-fa | news | fa | 134 | 3/3 -2 T | 3/3 -2 T | 3/3 -2 T | 3/3 -2 T | 3/3 T | 3/3 -4 |
| wiki-en | wiki | en | 874 | 4/4 T | 4/4 T | 4/4 T | 4/4 T | 4/4 T | 4/4 -3 |
| wiki-de | wiki | de | 1066 | 3/3 -1 T | 3/3 -1 T | 3/3 T | 3/3 T | 2/3 T | 3/3 -4 |
| wiki-fa | wiki | fa | 650 | 3/3 | 3/3 | 3/3 T | 3/3 T | 3/3 T | 3/3 -3 |
| docs-go-effective | docs | en | 139 | 4/4 T | 4/4 T | 4/4 T | 4/4 T | 4/4 -1 T | 4/4 -4 |
| docs-mdn-en | docs | en | 241 | 3/3 T | 3/3 T | 2/3 T | 2/3 T | 1/3 T | 3/3 -4 |
| docs-mdn-de | docs | de | 262 | 3/3 T | 3/3 T | 2/3 T | 2/3 T | 2/3 T | 3/3 -4 |
| blog-go-slices | blog | en | 43 | 3/3 T | 3/3 T | 3/3 T | 3/3 T | 3/3 T | 3/3 -3 |
| blog-rsc-vgo | blog | en | 42 | 3/3 T | 3/3 T | 3/3 T | 3/3 T | 3/3 T | 3/3 -1 |
| blog-cloudflare | blog | en | 942 | 4/4 T | 4/4 T | 4/4 T | 4/4 T | 4/4 T | 4/4 -4 |
| forum-hn | forum | en | 3354 | 4/4 -2 T | 4/4 -2 T | 3/4 T | 3/4 T | 0/4 T ∅ | 4/4 -2 |
| forum-golangbridge | forum | en | 19 | 2/4 T | 2/4 T | 4/4 T | 4/4 T | 4/4 T | 4/4 -1 |
| github-readme | forum | en | 405 | 3/3 T | 3/3 T | 3/3 T | 3/3 T | 3/3 T | 3/3 -4 |
| table-wiki-population | table | en | 1351 | 0/2 T | 0/2 T | 1/2 T | 1/2 T | 0/2 T | 2/2 -2 |
| table-wiki-fa-population | table | fa | 1535 | 2/2 T | 2/2 T | 2/2 T | 2/2 T | 0/2 T | 2/2 -2 |
| table-ecb-rates | table | en | 116 | 3/3 T | 3/3 T | 3/3 T | 0/3 -1 T | 3/3 T | 3/3 -4 |
| table-govuk-holidays | table | en | 171 | 3/3 T | 3/3 T | 3/3 T | 3/3 T | 2/3 T | 3/3 -4 |
| banner-govuk | banner | en | 64 | 3/3 T | 3/3 T | 1/3 T | 1/3 T | 1/3 T | 3/3 -4 |
| gov-destatis-de | gov | de | 168 | 0/3 T | 0/3 T | 0/3 T | 0/3 T | 0/3 T | 3/3 -3 |
| gov-bundesbank-de | gov | de | 335 | 2/3 -2 T | 2/3 T | 2/3 -2 T | 2/3 -2 T | 0/3 T ∅ | 3/3 -4 |
| article-verbraucherzentrale-de | blog | de | 677 | 3/3 T | 3/3 T | 3/3 -1 T | 3/3 -1 T | 3/3 T | 3/3 -4 |
| js-coinmarketcap | js | en | 907 | 2/3 T | 2/3 T | 2/3 T | 2/3 T | 2/3 T | 3/3 -2 |
| js-excalidraw | js | en | 6 | 0/0 T ∅ | 0/0 T ∅ | 0/0 T ∅ | 0/0 T ∅ | 0/0 T ∅ | 0/0 ∅ |
| large-go-spec | large | en | 333 | 3/3 T | 3/3 T | 3/3 T | 3/3 T | 3/3 -1 T | 3/3 -3 |
| large-whatwg-html | large | en | 15232 | 4/4 | 4/4 | 4/4 T | 4/4 T | 4/4 T | 4/4 -2 |
| legacy-aozora-sjis | legacy | ja | 603 | 0/3 T | 3/3 T | 3/3 T | 3/3 T | 3/3 -1 T | 3/3 -1 |
| legacy-bbc-fa-1256-meta | legacy | fa | 469 | 4/4 T | 4/4 T | 4/4 T | 4/4 T | 4/4 T | 4/4 -4 |
| legacy-bbc-fa-1256-header | legacy | fa | 469 | 4/4 T | 4/4 T | 4/4 T | 4/4 T | 4/4 T | 4/4 -4 |
| legacy-tagesschau-1252-meta | legacy | de | 576 | 3/3 T | 3/3 T | 3/3 T | 3/3 T | 3/3 T | 3/3 -4 |

## 3. Tables

For each table page, a few rows are labelled by their cells. Cell: rows whose cells all stay on one line in the plain text / in the Markdown output (html-to-markdown with the table plugin, run on the library's content node).

| Page | Rows | traf | traf+fb | readeck | shiori | distiller | whole |
|---|---|---|---|---|---|---|---|
| table-wiki-population | 3 | 0 / 3 | 0 / 3 | 0 / 0 | 0 / 0 | 0 / 0 | 3 / 3 |
| table-wiki-fa-population | 3 | 0 / 3 | 0 / 3 | 0 / 0 | 0 / 0 | 0 / 0 | 3 / 3 |
| table-ecb-rates | 3 | 0 / 0 | 0 / 0 | 0 / 0 | 0 / 0 | 3 / 3 | 3 / 3 |
| table-govuk-holidays | 3 | 0 / 0 | 0 / 0 | 3 / 3 | 0 / 3 | 3 / 3 | 3 / 3 |

## 4. By language

Main-text phrases found / boilerplate leaks, per language (legacy-encoding copies included).

| Library | en (19 pages) | de (9 pages) | fa (6 pages) | ja (1 pages) |
|---|---|---|---|---|
| trafilatura | 54 of 59 / 2 | 23 of 27 / 3 | 20 of 20 / 2 | 0 of 3 / 0 |
| trafilatura+fallback | 54 of 59 / 2 | 23 of 27 / 1 | 20 of 20 / 2 | 3 of 3 / 0 |
| readeck v2 | 53 of 59 / 0 | 22 of 27 / 3 | 20 of 20 / 2 | 3 of 3 / 0 |
| go-shiori | 50 of 59 / 1 | 20 of 27 / 3 | 20 of 20 / 2 | 3 of 3 / 0 |
| domdistiller | 47 of 59 / 2 | 18 of 27 / 0 | 18 of 20 / 0 | 3 of 3 / 1 |
| no extraction (whole page → Markdown) | 59 of 59 / 55 | 27 of 27 / 35 | 20 of 20 / 21 | 3 of 3 / 1 |

## 5. Encodings

Raw = the bytes as served, handed to the library (it cannot see the HTTP header). Go = decoded first by Go with `x/net/html/charset` (HTTP header, BOM, `<meta>`), then handed over as UTF-8. Cell: main-text phrases found / total, `M` if the text has U+FFFD or mis-decoded sequences.

| Page | Header charset | Meta charset | Go chose | traf raw | traf Go | traf+fb raw | traf+fb Go | readeck raw | readeck Go | shiori raw | shiori Go | distiller raw | distiller Go |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| legacy-aozora-sjis | — | shift_jis | shift_jis | 0/3 | 0/3 | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 |
| legacy-bbc-fa-1256-meta | windows-1256 | windows-1256 | windows-1256 | 0/4 M | 4/4 | 0/4 M | 4/4 | 0/4 M | 4/4 | 0/4 M | 4/4 | 0/4 M | 4/4 |
| legacy-bbc-fa-1256-header | windows-1256 | — | windows-1256 | 0/4 M | 4/4 | 0/4 M | 4/4 | 0/4 M | 4/4 | 0/4 M | 4/4 | 0/4 M | 4/4 |
| legacy-tagesschau-1252-meta | windows-1252 | windows-1252 | windows-1252 | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 |

Raw input on the 31 non-legacy pages (all UTF-8): pages where the raw result finds fewer phrases than the Go-decoded result, or has mis-decoded text.

| Library | Pages worse with raw input |
|---|---|
| trafilatura | 0  |
| trafilatura+fallback | 0  |
| readeck v2 | 0  |
| go-shiori | 0  |
| domdistiller | 0  |

## 6. JavaScript-heavy pages

Script share = bytes inside `<script>` / page bytes. Visible = characters of visible text on the whole page. Other cells: characters of extracted text.

| Page | KB | Script share | Visible | traf | traf+fb | readeck | shiori | distiller |
|---|---|---|---|---|---|---|---|---|
| js-coinmarketcap | 907 | 61% | 25047 | 18385 | 18385 | 18760 | 18605 | 13762 |
| js-excalidraw | 6 | 37% | 32 | 57 | 57 | 0 | 0 | 0 |

## 7. Speed and allocations

Per page: median of up to 5 runs after one warm-up run (pages over 4 MB: no warm-up). Then p50 and max over the 35 pages. Allocated = bytes allocated per extraction (not peak memory). The Markdown row converts each library's content node; its numbers are for the readeck v2 output.

| Library | Time p50 | Time max (page) | Allocs p50 | Allocated p50 | Allocated max |
|---|---|---|---|---|---|
| trafilatura | 23.8 ms | 3.97 s (large-whatwg-html) | 84 k | 10.9 MB | 1451.0 MB |
| trafilatura+fallback | 30.5 ms | 5.22 s (large-whatwg-html) | 113 k | 13.4 MB | 2099.9 MB |
| readeck v2 | 17.1 ms | 1.55 s (large-whatwg-html) | 38 k | 4.4 MB | 314.5 MB |
| go-shiori | 18.7 ms | 2.46 s (large-whatwg-html) | 76 k | 17.1 MB | 1304.6 MB |
| domdistiller | 17.2 ms | 4.91 s (large-whatwg-html) | 67 k | 12.1 MB | 1177.4 MB |
| no extraction (whole page → Markdown) | 10.5 ms | 1.51 s (large-whatwg-html) | 69 k | 4.1 MB | 517.2 MB |
| html-to-markdown on readeck v2 output (step only) | 2.2 ms | 1.08 s (large-whatwg-html) | 4 k | 0.8 MB | 385.2 MB |

Largest pages, time per extraction:

| Page | MB | trafilatura | trafilatura+fallback | readeck v2 | go-shiori | domdistiller | no extraction (whole page → Markdown) |
|---|---|---|---|---|---|---|---|
| wiki-de | 1.0 | 128.2 ms | 200.4 ms | 71.4 ms | 110.5 ms | 66.0 ms | 46.8 ms |
| forum-hn | 3.3 | 666.6 ms | 1.97 s | 1.17 s | 1.99 s | 494.9 ms | 337.4 ms |
| table-wiki-population | 1.3 | 106.4 ms | 139.1 ms | 51.7 ms | 72.5 ms | 54.2 ms | 57.7 ms |
| table-wiki-fa-population | 1.5 | 132.7 ms | 183.1 ms | 65.0 ms | 87.9 ms | 67.8 ms | 53.3 ms |
| large-whatwg-html | 14.9 | 3.97 s | 5.22 s | 1.55 s | 2.46 s | 4.91 s | 1.51 s |

## 8. Peak memory and first call

Each cell is a fresh child process that loads one page and runs one extraction. Peak = highest live heap above the starting heap, sampled every 1 ms. Total = memory the Go runtime holds from the OS after the run. First = the first call in the process (includes lazy setup such as regex compilation and model loading); second = a repeat call. `large-whatwg-html` is 14.9 MB; the 5 MB row cuts the same page at the SPEC 6.6 response limit.

| Page | Library | First | Second | Peak heap | Total from OS | Text chars |
|---|---|---|---|---|---|---|
| blog-go-slices (44 KB) | trafilatura | 383.7 ms | 11.3 ms | 186 MB | 255 MB | 9930 |
| blog-go-slices (44 KB) | trafilatura+fallback | 360.4 ms | 8.4 ms | 186 MB | 255 MB | 9930 |
| blog-go-slices (44 KB) | readeck v2 | 4.0 ms | 3.4 ms | 0 MB | 59 MB | 10053 |
| blog-go-slices (44 KB) | go-shiori | 6.7 ms | 6.1 ms | 3 MB | 58 MB | 9986 |
| blog-go-slices (44 KB) | domdistiller | 9.1 ms | 16.3 ms | 1 MB | 63 MB | 9873 |
| blog-go-slices (44 KB) | no extraction (whole page → Markdown) | 4.7 ms | 5.0 ms | 1 MB | 59 MB | 17495 |
| forum-hn (3.3 MB) | trafilatura | 1.92 s | 973.8 ms | 204 MB | 395 MB | 574270 |
| forum-hn (3.3 MB) | trafilatura+fallback | 2.70 s | 2.63 s | 202 MB | 399 MB | 574270 |
| forum-hn (3.3 MB) | readeck v2 | 2.91 s | 3.15 s | 104 MB | 162 MB | 502137 |
| forum-hn (3.3 MB) | go-shiori | 2.85 s | 2.76 s | 123 MB | 230 MB | 496491 |
| forum-hn (3.3 MB) | domdistiller | 779.2 ms | 560.9 ms | 111 MB | 174 MB | 0 |
| forum-hn (3.3 MB) | no extraction (whole page → Markdown) | 303.2 ms | 282.1 ms | 76 MB | 133 MB | 1894759 |
| large-whatwg-html cut at 5 MB | trafilatura | 1.84 s | 1.16 s | 230 MB | 469 MB | 1167741 |
| large-whatwg-html cut at 5 MB | trafilatura+fallback | 2.10 s | 1.67 s | 255 MB | 493 MB | 1167741 |
| large-whatwg-html cut at 5 MB | readeck v2 | 528.1 ms | 529.4 ms | 94 MB | 279 MB | 1496923 |
| large-whatwg-html cut at 5 MB | go-shiori | 812.9 ms | 821.8 ms | 179 MB | 275 MB | 1573803 |
| large-whatwg-html cut at 5 MB | domdistiller | 860.1 ms | 791.2 ms | 210 MB | 297 MB | 1087577 |
| large-whatwg-html cut at 5 MB | no extraction (whole page → Markdown) | 412.9 ms | 373.4 ms | 113 MB | 259 MB | 2503947 |
| large-whatwg-html (14.9 MB) | trafilatura | 3.74 s | 3.72 s | 550 MB | 721 MB | 3427509 |
| large-whatwg-html (14.9 MB) | trafilatura+fallback | 5.97 s | 6.14 s | 681 MB | 792 MB | 3427509 |
| large-whatwg-html (14.9 MB) | readeck v2 | 2.14 s | 2.20 s | 261 MB | 346 MB | 4283928 |
| large-whatwg-html (14.9 MB) | go-shiori | 2.86 s | 2.45 s | 447 MB | 603 MB | 4525068 |
| large-whatwg-html (14.9 MB) | domdistiller | 3.11 s | 3.03 s | 507 MB | 639 MB | 3178086 |
| large-whatwg-html (14.9 MB) | no extraction (whole page → Markdown) | 1.24 s | 1.17 s | 261 MB | 350 MB | 7299047 |

## 9. Selector mode (goquery + cascadia)

Each selector is compiled with `cascadia.Compile` (so errors are visible) and run with goquery on the Go-decoded page. Want = phrase the matched text must contain (empty = only needs a match). Time includes parsing the page.

| Page | Selector | Matches | Want found | First match (60 chars) | Time |
|---|---|---|---|---|---|
| wiki-en | `#firstHeading` | 1 | yes | bitcoin | 14.8 ms |
| wiki-en | `table.infobox th` | 34 | yes | denominations | 13.4 ms |
| wiki-fa | `#firstHeading` | 1 | yes | بیت‌کوین | 10.1 ms |
| table-ecb-rates | `table.forextable tbody tr td.currency` | 29 | yes | usd | 2.2 ms |
| table-ecb-rates | `table.forextable tr:has(td.currency:contains('JPY')) td.spot` | 1 | — | 179.70 | 2.3 ms |
| table-govuk-holidays | `#england-and-wales table caption` | 11 | yes | upcoming bank holidays in england and wales 2026 | 3.1 ms |
| table-wiki-population | `table.wikitable tbody tr:nth-child(3) td` | 6 | — | india | 19.1 ms |
| news-bbc-fa | `h1` | 1 | — | پای برهنه، هاله نور و «پنهان شدن در دستشویی»؛ پنج دهه حاشیه‌… | 2.8 ms |
| news-tagesschau-de | `h1` | 1 | — | vor der wahl in israel warum arabische parteien ausgeschloss… | 4.5 ms |
| docs-go-effective | `h2#names` | 1 | yes | names | 1.7 ms |
| github-readme | `article.markdown-body h2` | 8 | — | table of contents | 5.5 ms |
| forum-hn | `.commtext` | 2530 | — | all: our poor single-core server process has smoke coming ou… | 85.5 ms |
| large-whatwg-html | `h2` | 23 | — | table of contents | 353.5 ms |

Invalid selectors: `cascadia.Compile` error vs what `goquery.Find` does with the same string.

| Selector | cascadia.Compile | goquery Find matches |
|---|---|---|
| `div[` | error: expected identifier, found EOF instead | 0 |
| `a:has(` | error: expected selector, found EOF instead | 0 |
| `p >> span` | error: expected identifier, found > instead | 0 |
| `::before` | error: pseudo-element before found, but pseudo-elements support is disabled | 0 |
| `#` | error: expected name, found EOF instead | 0 |
| `td:nth-child(x)` | error: unexpected character while attempting to parse expression of form an+b | 0 |

## 10. Missed and leaked phrases (trafilatura, readeck v2)

**trafilatura**

- news-dw-fa: leaked "اینترنت بدون سانسور با سایفون", "دویچه وله فارسی را در اینستاگرام دنبال کنید"
- wiki-de: leaked "quelltext bearbeiten"
- forum-hn: leaked "apply to yc", "submit login"
- forum-golangbridge: missed "just wondering how you intend for people to use it", "this project grew out of sriracha, where it is used to match large ip lists"
- table-wiki-population: missed "this article reflects continually changing information from hundreds of sources", "population distribution by country in june-july 2025"
- gov-destatis-de: missed "der verbraucherpreisindex misst monatlich die durchschnittliche preisentwicklung aller waren und dienstleistungen", "der rund 700 güterarten umfasst", "der verbraucherpreisindex dient insbesondere zur messung der geldwertstabilität"
- gov-bundesbank-de: missed "ein zentrales suchfeld ermöglicht eine schnelle und direkte suche nach zeitreihen"; leaked "die bundesbank unterstützt den wandel hin zu einer kohlenstoffarmen wirtschaft", "suchen sie im aktuell gültigen verzeichnis der bankleitzahlen"
- js-coinmarketcap: missed "the live bitcoin price today is $83,205.70 usd"
- legacy-aozora-sjis: missed "その時私はまだ若々しい書生であった", "電報には母が病気だからと断ってあったけれども友達はそれを信じなかった", "私は妻には何にも知らせたくないのです"

**readeck v2**

- news-dw-fa: leaked "اینترنت بدون سانسور با سایفون", "دویچه وله فارسی را در اینستاگرام دنبال کنید"
- docs-mdn-en: missed "represents tabular data"
- docs-mdn-de: missed "tabellarische daten"
- forum-hn: missed "the board struggle reflected a cultural clash at the organization"
- table-wiki-population: missed "population distribution by country in june-july 2025"
- banner-govuk: missed "use this service to apply for, renew, replace or update your passport and pay for it online", "do not book travel until you have a valid passport"
- gov-destatis-de: missed "der verbraucherpreisindex misst monatlich die durchschnittliche preisentwicklung aller waren und dienstleistungen", "der rund 700 güterarten umfasst", "der verbraucherpreisindex dient insbesondere zur messung der geldwertstabilität"
- gov-bundesbank-de: missed "ein zentrales suchfeld ermöglicht eine schnelle und direkte suche nach zeitreihen"; leaked "die bundesbank unterstützt den wandel hin zu einer kohlenstoffarmen wirtschaft", "suchen sie im aktuell gültigen verzeichnis der bankleitzahlen"
- article-verbraucherzentrale-de: leaked "umfrage: ärger mit lemonswan?"
- js-coinmarketcap: missed "the live bitcoin price today is $83,205.70 usd"


## Manual notes

Written by hand after reading the dumped outputs (`go run . -nospeed -dump <dir>`). Not produced by the code.

- **Table readability.** Wikipedia population tables (en, fa): trafilatura keeps the table, but its plain text puts every cell on its own line; the Markdown step on its content node gives clean rows. readeck, go-shiori and domdistiller drop the main table. ECB rates table: trafilatura and readeck drop the rates table (only the text around it is kept); domdistiller keeps it. gov.uk bank holidays: trafilatura drops the date column (the row-header cells), so rows read "Monday / Summer bank holiday" without the date; readeck keeps full rows.
- **html-to-markdown table plugin.** With default options it skips every table that has a line break inside a cell, which is most Wikipedia tables. The spike uses NewlineBehaviorPreserve, minimal padding, SpanBehaviorMirror, SkipEmptyRows and HeaderPromotion.
- **Cookie banners.** On spiegel.de, heise.de, destatis.de and bundesbank.de the consent banner is injected by JavaScript and is not in the served HTML, so these pages do not test banner removal. Banners that are in the HTML: gov.uk (both pages), ECB, go.dev. trafilatura and readeck removed all of them. domdistiller leaked the go.dev banner on docs-go-effective and large-go-spec. go-shiori on the ECB page returned the cookie-policy text instead of the rates page (0/3).
- **Hub pages.** destatis.de "Verbraucherpreisindex" is a topic hub. The labelled paragraphs sit in teaser blocks (`s-contentteaser`) and every extractor drops them (0/3). Hub and index pages are a weak spot for all libraries.
- **JavaScript-only pages.** Excalidraw serves a 6 KB shell. trafilatura returns only "You need to enable JavaScript to run this app." plus the app name; the others return nothing. CoinMarketCap is 61% script, but the server-rendered HTML still has the article text; only the live price (filled in by script) is missing for every library.
- **Ruby text (Japanese).** trafilatura without fallback cuts each paragraph of the Aozora novel at its first `<ruby>` element, so most sentences are lost (0/3). With EnableFallback it picks the readeck/distiller result (3/3).
- **Persian.** Body text keeps ZWNJ in every library. trafilatura's title drops the ZWNJ ("بیتکوین" for "بیت‌کوین") and keeps the site suffix, so the wiki-fa title check fails. readeck returns the clean heading.
- **Titles.** trafilatura returns "HTML" for the WHATWG standard (the document's h1), not "HTML Standard".
- **Encodings.** Raw windows-1256 bytes break all five libraries, with or without a `<meta charset>`: they all guess the charset statistically and pick a wrong one. Decoding in Go first fixes it. Raw windows-1252 and Shift_JIS happened to be guessed right.
