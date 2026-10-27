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

> call call_tool→connect_api {"id":"coincap","title":"CoinCap API","base_url":"https://api.coincap.example/v2","docs":"https://docs.coincap.example","auth":{"type":"none"},"rate_limit":{"requests":10,"per":"minute"}}

> result connect_api:
> connection coincap proposed; the user confirms it in the form below.

**Agent:** The connection to the CoinCap API has been proposed as **coincap**. Please confirm it in the form that appears below to finalize the setup.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 1709 | 256 | 0 | 13 | tool_use | 1.1 s |  |
| 2 | 448 | 1792 | 0 | 88 | tool_use | 2.4 s |  |
| 3 | 553 | 1792 | 0 | 32 | end | 1.3 s |  |
