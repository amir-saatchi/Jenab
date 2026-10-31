# TASK-027 live: gemini-3.5-flash-lite, 1 rep

Run over two Gemini days, as Main-Session set: at most 150 requests on 2026-10-30, and under 120 on 2026-10-31.

- **2026-10-30:** the today condition. The 150-request cap stopped C8_para and C8_repeat.
- **2026-10-31:** both stopped runs again, then the cards condition. It started at 08:05 UTC, after the Pacific day reset, and used 119 requests.
- results/gemini-budget.json counts 150 + 119.
- There were no quota stops and no broken runs.
- Gemini rate-limited twice on 2026-10-31, so requests were spaced 4 s apart. One request in C1_ctrl got a 503 (overloaded) and was retried.
- results.md is the harness's raw table.

**The card pointer differs from gemma's and ministral's runs.** The cards runs used the build from af1ef9c. There, the card says "read_messages(chat_id: <this chat's id>, from: N)", where gemma and ministral got "read_messages from N". Gemini's cards runs made almost no history calls on repeats, so the pointer barely came into play.

With 1 rep, each row is one run per follow-up. One run changes a group by 13 to 17 points.

## By group, on the follow-ups that get a card

Follow-ups with no card are left out of this table. They are C3_para, C7_para, C3_ctrl, C6_ctrl and C8_ctrl, and they have no cards run. Their today runs: 5/5 done.

| group | carded follow-ups | condition | done | prompt tokens/run | requests/run | history calls/run |
|---|---|---|---|---|---|---|
| repeat | 8 | today | 7/8 | 46,988 | 9.1 | 1.9 |
| repeat | 8 | cards | 6/8 | 25,327 | 4.9 | 0.1 |
| paraphrase | 6 | today | 4/6 | 53,126 | 9.2 | 2.3 |
| paraphrase | 6 | cards | 4/6 | 48,185 | 8.0 | 2.0 |
| control | 5 | today | 5/5 | 10,888 | 2.4 | 0.0 |
| control | 5 | cards | 5/5 | 20,466 | 4.2 | 0.0 |

No check needed re-scoring: C4's two checks were fixed before these runs.

## Repeats

On cost, cards nearly halved prompt tokens (−46%) and requests (9.1 to 4.9). History calls dropped from 1.9 to 0.1 a run.

- **Gains.** C4_repeat went from failed to done. C1_repeat went from 13 requests to 5, and C7_repeat from 16 to 6.
- **Losses.** Two runs failed with the needed turn carded:
  - **C3_repeat** (card t5). The follow-up asks for the chart "for 2025 as well", but the expenses table holds only 2026. With the card, Gemini checked the dates and asked whether to save an empty 2025 chart. Today, it saved one (12 months of 0 spent).
  - **C6_repeat** (cards t3, t9). Gemini saved the Pounak table with t3's columns and rounding, so it used the card. But it left out t3's `area_m2 BETWEEN 80 AND 120`, after querying the listings, all four of which are 82 to 115 m². The table's rows are the same; the checks for 80 and 120 fail.

  Both are model choices with the right card in hand, not missing context. They are recorded as is.

## Paraphrases

Done is the same, 4/6 in both conditions, and C2_para and C8_para fail in both.

- C4_para and C5_para got cheaper: 14 to 5 requests, and 5 to 2.
- C1_para got dearer, 6 to 14 requests. Its card is t16, a wrong turn; the needed turn is t9. Gemini first read turns 12 to 19, around the card's turn. It then searched history, read turns 1 to 11 and a stored result, and saved the pipeline (and ran it once). The wrong card cost requests but didn't lead to a wrong answer.

## Controls (false cards)

All 5 carded controls stayed done, and no run acted on a carded turn.

- C2, C4 and C5 cost about the same as today.
- C7_ctrl took 3 requests instead of 2.
- **C1_ctrl** took 11 requests instead of 3. Gemini saved the same view. On the way, it doubted the query tool's two-decimal display of the ratio, and re-queried the prices 5 times. Its calls never touch the carded turns, t4 and t13. Whether the cards set this off can't be told from one run.

C1_ctrl is most of the +88% prompt tokens. Without it, carded controls cost +18%: 11,753 against 9,946 tokens a run. gemma's controls cost about 2% more.

## In short

On Gemini, 1 rep:

- Cards made repeats about half as costly, with nearly no history searches.
- Done did not improve: repeats were 6/8 against 7/8, with both losses being model choices with the right card. Paraphrases were 4/6 in both conditions.
- False cards changed no control's answer, but cost +18% (+88% with C1_ctrl's detour).

So, on tasks done, Gemini doesn't meet "cards beat today's window" in this rep. One rep of both conditions took 269 requests, so the 4 more reps the bar asks for would need about 1,080: about 9 Gemini days at under 120 a day.
