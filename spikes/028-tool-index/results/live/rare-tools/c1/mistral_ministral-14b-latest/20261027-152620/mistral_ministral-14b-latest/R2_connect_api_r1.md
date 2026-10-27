# R2_connect_api

Propose an API connection with no key and a rate limit

Good behaviour: connect_api with auth type none and a rate limit of 10 a minute, as nested objects.

## Turn 1

**User:** Set up a connection to the CoinCap API for us. Call it coincap. The base URL is https://api.coincap.example/v2, the docs are at https://docs.coincap.example, it needs no key, and it allows at most 10 requests a minute.

> call connect_api {"auth":{"type":"none"},"base_url":"https://api.coincap.example/v2","docs":"https://docs.coincap.example","id":"coincap","rate_limit":{"per":"minute","requests":10},"title":"CoinCap API"}

> result connect_api:
> connection coincap proposed; the user confirms it in the form below.

**Agent:** The connection to the CoinCap API is proposed. Please confirm the details in the form below to finalize it:

- **Title:** CoinCap API
- **Base URL:** `https://api.coincap.example/v2`
- **Authentication:** None (no API key required)
- **Rate Limit:** 10 requests per minute
- **Documentation:** [CoinCap Docs](https://docs.coincap.example)

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 168 | 3712 | 0 | 76 | tool_use | 2.0 s |  |
| 2 | 3973 | 0 | 0 | 89 | end | 2.3 s |  |
