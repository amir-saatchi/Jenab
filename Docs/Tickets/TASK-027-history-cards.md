# TASK-027 — History cards after a cut
**Type:** Task
**Status:** Open
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
- Tests: no card when nothing matches, none when a turn in the window matches better, small words alone make no card, one card per turn, the size cap, Persian.
- SPEC 3.6 has the scoring rule.

## Result
