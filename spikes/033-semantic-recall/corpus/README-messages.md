# SPIKE-033 message corpus

The message half of the corpus for [SPIKE-033](../../../Docs/Tickets/SPIKE-033-semantic-recall.md) (Method 1 "Messages" and Method 2). All text is synthetic.

- `messages.yaml`: 6 chats, 300 messages, m001–m300 in chat order.
- `queries-messages.yaml`: 60 labelled queries, qm001–qm060.
- `check_messages_test.go`: checks both files (see below).

## The chats
Each chat is 50 messages between a user and the agent about project work. Code, commands, numbers, prices and URLs are mixed in. Some chats switch language partway, the way a user does when someone else reads along or a topic is easier in another language.

| Chat | Topic | en | de | fa |
|---|---|---|---|---|
| c1 | A Go tool that tracks used GPU prices: SQLite schema, cron, currency, Telegram alerts, seller filters. Switches to Persian. | 36 | | 14 |
| c2 | A trip to Lisbon and Porto in May: budget, hotel, train, visa, food, weather. In German, with an English part. | 12 | 38 | |
| c3 | A study plan for IELTS: exam date, study hours, weekly plan, books, writing time, flashcards. In Persian, with an English part. | 10 | | 40 |
| c4 | A small shop for soy candles: costs and prices in toman, wax ratio, Etsy fees, collection names, a broken order, product photos. | 18 | | 32 |
| c5 | Backups for a home server: restic, retention, a full SSD, Caddy, a backup time window, an alert script, Jellyfin's port. | 26 | 24 | |
| c6 | Moving to Leipzig: notice period, deposit, handover checklist, boxes, a van, a cat, Anmeldung, the internet contract. | | 23 | 27 |
| | **Total** | **102** | **85** | **113** |

The Persian is written the way people type in a chat, often colloquial (می‌خوام، می‌شه، خونه), with ZWNJ inside words (قیمت‌ها، اسباب‌کشی، صاحبخونه‌ی). Numbers are in both Persian and ASCII digits.

## The queries
Five per language and kind:

| Kind | en | de | fa |
|---|---|---|---|
| same_words | 5 | 5 | 5 |
| paraphrase | 5 | 5 | 5 |
| cross_language | 5 | 5 | 5 |
| no_answer | 5 | 5 | 5 |

They are written like an agent's `search_history` query: a few key words or a short question.
- **same_words:** every word of the query is in a target, so today's FTS finds it. Some distractors match every word too (qm021 also finds the flight message m068 and ranks it first; qm041 also finds the cost message m154), so FTS has to rank them.
- **paraphrase:** in the language of the target, with no word in common. A word counts as shared when one starts with the other, or both start with the same 5 letters, after the store's normalization and ignoring stop words.
- **cross_language:** the target is in another language. Every pair occurs: en → de and fa, de → en and fa, fa → en and de. A name may appear in both (Etsy, Vodafone), but FTS never finds the target.
- **no_answer:** nothing in the corpus answers it, and FTS finds nothing. Most are near a topic in the corpus (a bitcoin alert next to the GPU alerts, a passport renewal next to the visa messages, a dentist next to the paediatrician), so a vector search returns close but wrong messages.

A query has one or two targets. Two when the user's message and the agent's answer both answer it (the rule "keep 7 daily…" and its `restic forget` command). A hit on either counts. Some targets have queries of two kinds (m255 has a German paraphrase and an English cross-language query).

## Distractors
Each target sits among messages on a near topic, mostly in the same chat and a few turns away:
- **Same thing, other value:** "show prices in euros" (m025) after the prices in dollars (m005–m020); the small candle's price (m155) next to its cost (m154), its price in euros (m167) and the Yalda discount (m197).
- **Same words, other meaning:** the cron line every 30 minutes (m014) next to a retry every 5 minutes (m016), the 8:00 summary (m018) and DuckDNS every 5 minutes (m226); the train Lisbon–Porto (m064) next to the bus (m066) and the flight (m068).
- **Same request, other detail:** the Telegram alert for the 4070 (m033) next to email alerts (m030) and the 4060 with no alert (m035); the weekly report (m047) next to the daily report (m049) and the Sunday message (m048, m050).
- **Across chats:** "Mietwagen" for the trip (m074) against the van for the move (m273); "Kaution" for the rental car (m074) against the deposit (m259); alerts in c1 against the backup alert in c5; "weekly" plans in c1 and c3.

## The check
`check_messages_test.go` has the build tag `corpuscheck`, so the harness's `go test` doesn't run it. It needs the harness's module (the app's store, `go.yaml.in/yaml/v3` and `modernc.org/sqlite`). From the spike's folder:

```bash
go test -tags corpuscheck -v ./corpus
```

It checks that chat, message and query IDs are unique and in order, that roles, languages and kinds are valid, and that every target exists. For each query it builds an FTS5 table like `messages_fts`, with the store's `Normalize` and the same query (every word with a prefix star, joined with AND), and checks the promise of its kind, as listed above. It passed on the committed files.
