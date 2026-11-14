# anthropic-claude-haiku-5-5_reviewer_c1_r3

Pipeline reviewer, 1 key, rep 3, 14 s.

**Task:** true (read the config true; error-handling words in the reply 7). Groups [read_data views_pipelines], recall 1.00, precision 1.00, forbidden 0. Refused 0 (after: ), Mother refused 0. Created 0, sent 0, on the plain message 0.

## 01M782PM5SQB664MXX7YN5MX01 · task

3 requests, 14536 prompt tokens, tools 1791 tokens, system 577 tokens.

**User:** Please review the weekly_summary pipeline.

- `read_config` {"id":"weekly_summary"}
  → version: 1 id: weekly_summary description: Every Monday, a summary of the week's prices and news with the euro rate, saved as a file trigger:   schedule: "0 9 * * 1"   timezone: UTC steps:   - id: prices     use: db.query     with:       sql: SELECT date, price FROM btc_prices ORDER BY date DESC LIM…
- `read_config` {"id":"daily_btc"}
  → version: 1 id: daily_btc description: Daily BTC price and the 5 news items most likely to affect it trigger:   schedule: "0 8 * * *"   timezone: UTC   catch_up: once   manual: true steps:   - id: price     use: http.get     with:       url: https://api.example.com/btc/daily   - id: save_price     us…

**Reply:** Problems in `weekly_summary`, most important first:

1. **The page fetch can't work: `item.url` is never selected.** The `news` query returns only `date, title, reason`, so `pages` gets no URLs. Fix:
```yaml
  - id: news
    use: db.query
    with:
      sql: SELECT url, date, title, reason FROM btc_news WHERE date >= date('now', '-7 days') ORDER BY date DESC
```
The `WHERE` also limits the news to the past week. The `LIMIT 50` stays out of the way, but it was not a week filter.

2. **The EUR rate is fetched but never used.** `fx` runs, yet `llm.extract` only gets `pages`, and the instruction asks for EUR. Pass the rate in. The exact field path depends on the API's response, so check it first:
```yaml
  - id: summary
    use: llm.extract
    with:
      items: ${{ steps.pages.each }}
      context: { usd_eur: ${{ steps.fx.json.rate }} }
      instruction: Summarise the week for a Bitcoin holder, with prices in USD and EUR (use usd_eur for conversion).
      fields: { summary: string }
```

3. **Unbounded cost and fan-out.** `pages` fetches every news page, and `llm.extract` sends all of them in one call. Nothing caps the size, so one long page or a larger news table can blow the context or the budget. Cap the input and the fetches:
```yaml
  - id: news
    use: db.query
    with:
      sql: SELECT url, date, title, reason FROM btc_news WHERE date >= date('now', '-7 days') ORDER BY date DESC LIMIT 10
```

4. **No timeouts, retries or error handling on network steps.** `fx`, `pages` (one request per item) and the summary call can hang or fail transiently, and a single failed page currently stops the whole run. Add to each network step:
```yaml
    timeout: 30s
    retry: { attempts: 3, backoff: 10s }
    continue_on_error: true   # pages only: skip a failed page
```
I couldn't confirm these key names from this project's configs, since none of them use retries or timeouts. Check them against the step reference before saving.

5. **Silent failure on save and no failure notice.** `save` writes `steps.summary.items[0].summary` with no check. If the LLM returns nothing, an empty summary file is saved and the run looks successful. The pipeline also has no notification on failure, so a broken Monday run goes unnoticed. Add a guard and a notification (again, check the exact key names):
```yaml
  - id: save
    use: bucket.put
    when: ${{ steps.summary.items[0].summary != "" }}
    with:
      key: summaries/${{ today() }}.md
      content: ${{ steps.summary.items[0].summary }}
on_failure:
  notify: true
```

6. **Minor:** the `fx` and `pages` steps use `api.example.com` and `news.example.com`-style placeholder hosts. Confirm they're real endpoints before relying on the run.

## End state

Tables [btc_news btc_prices watch_keywords]; keywords [bitcoin ETF SEC Fed regulation]; pipelines saved []; files [].
