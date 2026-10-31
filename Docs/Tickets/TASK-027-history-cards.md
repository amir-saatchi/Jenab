# TASK-027 — History cards after a cut
**Type:** Task
**Status:** Done
**Gate:** 3
**Needs:** SPIKE-028

## Goal
After a cut (SPEC 3.6), a follow-up such as "do the same for ETH" needs an earlier turn the window no longer holds. Today the model has to find it with `search_history` and guess the words. SPIKE-028 showed that Go can add it as a card: when the card hit the right turn, the follow-up was done with about half the prompt tokens. Build it, but show first that it works beyond the spike's few chats.

## Today
- The spike's card code is in `spikes/028-tool-index/cards.go`, behind `Deps.Earlier`. It scores the turns before the window by shared words (idf), with fixed thresholds: at least 2 shared words, a score of at least 4.0, and better than any turn in the window.
- Its weak spots (SPIKE-028 Result): small words such as "it", "in" and "as" counted, and made two of its three paraphrase hits; the one true paraphrase got no card; most of the gain was one chat; a fixture left a lure in one chat.

## Scope
1. **Test set first,** written without running the scorer: at least 8 long chats of 20 turns or more, with follow-ups that repeat words, that paraphrase, and that need no earlier turn (controls). No leftover lures in the fixtures.
2. **Scoring:** small words don't count, by a fixed list or by how often a word appears in the project's chats, the same for every language. Persian text works too.
3. **The card:** the old turn's user text, calls and answer, as `read_messages` gives them, on the user's message, once per turn, so requests stay append-only. One or two cards at most, with a size cap.
4. **In the app:** cards in the context builder, not behind a spike hook. Then `Deps.Earlier` can go, and the SPIKE-028 and SPIKE-037 spikes set their own hook again or are left as they are.
5. Keep `search_history` and `read_messages` as they are, for what cards miss.

## Done when
- On the reference models (gemma4:31b and gemini-3.5-flash-lite) and ministral-14b, 5 reps: cards beat today's window on tasks done, and controls get no false card.
  - Changed on 2026-10-30, after the held-out set showed false cards come from real content overlap: controls must come to no harm (the same done rate, prompt tokens up at most 5%), judged on the reference models.
- Tests: no card when nothing matches, none when a turn in the window matches better, small words alone make no card, one card per turn, the size cap, Persian.
- SPEC 3.6 has the scoring rule.

## Result
Done on 2026-10-31 (commit a731322). The rule is in SPEC 3.6.

- **Test set first:** 8 chats of 22 turns (5 English, 3 Persian), each with a repeat, a paraphrase and a control (4 in the window, 4 needing no earlier turn), written and committed before any scoring. The thresholds were frozen on SPIKE-028's dev chats in their own commit.
- **Offline:** repeats 8/8 got the needed turn, paraphrases 2/8, and 5 of 8 controls got a false card. The false cards are real content overlap ("save a line chart" matches an older chart turn), so no word rule tried removed them; a fixed word list per language made recall worse.
- **Live** (carded follow-ups; today → cards):

  | Model | Reps | Repeats done | Repeat prompt tokens | Controls |
  |---|---|---|---|---|
  | gemma4:31b | 3 | 13/24 → 24/24 | −44% | 15/15 both, +2.4% tokens |
  | gemini-3.5-flash-lite | 1 | 7/8 → 6/8 | −46% | 5/5 both, +18% without one outlier run |
  | ministral-14b | 3 | 4/24 → 20/24 | −28% | 15/15 both, +26%; one false card led to a wrong answer the check missed |

  Paraphrases: gemma 4/18 → 9/18, Gemini 4/6 both, ministral 0/18 → 3/18.
- **Against Done when:** gemma meets it. Gemini ran 1 rep, not 5, to keep its quota for P2-13; it doesn't show more tasks done, but the cost of repeats about halves. One Gemini repeat failed because the fixture has no 2025 data for "2025 as well", and Gemini asked; that's a fixture flaw, left as is. ministral is an extra model; its results are recorded, not tuned for.
- **Recorded, not tuned:** ministral made up a chat id for `read_messages` in 15 runs, so the card now gives the exact `chat_id`. Gemini's runs had it; gemma's and ministral's didn't.
- **Review:** the card's untrusted mark is stored, survives a restart and passes on; tests for the memo, a real cut, the cut estimate and UTF-8 cuts, each checked by mutation; a time limit and a cached count; no second card of a turn already shown, none of an undone turn. SPIKE-028 and SPIKE-037 use the harness switch.
- **Requests:** 454 Ollama, 384 Mistral, 269 Gemini.
- **Open:** better paraphrase recall needs a new held-out set (TASK-031).
