# R2_connect_api

Propose an API connection with no key and a rate limit

Good behaviour: connect_api with auth type none and a rate limit of 10 a minute, as nested objects.

## Turn 1

**User:** Set up a connection to the CoinCap API for us. Call it coincap. The base URL is https://api.coincap.example/v2, the docs are at https://docs.coincap.example, it needs no key, and it allows at most 10 requests a minute.

> call connect_api {"auth":{"type":"none"},"base_url":"https://api.coincap.example/v2","docs":"https://docs.coincap.example","id":"coincap","rate_limit":{"per":"minute","requests":10},"title":"CoinCap API"}

> result connect_api:
> connection coincap proposed; the user confirms it in the form below.

**Agent:** The connection to the CoinCap API is ready for confirmation. Please confirm the details in the chat form below:

- **ID:** coincap
- **Title:** CoinCap API
- **Base URL:** [https://api.coincap.example/v2](https://api.coincap.example/v2)
- **Authentication:** No key required
- **Rate Limit:** 10 requests per minute
- **Docs:** [https://docs.coincap.example](https://docs.coincap.example)

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 3880 | 0 | 0 | 76 | tool_use | 3.6 s |  |
| 2 | 3717 | 256 | 0 | 105 | end | 2.9 s |  |
