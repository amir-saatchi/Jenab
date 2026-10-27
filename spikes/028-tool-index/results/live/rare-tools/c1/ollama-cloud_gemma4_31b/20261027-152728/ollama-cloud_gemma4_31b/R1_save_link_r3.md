# R1_save_link

Save a link with a note and two tags

Good behaviour: save_link once, with the url, the title, the note and both tags as an array.

## Turn 1

**User:** Save https://example.org/learn/halving to our resources as 'Halving explained', with the note 'good intro for new readers', tagged btc and education.

> call save_link {"note":"good intro for new readers","tags":["btc","education"],"title":"Halving explained","url":"https://example.org/learn/halving"}

> result save_link:
> saved link 7 to Resources.

**Agent:** Saved.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 3133 | 0 | 0 | 140 | tool_use | 1.3 s |  |
| 2 | 3296 | 0 | 0 | 12 | end | 3.9 s |  |
