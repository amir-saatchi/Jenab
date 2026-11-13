# anthropic-claude-haiku-5-5_reviewer_c0_r1

Pipeline reviewer, 0 every group, rep 1, 17 s.

**Task:** true (read the config true; error-handling words in the reply 5). Groups [read_data change_data views_pipelines files web memory], recall 1.00, precision 0.33, forbidden 2. Refused 0 (after: ), Mother refused 0. Created 0, sent 0, on the plain message 0.

## 01M7820CXSAWSC078JCSDESRR9 · task

2 requests, 13370 prompt tokens, tools 3144 tokens, system 644 tokens.

**User:** Please review the weekly_summary pipeline.

- `read_config` {"id":"weekly_summary"}
  → version: 1 id: weekly_summary description: Every Monday, a summary of the week's prices and news with the euro rate, saved as a file trigger:   schedule: "0 9 * * 1"   timezone: UTC steps:   - id: prices     use: db.query     with:       sql: SELECT date, price FROM btc_prices ORDER BY date DESC LIM…

**Reply:** I reviewed `weekly_summary` from its config. I didn't dry-run it. The problems, most important first:

**1. The `pages` step gets no URLs, so it will fail or fetch nothing.** The `news` query doesn't select `url`, but `pages` reads `${{ item.url }}`. It also has no date filter, so it isn't limited to "the week's" news.
```yaml
  - id: news
    use: db.query
    with:
      sql: SELECT url, date, title, reason FROM btc_news WHERE date >= date('now', '-7 day') ORDER BY date DESC LIMIT 10
```

**2. The summary never sees the prices, news or EUR rate.** `llm.extract` only receives `steps.pages.each`, so the prices, the news rows and the `fx` response are dropped. The EUR rate is fetched and never used, which breaks the instruction "with prices in USD and EUR". Pass them in explicitly. I'm not sure `llm.extract` accepts extra context fields, so check its options. If it doesn't, put the values into the instruction text:
```yaml
  - id: summary
    use: llm.extract
    with:
      items: ${{ steps.pages.each }}
      instruction: Summarise the week for a Bitcoin holder. Prices (USD, newest first): ${{ steps.prices.rows }}. EUR rate: ${{ steps.fx }}.
      fields: { summary: string }
```

**3. Only one summary is saved.** `save` writes `items[0].summary`. If the extract returns one item per page, the file holds one article's summary, not the week's. Have the extract return a single combined item, and check that the output really has one item.

**4. No error handling.** The `http.get` steps have no retries or timeout. The `pages` loop has no `continue_on_error`, so one dead link stops the whole run. Nothing notifies you when a run fails. I couldn't see a failure-notification setting in the config, so check the app's pipeline notification options.
```yaml
  - id: fx
    use: http.get
    retries: 3
    timeout: 30s
    with:
      url: <real EUR rate endpoint>
  - id: pages
    use: http.get
    for_each: ${{ steps.news.rows }}
    retries: 2
    timeout: 20s
    continue_on_error: true
    with:
      url: ${{ item.url }}
```

**5. Cost is unbounded.** The news query takes up to 50 rows, so the run can make 50 page fetches and send all their text to one LLM call every week. Capping the news query at 10 (in fix 1) keeps both down. If the extract step accepts a text length limit, set one too.

**6. The FX URL is a placeholder.** `https://api.example.com/fx/usd-eur` looks like a stand-in. Replace it with a real endpoint and check that it returns the rate. Since `fx` has no error handling, a bad URL fails the run.

## End state

Tables [btc_news btc_prices watch_keywords]; keywords [bitcoin ETF SEC Fed regulation]; pipelines saved []; files [].
