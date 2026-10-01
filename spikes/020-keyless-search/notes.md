Test run: 2026-09-28, from the test machine's network (one public IP). The spike used no API keys, no accounts and no scraping of result pages. Terms and pricing were read the same day from the linked official pages.

## Summary

The proposal's promise does not hold as written. There is no keyless, sanctioned, general web-search API that works out of the box.

- **DuckDuckGo** has no web-results API. The Instant Answer API returned 0 results for all 10 queries. It only answers plain entities: the control query "Bitcoin" returned a Wikipedia abstract and 3 links.
- **Public SearXNG** refused `format=json` on all 8 instances tested, on the first request. 6 answered HTTP 429 or 403, and 2 answered a bot-check page.
- **Marginalia's shared `public` key** was used up. All 13 requests got 429 "QPM/Daily Limit Exceeded".
- **GDELT** is keyless and its terms are generous, but today it was barely usable:
  - Only 12 of 44 HTTP attempts got through; the rest were 429, even at one request per 30 s.
  - Every TLS handshake took 10–13 s. On the first run all 20 requests hit Go's default 10 s handshake timeout.
- **Local SearXNG**, the only realistic keyless general web search, was **not tested**. Docker Desktop failed to start on this machine (Docker's "Inference manager" could not remove a stale socket file). No container was created and no image was pulled.

What works without a key:

- **Wikipedia** (10/10 queries answered, ~0.5 s). Reference only, no publication dates.
- **Hacker News Algolia** (10/10 answered, ~0.45 s, dated). Tech topics only.
- **RSS/Atom feeds** (3/3 feeds fetched, every item dated). They were the best source for the Bitcoin example: 36 matches for q01, 18 of them from the last 7 days (Decrypt's feed also carries evergreen items back to 2025-12), and 5/5 of the top 5 relevant. They only cover the topics of the feeds you pick.

## Per source

"works" = measured today. Numbers are the top-5 relevance and latency from section 1.

| source | keyless | terms allow use in an app | works | results / dated / ≤7 d / relevant | latency (median / max) | limits |
|---|---|---|---|---|---|---|
| SearXNG, local (Docker) | yes | yes (AGPL software; the upstream engines are scraped, see notes) | **not tested** (Docker failed) | – | – | depends on the upstream engines |
| SearXNG, public instances | yes | not for bots in practice: the limiter allows about 4 API requests per IP per hour | **no**: 0 of 16 requests, 8/8 instances refused JSON | – | 0.6–1.8 s to the refusal | 429 / 403 / bot-check page |
| DuckDuckGo Instant Answer | yes | non-commercial with attribution, or by email approval | **not a web search**: 0/10 queries | 0 | 0.14 / 1.0 s | none hit |
| DuckDuckGo html/lite | – | no API; `robots.txt` disallows `/html` and `/lite` | not tested (scraping) | – | – | – |
| GDELT DOC 2.0 | yes | yes: "unlimited and unrestricted use", with citation | **unreliable**: 27% of attempts got through; non-English keywords found nothing | 9 successful requests over 3 variants (6 with results, 1–25 each), 100% dated, 7% ≤7 d with the default 3-month window and 100% with `timespan=7d`, 50% relevant (9/18) | 16 / 23 s | "one every 5 seconds", but 429 even at 30 s apart |
| Wikipedia (MediaWiki search) | yes | yes, with a UA that includes contact info | **yes**: 10/10 | 10 results, 0% dated (last-edit time only), 24% relevant (7/10 queries had a hit) | 0.5 / 1.2 s | 200/min with a UA (new in 2026) |
| HN Algolia | yes | no terms published; rate limit only | **yes**: 10/10 (7 with results) | median 2, 100% dated, 0% ≤7 d (sorted by relevance), 60% relevant | 0.45 / 0.8 s | 10,000/h per IP |
| Marginalia (`public` key) | shared key | CC-BY-NC-SA results; the `public` key is "for experimentation" | **no**: 13/13 got 429 | – | 0.14 s to the 429 | shared key used up; own keys are free by email |
| RSS/Atom (CoinDesk, Cointelegraph, Decrypt) | yes | normal RSS reading, yes. Publisher terms restrict automated access and reuse; Cointelegraph bans AI use | **yes**: 3/3 feeds | 111 items, 100% dated. q01: 36 matches, 18 ≤7 d (top 10 all ≤7 d), 5/5 relevant. Other queries: 0 (off-topic for crypto feeds) | 0.55–0.9 s per feed | none hit |
| Google News RSS search | yes | **no**: `news.google.com/robots.txt` disallows `/rss`, and Google's ToS ties automated access to robots.txt | not tested | – | – | – |

## Recommendation for v1

1. **Don't ship a keyless "web search" as the default promise.** `web_search` needs a provider the user configures. There are three kinds:
   - **A keyed API, the recommended path.** Pick Tavily or Brave (their free monthly tiers cover a personal tracker), plus Exa as a third.
   - **A SearXNG URL.** The user's own or local instance. Document the Docker one-liner in `README.md`. It is the keyless option for people who want it. It still needs a measured rerun before we recommend it; see "Open" below.
   - **None.** Then the tool says so clearly, instead of silently returning nothing.
2. **Ship keyless news and reference sources as first-class tools and pipeline steps, separate from `web_search`:**
   - **RSS/Atom (`feed.fetch`).** The most reliable and freshest keyless source, and all items are dated. The user picks the feeds; example templates suggest them.
   - **Wikipedia search**, for reference: pick the wiki from the query language.
   - **HN Algolia**, for tech and release tracking: use `search_by_date` or a `created_at_i` filter for freshness.
   - **GDELT**, as a best-effort news search:
     - long timeouts (60 s) and a 429 backoff
     - results cached for a day
     - `sourcelang:english` (or the user's language) added to the query
     - words under 3 characters dropped, or phrases quoted: GDELT rejected "Go 1.26 release", "Notion vs Obsidian" and "how to build a daily meditation habit" with "keyword that was too short"
     - never the only source in a pipeline
3. **The Bitcoin example can be reproduced keyless** by using feeds, not web search:
   - the price comes from a keyless price API (not part of this spike)
   - "the 5 news items most likely to affect it" come from 2–3 crypto RSS feeds, with GDELT as an optional extra and the LLM ranking the items
   - This is honest and it worked today: 36 dated matches, 18 from the last 7 days, with 5/5 of the top 5 relevant.
4. **What needs a user key:** general web search in any language, including for "competitors", "habits" and "how to" queries. Keyless sources answered those poorly: Wikipedia got 0/5 relevant for "Notion vs Obsidian" (HN got 3/5), and 1/5 for the meditation habit query (HN 0/2).

### What the proposal can honestly promise

Suggested wording, for the main session to adapt:

> "Web search uses search APIs built for programmatic access, never scraping of search result pages. You can bring a key for a search API (several have free monthly tiers), or point Burrow at a SearXNG instance you run yourself. Without either, Burrow still has keyless sources for trackers: RSS/Atom feeds, Wikipedia, Hacker News, and GDELT news search (best effort)."

Drop "DuckDuckGo" from the promise. Its only API is not a web search, and DuckDuckGo says it does not have the rights to syndicate its results. Drop "works out of the box" for search.

Phase 5: change the goal to "with keyless data providers (feeds and news APIs) and their own LLM key". Keep web search out of the keyless claim.

### Open

- **Local SearXNG is still unmeasured.** Rerun `go run . -collect searxng-local -rounds 3` once Docker works (see README). The report already has the per-engine table.
- **Also note for users:** SearXNG itself scrapes the upstream engines' result pages (Google, Bing, DuckDuckGo, Brave, and others). Burrow calling a SearXNG instance the user runs is the user's choice. But offering it as Burrow's default would go against the proposal's own "never scraping" line, even at one remove.

<!-- TABLES -->

## 6. Keyed providers (public docs only; no accounts, keys or sign-ups)

Checked 2026-09-28. "Dates" means whether results carry a publication date.

| provider | free tier | price after | rate limit | dates in results | terms for a desktop app with the user's own key |
|---|---|---|---|---|---|
| Brave Search API | $5 of credit every month (about 1k searches), which needs attribution on your site/about page. The old free 2k/month plan is gone | $5 / 1k requests (web, news) | 50 QPS | yes: `page_age` (published or modified), plus a `freshness` parameter (pd/pw/pm/py or a date range) | Using it with the user's own key is fine. **But:** you may not "store, cache, or create a database of Search Results" beyond transient storage, unless your plan grants storage rights. That matters if pipelines write results into tables |
| Tavily | 1,000 credits a month (basic search = 1 credit, advanced = 2), no card | $0.008 per credit pay-as-you-go ($0.005–0.0075 on plans) | 100 RPM on a dev key, 1,000 RPM in production | yes: `published_date` (automatic with `topic=news`), plus `time_range` and `start_date`/`end_date` | Keys may not be shared with third parties; one key per user fits that. I found no ban on use inside apps or on storage |
| Exa | $10 of credit a month (about 1,400 searches) | $7 / 1k searches (up to 10 results) | 10 QPS for `/search` | yes: `publishedDate` (an estimate), plus `startPublishedDate`/`endPublishedDate` | The licence is non-transferable. §4.2(a) bans copying or redistributing results beyond browser caching. Grey for storing results in tables |
| Mojeek | no self-serve free tier ("free trial ... get in touch") | £2 CPM (Startup: 5 QPS, 100k/day), £3 CPM (Business) | 5–10 QPS | last-modified date with `date=1`, plus `since`/`before` filters | Results may be used for AI on all plans. Caching for 1 h is allowed; longer storage needs the Business plan |
| SerpApi / Serper / SearchApi | 250 / 2,500 / 100 free searches | varies | varies | varies | **They scrape Google and Bing result pages**. SerpApi: "Scrape Google and other search engines from our fast, easy, and complete API." Not compatible with the proposal |
| Bing Web Search API | – | – | – | – | Retired on 2025-08-11 |
| Google Custom Search JSON API | 100/day | $5 / 1k | – | – | "closed to new customers"; existing customers have until 2027-01-01 |

Sources: [brave.com/search/api](https://brave.com/search/api/), [Brave API terms](https://api-dashboard.search.brave.com/documentation/resources/terms-of-service), [Brave blog, 2026-02](https://brave.com/blog/most-powerful-search-api-for-ai/), [docs.tavily.com credits](https://docs.tavily.com/documentation/api-credits), [Tavily rate limits](https://docs.tavily.com/documentation/rate-limits), [tavily.com/terms](https://tavily.com/terms), [exa.ai/pricing](https://exa.ai/pricing), [Exa rate limits](https://exa.ai/docs/reference/rate-limits), [exa.ai/terms](https://exa.ai/terms), [Mojeek API](https://www.mojeek.com/services/search/web-search-api/), [serpapi.com](https://serpapi.com/), [Bing retirement](https://learn.microsoft.com/en-us/lifecycle/announcements/bing-search-api-retirement), [Google CSE](https://developers.google.com/custom-search/v1/overview).

**Pick for v1:** Tavily and Brave as the built-in keyed providers. Both have a free monthly tier that fits a personal tracker, and both return dates and a freshness filter. Tavily's terms are the least restrictive about keeping results. Brave has its own index (not a scraper) and more QPS, but it needs attribution on the free credit, and its "no database of results" clause means Burrow should store only what the user's pipeline extracts, not raw result lists. Exa is optional. Mojeek needs a paid contract, so it is not a default.

## 7. Terms that matter (short quotes)

- **DuckDuckGo** ([API page source](https://github.com/duckduckgo/duckduckgo-publisher/blob/master/share/site/duckduckgo/api.tx)):
  - "it is not a full search results API"
  - "we unfortunately do not have the rights to fully syndicate our results, free or paid"
  - [duckduckgo.com/robots.txt](https://duckduckgo.com/robots.txt) has `Disallow: /html` and `Disallow: /lite`.
- **SearXNG** ([search API docs](https://docs.searxng.org/dev/search_api.html)):
  - "many public instances have these formats disabled"
  - `search.formats` defaults to html only. The [limiter](https://docs.searxng.org/src/searx.botdetection.html) allows about 4 non-HTML requests per IP per hour (`API_MAX = 4`, `API_WINDOW = 3600`).
- **GDELT** ([about](https://www.gdeltproject.org/about.html)): "available for unlimited and unrestricted use for any academic, commercial, or governmental use". Its 429 body says: "Please limit requests to one every 5 seconds".
- **Wikimedia:**
  - [User-Agent policy](https://foundation.wikimedia.org/wiki/Policy:User-Agent_policy): generic user agents may be "blocked without notice". Burrow must send a UA with contact info.
  - [Rate limits](https://www.mediawiki.org/wiki/Wikimedia_APIs/Rate_limits): 200/min with a UA, 10/min for an IP only. These are "new in 2026".
- **Hacker News Algolia** ([hn.algolia.com/api](https://hn.algolia.com/api)): "limiting the number of API requests from a single IP to 10,000 per hour".
- **Marginalia** ([API](https://about.marginalia-search.com/article/api/)):
  - The `public` key "often hits a shared rate limit". Today it answered 429, not the documented 503.
  - Results are "Provided under CC-BY-NC-SA 4.0". Free personal keys are available by email.
- **Google News RSS:**
  - [robots.txt](https://news.google.com/robots.txt) has `User-agent: *`, `Disallow: /`, and no `Allow` for `/rss`.
  - [Google ToS](https://policies.google.com/terms) forbids automated access "in violation of the machine-readable instructions on our web pages (for example, robots.txt".
  - There is no official documentation of the search feed.
- **Crypto feeds:**
  - [CoinDesk terms](https://www.coindesk.com/terms): no "automated means to access the Services ... without our express written permission".
  - [Cointelegraph terms](https://cointelegraph.com/terms-and-privacy) ban redistribution and AI use without consent.
  - [Decrypt terms](https://decrypt.co/terms-of-service) require non-commercial use.
  - A user subscribing to a feed in a reader is normal RSS use. Burrow should let the user add feeds and should not ship publisher content. Templates can suggest feed URLs, with a note.

## 8. Other observations

- **Query shape matters for keyless APIs.** HN Algolia requires every word, so "bitcoin price news this week" returned 0 hits. GDELT rejects words shorter than 3 characters. Burrow's adapters, or the tool description for the LLM, should ask for 2–4 keyword queries with no filler words.
- **Language handling:**
  - Wikipedia handles German and Persian well, as long as Burrow picks `de.`/`fa.wikipedia.org` from the query language (5/5 top results in the right language for both). The Persian query also matched the ZWNJ spelling بیت‌کوین.
  - GDELT found **0** articles for "Bundesbank Zinsen" and for the Persian query over 3 months. For English queries it returns articles in many languages (Turkish, Russian, Lithuanian, Macedonian for the ECB query). The keywords seem to be matched against GDELT's English translation, so Burrow should query in English and add `sourcelang:`.
  - HN and the crypto feeds are English-only.
- **Freshness:**
  - Only GDELT with `timespan=7d` and the RSS feeds gave results from the last 7 days.
  - HN sorted by relevance returned old stories (2017–2025); `search_by_date` fixes that.
  - Wikipedia gives only the time of the last edit (stored as `mod_date`, not counted as a date).
- **Speed:**
  - Wikipedia, HN and the RSS feeds: 0.4–0.9 s.
  - GDELT: 11–23 s per attempt, because the TLS handshake alone took 10–13 s. Its default 10 s handshake timeout in Go fails, so the adapter needs a 30 s handshake timeout.
- **Politeness log:**
  - Public SearXNG: 2 requests per instance.
  - GDELT: 20 attempts that timed out in TLS, then 34 attempts including retries at ≥10 s apart, then 10 attempts at 30 s apart, plus 2 manual probes.
  - Marginalia: 26 attempts, half of them the single retry after 15 s.
  - Every other host: ≤10 requests at ≥1.1 s apart.
