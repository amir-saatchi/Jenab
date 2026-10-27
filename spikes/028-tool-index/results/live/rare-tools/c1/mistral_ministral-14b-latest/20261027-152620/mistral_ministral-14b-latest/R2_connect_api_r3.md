# R2_connect_api

Propose an API connection with no key and a rate limit

Good behaviour: connect_api with auth type none and a rate limit of 10 a minute, as nested objects.

## Turn 1

**User:** Set up a connection to the CoinCap API for us. Call it coincap. The base URL is https://api.coincap.example/v2, the docs are at https://docs.coincap.example, it needs no key, and it allows at most 10 requests a minute.

> call connect_api {"auth":{"type":"none"},"base_url":"https://api.coincap.example/v2","docs":"https://docs.coincap.example","id":"coincap","rate_limit":{"per":"minute","requests":10},"title":"CoinCap API"}

> result connect_api:
> connection coincap proposed; the user confirms it in the form below.

**Agent:** The connection to the CoinCap API is proposed and ready for confirmation. Please confirm the connection in the form that appeared.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 40 | 3840 | 0 | 76 | tool_use | 1.9 s |  |
| 2 | 3717 | 256 | 0 | 25 | end | 1.5 s |  |
