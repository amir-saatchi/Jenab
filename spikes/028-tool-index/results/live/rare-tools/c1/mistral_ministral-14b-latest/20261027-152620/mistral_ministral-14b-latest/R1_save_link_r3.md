# R1_save_link

Save a link with a note and two tags

Good behaviour: save_link once, with the url, the title, the note and both tags as an array.

## Turn 1

**User:** Save https://example.org/learn/halving to our resources as 'Halving explained', with the note 'good intro for new readers', tagged btc and education.

> call save_link {"url":"https://example.org/learn/halving","title":"Halving explained","note":"good intro for new readers","tags":["btc","education"]}

> result save_link:
> saved link 7 to Resources.

**Agent:** Saved the link to Resources.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 3599 | 256 | 0 | 48 | tool_use | 1.8 s |  |
| 2 | 3657 | 256 | 0 | 7 | end | 1.0 s |  |
