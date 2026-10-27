# R2_connect_api

Propose an API connection with no key and a rate limit

Good behaviour: connect_api with auth type none and a rate limit of 10 a minute, as nested objects.

## Turn 1

**User:** Set up a connection to the CoinCap API for us. Call it coincap. The base URL is https://api.coincap.example/v2, the docs are at https://docs.coincap.example, it needs no key, and it allows at most 10 requests a minute.

> call describe_tool {"name":"connect_api"}

> result describe_tool:
> connect_api
> Propose a connection to a web API. The user adds the key, if one is needed, and confirms it in a chat form.
> 
> Arguments (JSON Schema):
> {"additionalProperties":false,"properties":{"auth":{"additionalProperties":false,"properties":{"name":{"description":"the header or query parameter that carries the key","type":"string"},"prefix":{"description":"text before the key, such as \"Bearer \"","type":"string"},"type":{"enum":["none","header","query"],"type":"string"}},"required":["type"],"type":"object"},"base_url":{"pattern":"^https://","type":"string"},"docs":{"pattern":"^https?://","type…

> call call_tool→connect_api {"auth":{"type":"none"},"base_url":"https://api.coincap.example/v2","docs":"https://docs.coincap.example","id":"coincap","rate_limit":{"per":"minute","requests":10},"title":"CoinCap API"}

> result connect_api:
> connection coincap proposed; the user confirms it in the form below.

**Agent:** I've proposed the connection to the CoinCap API. Please confirm it in the form.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 1913 | 0 | 0 | 68 | tool_use | 1.1 s |  |
| 2 | 2254 | 0 | 0 | 210 | tool_use | 1.4 s |  |
| 3 | 2493 | 0 | 0 | 34 | end | 3.9 s |  |
