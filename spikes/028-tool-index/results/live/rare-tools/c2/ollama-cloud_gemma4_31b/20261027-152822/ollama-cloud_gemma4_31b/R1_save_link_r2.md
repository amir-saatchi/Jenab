# R1_save_link

Save a link with a note and two tags

Good behaviour: save_link once, with the url, the title, the note and both tags as an array.

## Turn 1

**User:** Save https://example.org/learn/halving to our resources as 'Halving explained', with the note 'good intro for new readers', tagged btc and education.

> call describe_tool {"name":"save_link"}

> result describe_tool:
> save_link
> Save a link to the project's Resources panel, with a title, an optional note and tags.
> 
> Arguments (JSON Schema):
> {"additionalProperties":false,"properties":{"note":{"type":"string"},"tags":{"items":{"pattern":"^[a-z0-9-]+$","type":"string"},"maxItems":10,"type":"array"},"title":{"minLength":1,"type":"string"},"url":{"pattern":"^https?://","type":"string"}},"required":["url","title"],"type":"object"}

> call call_tool→save_link {"note":"good intro for new readers","tags":["btc","education"],"title":"Halving explained","url":"https://example.org/learn/halving"}

> result save_link:
> saved link 7 to Resources.

**Agent:** Saved 'Halving explained' to your resources.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 1884 | 0 | 0 | 73 | tool_use | 1.2 s |  |
| 2 | 2088 | 0 | 0 | 126 | tool_use | 1.0 s |  |
| 3 | 2236 | 0 | 0 | 20 | end | 0.6 s |  |
