package main

// Notes written by hand after reading the extracted text (go run . -dump dir).
const manualNotes = `
Written by hand after reading the dumped outputs (` + "`go run . -nospeed -dump <dir>`" + `). Not produced by the code.

- **Table readability.** Wikipedia population tables (en, fa): trafilatura keeps the table, but its plain text puts every cell on its own line; the Markdown step on its content node gives clean rows. readeck, go-shiori and domdistiller drop the main table. ECB rates table: trafilatura and readeck drop the rates table (only the text around it is kept); domdistiller keeps it. gov.uk bank holidays: trafilatura drops the date column (the row-header cells), so rows read "Monday / Summer bank holiday" without the date; readeck keeps full rows.
- **html-to-markdown table plugin.** With default options it skips every table that has a line break inside a cell, which is most Wikipedia tables. The spike uses NewlineBehaviorPreserve, minimal padding, SpanBehaviorMirror, SkipEmptyRows and HeaderPromotion.
- **Cookie banners.** On spiegel.de, heise.de, destatis.de and bundesbank.de the consent banner is injected by JavaScript and is not in the served HTML, so these pages do not test banner removal. Banners that are in the HTML: gov.uk (both pages), ECB, go.dev. trafilatura and readeck removed all of them. domdistiller leaked the go.dev banner on docs-go-effective and large-go-spec. go-shiori on the ECB page returned the cookie-policy text instead of the rates page (0/3).
- **Hub pages.** destatis.de "Verbraucherpreisindex" is a topic hub. The labelled paragraphs sit in teaser blocks (` + "`s-contentteaser`" + `) and every extractor drops them (0/3). Hub and index pages are a weak spot for all libraries.
- **JavaScript-only pages.** Excalidraw serves a 6 KB shell. trafilatura returns only "You need to enable JavaScript to run this app." plus the app name; the others return nothing. CoinMarketCap is 61% script, but the server-rendered HTML still has the article text; only the live price (filled in by script) is missing for every library.
- **Ruby text (Japanese).** trafilatura without fallback cuts each paragraph of the Aozora novel at its first ` + "`<ruby>`" + ` element, so most sentences are lost (0/3). With EnableFallback it picks the readeck/distiller result (3/3).
- **Persian.** Body text keeps ZWNJ in every library. trafilatura's title drops the ZWNJ ("بیتکوین" for "بیت‌کوین") and keeps the site suffix, so the wiki-fa title check fails. readeck returns the clean heading.
- **Titles.** trafilatura returns "HTML" for the WHATWG standard (the document's h1), not "HTML Standard".
- **Encodings.** Raw windows-1256 bytes break all five libraries, with or without a ` + "`<meta charset>`" + `: they all guess the charset statistically and pick a wrong one. Decoding in Go first fixes it. Raw windows-1252 and Shift_JIS happened to be guessed right.
`
