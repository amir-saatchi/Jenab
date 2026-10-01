# SPIKE-020 — Keyless web search
**Type:** Spike
**Status:** Done
**Gate:** 2

## Question
Can Jenab offer a free, keyless web search out of the box, using only interfaces meant for programmatic access and no scraping (PROPOSAL "Bring your own keys")?

1. **SearXNG:** a local instance (the official Docker image) and a few public instances. Does the JSON API work, and how often are requests blocked or limited? Which engines return results?
2. **DuckDuckGo:** what its official endpoints return for a normal web query (Instant Answer API), and whether anything else DuckDuckGo offers is allowed.
3. **Other keyless sources for the tracker domain:** GDELT (news), Wikipedia, Hacker News (Algolia), RSS feeds, Marginalia's public key. What do they return for the Bitcoin example?
4. **Keyed providers (from their docs, no account):** Brave, Tavily, Exa, Mojeek. Free tier, limits, and terms for a desktop app.
5. **Quality:** result counts, freshness and relevance for 10 tracker-style queries, and speed.

## Done when
Each source has a row: keyless or not, allowed by its terms, works, quality and speed. There is a decision on the default search for v1, and on what the proposal may promise.

## Result
Run on 2026-09-28 from one home IP, with 10 queries (8 English, 1 German, 1 Persian). No keys, accounts or scraping of result pages. Code and data: `spikes/020-keyless-search/` (`results.md`).

**The promise "a free, keyless search works out of the box" does not hold.** No keyless, allowed, general web-search API worked.

| Source | Keyless | Terms allow app use | Works | Quality (top 5) | Speed (median) |
|---|---|---|---|---|---|
| SearXNG, local (Docker) | yes | yes, but SearXNG itself scrapes Google, Bing and others | **not tested**: Docker Desktop failed to start on this machine | – | – |
| SearXNG, 8 public instances | yes | in practice no: about 4 API requests per IP per hour | **no**: 0 of 16 requests (429, 403 or a bot check) | – | – |
| DuckDuckGo Instant Answer | yes | non-commercial, with attribution | **not a web search**: 0 results for 10/10 queries | – | 0.14 s |
| DuckDuckGo html/lite | – | no: no API, `robots.txt` disallows it | not tested | – | – |
| GDELT DOC 2.0 (news) | yes | yes, with citation | **unreliable**: 12 of 44 attempts got through; 429 even at 30 s apart; nothing for German or Persian | dated; 50% relevant | 16 s |
| Wikipedia search | yes | yes, with a User-Agent that has contact info | **yes**: 10/10 | no dates; 24% relevant; German and Persian good | 0.5 s |
| Hacker News (Algolia) | yes | no terms; 10,000 requests/h per IP | **yes**: 10/10 | dated; 60% relevant; tech only | 0.45 s |
| Marginalia `public` key | shared key | results CC-BY-NC-SA | **no**: 13/13 got 429 (shared limit used up) | – | – |
| RSS/Atom (3 crypto feeds) | yes | normal feed reading; publishers restrict reuse | **yes**: 3/3 feeds, 111 items, all dated | Bitcoin query: 36 matches, 18 from the last 7 days, 5/5 relevant | 0.7 s per feed |
| Google News RSS | yes | **no**: `robots.txt` disallows `/rss` | not tested | – | – |

**Keyed providers (from their docs, no account):**

| Provider | Free tier | Price after | Dates | Terms that matter |
|---|---|---|---|---|
| Tavily | 1,000 credits/month | $0.008/credit | yes | no ban found on use in apps or storage |
| Brave | $5 credit/month (~1,000 searches), needs attribution | $5 / 1,000 | yes | no "database of Search Results" beyond transient storage |
| Exa | $10 credit/month (~1,400 searches) | $7 / 1,000 | yes | no copying or redistributing results |
| Mojeek | no self-serve free tier | £2–3 / 1,000 | yes | 1 h caching |
| SerpApi, Serper, SearchApi | – | – | – | they scrape Google and Bing: not allowed by the proposal |

Bing's Web Search API was retired on 2025-08-11, and Google's Custom Search JSON API is closed to new customers.

**Other findings:**
- **Query shape:** HN needs every word to match, and GDELT rejects words shorter than 3 characters. So the keyless adapters need 2–4 keywords with no filler words.
- **GDELT:**
  - It seems to match English keywords only, so Jenab should query it in English and add `sourcelang:`.
  - Its TLS handshake took 10–13 s, over Go's 10 s default.
- **Wikipedia:** it needs the wiki picked from the query language (`de.`, `fa.`).
- **Freshness:** only RSS and GDELT with `timespan=7d` returned items from the last 7 days. HN needs `search_by_date`.

## Decision
**Decided (2026-09-28):**
1. **No keyless web search in v1.** `web_search` and `web.search` use a provider the user sets up:
   - **Tavily** (default suggestion) or **Brave**, with the user's key.
   - Or the URL of a **SearXNG** instance the user runs. It's documented, but not the default.
   - Without a provider, the tool returns a clear "no search provider set up" error, and saving a pipeline with `web.search` fails validation with the same hint.
2. **Keyless sources get their own tools, and don't count as web search:**
   - a `feed.read` step and a `read_feed` tool for RSS/Atom (the user adds the feeds)
   - a `source` input on `web.search` and `web_search`: `web` (the provider, default), `news` (GDELT, best effort), `wikipedia` or `hn`
   - The adapters follow the rules in "Other findings".
   - For GDELT: a 60 s timeout and backoff on 429.
   - All requests send a User-Agent with contact info.
3. **Bitcoin example (SPEC 9):** news comes from 2–3 crypto feeds, filtered by the watch keywords and ranked by `llm.select`, with GDELT as an optional extra. That keeps the Phase 5 goal keyless. Keyless price API: not covered here; still to pick.
4. **Brave's storage clause:** a pipeline that saves search results into a table is a grey area under Brave's terms. So Tavily is the suggested default.
5. **Proposal wording (for PROPOSAL "Bring your own keys"):** "Web search uses search APIs built for programmatic access, never scraping of search result pages. Bring a key for a search API (several have free monthly tiers) or point Jenab at a SearXNG instance you run. Without either, Jenab still has keyless sources for trackers: RSS/Atom feeds, Wikipedia, Hacker News and GDELT news (best effort)." Drop "DuckDuckGo" and "works out of the box".
6. **Open:** measure local SearXNG and its engine block rates once Docker works (the command is in the spike README). This doesn't change the decision.

Doc changes (made in SPEC v0.5 and PROPOSAL):
- SPEC 6.5 (`feed.read`, `source`), 8.1 (`read_feed`, `source`), 9.2 (feeds instead of `web.search`), 10 (the provider check) and the changelog
- PROPOSAL lines 134, 173 and 252, and the Phase 5 "Done when"
