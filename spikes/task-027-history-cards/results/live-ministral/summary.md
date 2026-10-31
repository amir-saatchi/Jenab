# TASK-027 live: ministral-14b-latest, 3 reps, thresholds frozen

Run on 2026-10-30 on Mistral (openai_compatible), 384 requests in all. Mistral rate-limited once, so the harness spaced requests 2s apart. There was no quota, no stopped run and no broken run. results.md is the harness's raw table. The C4 checks were already fixed for JSON-escaped `>`, so nothing was re-scored.

## By group, on the follow-ups that get a card

Five follow-ups get no card: C3_para, C7_para, C3_ctrl, C6_ctrl and C8_ctrl. They are left out of this table and have no cards run. In the today condition, the paraphrases among them were done in 0/6 runs and the controls in 9/9.

| group | carded follow-ups | condition | done | prompt tokens/run | requests/run | history calls/run |
|---|---|---|---|---|---|---|
| repeat | 8 | today | 4/24 | 37,109 | 5.1 | 0.4 |
| repeat | 8 | cards | 20/24 | 26,810 | 3.7 | 0.8 |
| paraphrase | 6 | today | 0/18 | 7,066 | 1.4 | 0.0 |
| paraphrase | 6 | cards | 3/18 | 14,464 | 2.7 | 0.6 |
| control | 5 | today | 15/15 | 11,856 | 2.2 | 0.0 |
| control | 5 | cards | 15/15 | 14,987 | 2.6 | 0.3 |

Repeats went from 0/3 to 3/3 in C1, C3, C4, C5 and C8, and from 1/3 to 3/3 in C2. Two repeats did not improve:

- C6_repeat was 0/3 in both conditions. With the card, ministral saved the Punak table each time but dropped the 80 to 120 m2 range.
- C7_repeat went from 3/3 to 2/3. In one cards run, ministral read turn 7 and then described the change in prose instead of saving the view.

## Controls: "no harm" is not met on ministral

The bar was that the controls' done rate stays the same and prompt tokens rise by at most 5%.

- **Done rate:** unchanged at 15/15.
- **Prompt tokens:** up 26% on the carded controls (11,856 to 14,987 per run). Across all 8 controls, counting the no-card ones at today's numbers, the rise is 17.6%.

The cost comes from two sources:

- **The card's own bytes.** Runs with no extra call grew by 2 to 9%: C4 by 2%, C5 by 5%, C1 by 7% and C2 by 9%. On gemma the same cards added 2.4%.
- **False cards that drew a history call.**
  - C7_ctrl's false card (t10) led to `search_history "کتاب"` in all 3 runs, adding 74% in tokens.
  - In one C4_ctrl run, the false card (t8) led to read_messages and list_chats: 5 requests instead of 2, and 170% more tokens.

**One answer was wrong, and the check missed it.** In C7_ctrl cards r1, ministral searched history after the false card and then filtered `WHERE rating IS NOT NULL`. It answered that Dowlatabadi has 1 book, then that Sepehri has 6 books, which is not in the data. Today's three runs, and the other two cards runs, all gave the right answer: Dowlatabadi, 3 books. The check looks only at the query call, so it passed.

## Did a wrong card change the answer?

Turn 23 was read in all 3 reps, with and without cards.

- **C1_para** (the card is t16, the needed turn is t9). Today, ministral talks about price alerts (0/3). With the card, it read t16 in every run and offered the ETH gap pipeline (eth_gap_check) as the answer for Bitcoin (0/3). The wrong card pulled the answer to the wrong turn. On gemma, the same card led to a history search and the right turn.
- **C8_para** (the card is t19, the needed turn is t10). Today, the query covers one day. With the card, every run queries 6 to 19 October instead. It fails in both conditions, but the card changed the query.
- **C6_para** (the cards are t5 and t18, the needed turn is t9). With the cards, ministral read t18 or t5, and two runs then said this chat was about rent, not the deposit. It was 0/3 in both conditions.
- **C4_para** (the card is t6). ministral offered a race digest in both conditions and never used the card (0/3 each).
- **C2_para** (the needed t8 is carded, plus t12). The card changed the query to "interviewed and waiting 14 days or more", but the run still failed the checks (0/3 in both conditions).
- **C5_para** (the needed t11 is carded, plus t17). It went from 0/3 to 3/3: heat_guard was saved in every run.

## A tool slip, for the record

ministral passes `chat_id` to read_messages even though it defaults to this chat. It used "this_chat", "this chat", "current" and "mother". The call fails, and ministral recovers with list_chats and a second read_messages. This happened in 15 cards runs and in no today run, because today ministral found the turn with search_history first, which gives the chat's id. Each slip costs 1 or 2 extra requests. The card says `read_messages(from: N)` with no chat id. It is the model's failure; nothing is tuned for it.

In short, on ministral, right cards made repeats work: done went from 17% to 83%, with 28% fewer prompt tokens. Wrong cards on paraphrases pulled the answer toward the carded turn more than on gemma, though they turned no passing run into a fail. False cards on controls cost 26% in prompt tokens, and one led to a wrong answer the check missed.
