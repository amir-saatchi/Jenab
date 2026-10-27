# R1_save_link

Save a link with a note and two tags

Good behaviour: save_link once, with the url, the title, the note and both tags as an array.

## Turn 1

**User:** Save https://example.org/learn/halving to our resources as 'Halving explained', with the note 'good intro for new readers', tagged btc and education.

> call call_tool→save_link {"url":"https://example.org/learn/halving","title":"Halving explained","note":"good intro for new readers","tags":["btc","education"]}

> result save_link:
> saved link 7 to Resources.

**Agent:** Saved the link "Halving explained" to Resources.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 2018 | 256 | 0 | 59 | tool_use | 1.1 s |  |
| 2 | 2087 | 256 | 0 | 12 | end | 0.7 s |  |
