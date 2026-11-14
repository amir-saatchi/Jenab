# anthropic-claude-haiku-5-5_reviewer_c0_r3

Pipeline reviewer, 0 every group, rep 3, 11 s.

**Task:** true (read the config true; error-handling words in the reply 5). Groups [read_data change_data views_pipelines files web memory], recall 1.00, precision 0.33, forbidden 2. Refused 0 (after: ), Mother refused 0. Created 0, sent 0, on the plain message 0.

## 01M782PFM999CKW9VS09WH61T7 · task

2 requests, 13381 prompt tokens, tools 3144 tokens, system 644 tokens.

**User:** Please review the weekly_summary pipeline.

- `read_config` {"id":"weekly_summary"}
  → version: 1 id: weekly_summary description: Every Monday, a summary of the week's prices and news with the euro rate, saved as a file trigger:   schedule: "0 9 * * 1"   timezone: UTC steps:   - id: prices     use: db.query     with:       sql: SELECT date, price FROM btc_prices ORDER BY date DESC LIM…

**Reply:** Review of `weekly_summary`. Most important first:

1. **The summary only uses the first item, so the run does much of its work for nothing.** `save` writes `steps.summary.items[0].summary`, so the other items the LLM returns are discarded. The `pages` step also fetches every news URL (up to 50) for that one result. Fix: have the LLM step take one combined input and write its whole output.
   ```yaml
   - id: summary
     use: llm.extract
     with:
       items:
         - prices: ${{ steps.prices.rows }}
           news: ${{ steps.pages.each }}
           fx: ${{ steps.fx.body }}
       instruction: Summarise the week for a Bitcoin holder, with prices in USD and EUR, using the euro rate from fx.
       fields: { summary: string }
   - id: save
     use: bucket.put
     with:
       key: summaries/${{ today() }}.md
       content: ${{ steps.summary.items[0].summary }}
   ```

2. **The news query doesn't return the URLs, so `pages` can't work.** `item.url` is read from `SELECT date, title, reason`, which has no `url` column. Fix:
   ```yaml
   - id: news
     use: db.query
     with:
       sql: SELECT url, date, title, reason FROM btc_news WHERE date >= date('now', '-7 days') ORDER BY date DESC
   ```
   The `WHERE` also replaces the arbitrary `LIMIT 50` with the actual week, and the same `date` filter applies to prices if you want exactly seven days.

3. **The euro rate is fetched but never used.** `fx` has no consumer, and the instruction asks for EUR prices without it. Fixed by the snippet in item 1, but confirm the `fx` value is actually the rate and not a placeholder. The URL `api.example.com` looks like a placeholder, so verify it before relying on it.

4. **No error handling on network steps.** `fx` and every `pages` call have no retries, timeout or `continue_on_error`. One dead article URL fails the whole run, and nothing tells you. Fix:
   ```yaml
   - id: fx
     use: http.get
     retries: 3
     timeout: 20s
     with:
       url: https://api.example.com/fx/usd-eur   # verify this URL
   - id: pages
     use: http.get
     for_each: ${{ steps.news.rows }}
     retries: 2
     timeout: 15s
     continue_on_error: true
     with:
       url: ${{ item.url }}
   ```
   Also add a failure notification in the pipeline's settings, so a missed Monday summary is noticed.

5. **Unbounded LLM cost.** Up to 50 full pages go into one `llm.extract` call, with no cap on input size. Fix: keep the week's news to a limit and cap the text per page, for example `limit: 20` in the news query and a truncation in the input (for example `text: ${{ truncate(item.body, 2000) }}`, if the step language supports it). If that function doesn't exist, pass only `title` and `reason` from `btc_news`, which the DB already has, and skip the page fetch altogether.

Nothing has been changed. I didn't dry-run it; I can do that after you apply the fixes.

## End state

Tables [btc_news btc_prices watch_keywords]; keywords [bitcoin ETF SEC Fed regulation]; pipelines saved []; files [].
