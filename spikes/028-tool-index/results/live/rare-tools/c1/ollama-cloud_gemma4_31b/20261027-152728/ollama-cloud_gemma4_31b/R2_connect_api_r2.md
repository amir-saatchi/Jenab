# R2_connect_api

Propose an API connection with no key and a rate limit

Good behaviour: connect_api with auth type none and a rate limit of 10 a minute, as nested objects.

## Turn 1

**User:** Set up a connection to the CoinCap API for us. Call it coincap. The base URL is https://api.coincap.example/v2, the docs are at https://docs.coincap.example, it needs no key, and it allows at most 10 requests a minute.

> call connect_api {"auth":{"name":"","prefix":""},"base_url":"https://api.coincap.example/v2","docs":"https://docs.coincap.example","id":"coincap","rate_limit":{"per":"minute","requests":10},"title":"CoinCap API"}

> result connect_api (error):
> wrong arguments:
> - at /auth: missing property 'type'

> call connect_api {"auth":{"name":"","prefix":"","type":"none"},"base_url":"https://api.coincap.example/v2","docs":"https://docs.coincap.example","id":"coincap","rate_limit":{"per":"minute","requests":10},"title":"CoinCap API"}

> result connect_api:
> connection coincap proposed; the user confirms it in the form below.

**Agent:** I've proposed the connection to the CoinCap API. Please confirm it in the form.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 3162 | 0 | 0 | 475 | tool_use | 3.6 s |  |
| 2 | 3667 | 0 | 0 | 496 | tool_use | 9.2 s |  |
| 3 | 4193 | 0 | 0 | 28 | end | 1.5 s |  |
