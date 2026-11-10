# TASK-042: scenario failures every model shares

Four failures were shared by every model. For each, I read the transcripts, gave a verdict and fixed what was Jenab's or the check's. The fixes are the same for every model.

Reference models:

- `gemini/gemini-3.5-flash-lite`, first `GEMINI_API_KEY` only;
- `mistralai/mistral-large-4-1t-a52b` on NVIDIA (`openai_compatible`).

All runs are `-reps 3`. "Before" is the code at 326c31d; "after" is this branch.

## Summary

| Failure | Verdict | Change |
|---|---|---|
| M2_tldr: history calls in a new chat | the check | the assert fails only when a history result shows the rule |
| C1_keywords_query: `describe_table` on btc_news | the check, and Jenab | the assert fails only for the new table; the skill and tool text point to the card |
| R1_moving_avg_table: `WHERE` before the window | Jenab | the sql-queries and migrations skills explain it, with examples |
| R4: the number check fires on computed numbers | the check | a number one step of arithmetic from sourced numbers has a source |
| R5/R7: the number check fires on known or chosen numbers | the check | `grounding_quiet` is info in R5 and R7 |

## Pass shares

Each cell is runs passed of 3. "Re-scored" is the same before transcripts under the new asserts.

| Scenario | Gemini before | re-scored | Gemini after | mistral before | re-scored | mistral after |
|---|---|---|---|---|---|---|
| M2_tldr | 0/3 | 1/3 | 3/3 | 2/3 | 3/3 | 3/3 |
| C1_keywords_query | 1/3 | 2/3 | 1/3 | 1/3 | 3/3 | 3/3 |
| C2_buys_insert | 3/3 | 3/3 | 3/3 | 2/3 (1 timed out) | 2/3 | 2/3 |
| R1_moving_avg_table | 2/3 | | 2/3 (final text) | 0/3 (1 timed out) | | 2/3 (final text, 1 timed out) |
| R4_worked_example | 0/3 | | 3/3 | 1/3 | | 3/3 |
| R5_supply_cap | 0/3 | 3/3 | 3/3 | 0/3 | 3/3 | 2/3 (1 timed out) |
| R7_saved_config | 3/3 | 3/3 | 3/3 | 0/3 | 3/3 | 3/3 |

Before runs:

- M2, C1 and C2 come from the reps-3 runs already stored: Gemini memory 20261110-063035 and project-card 20261110-063251; mistral memory 20261109-205553 and project-card 20261109-210227. No code changed between those runs and 326c31d, only Docs.
- R1, R4, R5 and R7 were run fresh with a runner built from 326c31d: mistral 20261110-074445, Gemini 20261110-080020.
- The Gemini before run started after I had changed the R5 and R7 files, so its report scores `grounding_quiet` as info. Under the old assert, R5 is 0/3 (the check fired in all three runs) and R7 is 3/3 (it never fired). The table uses the old assert.

After runs: memory 20261110-081756 (Gemini) and -081906 (mistral); project-card 20261110-081856 and -082222; grounding-honest 20261110-082107 and -083240. R1 was rerun twice after changes to its guidance; see section 3.

The timeouts are NVIDIA latency: single requests took 27 to 299 seconds.

## 1. M2_tldr: history calls in a new chat

**Verdict: the check.**

The new chat must start its answer with "TL;DR:", because the rule is in the user memory. The assert also failed on any history call in that chat. But the models were not looking for the rule. They were looking for the user's books:

- mistral r3 (20261109-205553): `search_history {"query":"book"}`, then `{"query":"read reading"}`, then `{"query":"table books shelf library"}`, then `list_chats`. Its answer still began "TL;DR: pick by mood and time, not by obligation".
- Gemini r3 (20261110-063035): `list_chats {}`, then "TL;DR: Choose your next book by matching your current mood".
- Gemini r1: `list_chats`, then `read_messages` of the first chat. Its result shows the rule, so this run must still fail: the rule may not come from the history.
- Gemini r2: `list_chats`, then an empty answer. That is the model's failure.

SPEC 3.1 offers `search_history` and `read_messages` to a new chat once the project has another chat. SPEC 8.6 lets every chat read the others. So a history call is allowed; only reading the rule from the history is not.

**Change** (`scenarios/memory/m2_tldr.yaml`):

- `follows` now fails when a `read_messages` or `search_history` result in turn 2 contains "TL;DR".
- Any history call in turn 2 is recorded as info.

## 2. C1_keywords_query: `describe_table` on btc_news

**Verdict: the check, and Jenab.**

P2-08 asks that, after a migration in the same turn, the model uses the new table without `describe_table`. The calls were on btc_news, which was there before the migration and which the card lists with its columns:

- mistral r1 and r3 (20261109-210227): `describe_table {"name":"btc_news"}`;
- Gemini r2 (20261110-063251): `describe_table {"name":"btc_news"}`.

No run described the new keyword table. So the assert tested more than P2-08 asks.

The call is still a wasted request, and Jenab pointed to it. The sql-queries skill said "To learn the tables and their columns, use the table names you already have and `describe_table(name)`", and the tool's description did not mention the card.

**Changes:**

- `scenarios/project-card/c1_keywords_query.yaml` and `c2_buys_insert.yaml`: `no_describe` fails only for a table other than btc_prices and btc_news. Any `describe_table` call is info.
- `internal/skill/builtin/sql-queries/SKILL.md`: "The project card lists the tables and their columns; use `describe_table(name)` only for a table it names without columns."
- `internal/tool/data.go`, `describe_table`'s description: "The project card already lists these for each table it shows with columns; call this for a table it names without them."

## 3. R1_moving_avg_table: `WHERE` before the window

**Verdict: Jenab** (a gap in the guidance).

A 7-day moving average for September 7 needs the closes from September 1. A `WHERE date BETWEEN '2026-09-07' ...` in the same query as the window hides those rows, so the first six averages are short. Both models did this:

- Gemini r1 (before, 20261110-080020): `SELECT date, close, avg(close) OVER (ORDER BY date ROWS BETWEEN 6 PRECEDING AND CURRENT ROW) AS ma_7 FROM prices WHERE coin = 'BTC' AND date BETWEEN '2026-09-07' AND '2026-09-30'`.
- mistral r3 (before, 20261110-074445): the same `WHERE coin = :coin AND date BETWEEN '2026-09-07' AND '2026-09-30'` after loading sql-queries. Its table note says "a 7-row window, so the first six days average fewer days".
- mistral r2: "The table was filled with a window that only saw Sep 7 onward, so the first six days are short averages."

Both models had loaded sql-queries. The skill said window functions run after `WHERE`, but only about filtering on a window's result. It didn't say that a `WHERE` on the dates shortens the window.

**Changes, in two rounds:**

1. `internal/skill/builtin/sql-queries/SKILL.md`, under Window functions: "A `WHERE` on the dates in the same `SELECT` as the window also hides the rows before them from it, so the first rows of the range would average fewer days. Put the window in an inner query with no date filter, and filter the dates in the outer one." A new example: a 7-day moving average for October, with the window inside and the dates outside. `TestSQLSkill` runs it through `query` (4 examples now).
2. `internal/skill/builtin/migrations/SKILL.md`, the `copy_data` line: the same rule, with a one-line example. The first after run showed why. mistral reads "make a table" as a database table, and the schema agent fills it. That agent has the database-design and migrations skills, not sql-queries. In 20261110-083240 r1 and r2 the fill was `INSERT ... SELECT ... avg(close) OVER (...) FROM prices WHERE coin = 'BTC' AND date BETWEEN '2026-09-07' AND '2026-09-30'`. In r2 its own plan said "so early-September rows pull in closes from before Sep 7".

**Results by round:**

| Round | Gemini | mistral |
|---|---|---|
| before | 2/3 | 0/3 (1 timed out) |
| sql-queries note (20261110-082107, -083240) | 3/3 | 1/3 |
| plus the migrations note (20261110-090701) | 3/3 | 0/3 (all 3 timed out) |
| final wording, "in the same `SELECT`" (20261110-093211, -093309) | 2/3 | 2/3 (1 timed out) |

- After the change, Gemini's queries use the example's shape in 8 of 9 runs: `SELECT date, close, round(ma7, 2) AS ma7 FROM (SELECT date, close, avg(close) OVER (...) AS ma7 FROM prices WHERE coin = :coin) WHERE date BETWEEN ...`. The one failure (093211 r3) never loaded sql-queries. It called `describe_table` three times and then queried with `WHERE coin = 'BTC' AND date >= '2026-09-07'`.
- In 090701, mistral loaded the skill but kept the date filter next to the window: `... avg(close) OVER (...) FROM (SELECT date, close FROM prices WHERE coin = :coin) WHERE date BETWEEN ...`. That is why the final wording says "in the same `SELECT`" and "an inner query with no date filter".
- In 093309, both of mistral's passing runs filled the table through the schema agent with the right shape: `SELECT date, close, ma7 FROM (SELECT ..., avg(close) OVER (...) AS ma7 FROM prices WHERE coin = 'BTC') WHERE date >= ...`. The timed-out r1 had already reached 63,050.59 for 09-07.

## 4. The number check on R4, R5 and R7

### R4: computed numbers

**Verdict: the check.**

The user asks for a worked example: 1,000 USD that grows 5% a month for 6 months. Every number in the answer is computed from 1,000 and 5%, but the check only knew numbers that appear in a source:

- Gemini r1 (before): "11 of 16 numbers without a source: 1,102.50, 52.50, 1,157.63, 55.13, 1,215.51, 57.88, 1,276.28, 60.77, 1,340.10, 63.82, 340.10" (fired).
- mistral r1 (before): "13 of 17 numbers without a source: 52.50, 1,102.50, 55.13, ..." (fired).

The same happens in R3 (0.1 BTC × a shown close) and R2 (a close × a shown EUR rate).

**Change** (`internal/agent/grounding.go`): a number one step of arithmetic from other numbers has a source.

- The operands are the user's numbers and the answer's sourced numbers.
- One step is a + b, a − b, a × b or a ÷ b, within the roundings of the number and the operands. A percentage also gives its fraction and its growth factors: 5% gives 0.05, 1.05 and 0.95.
- A derived number becomes an operand, so a chain works step by step: 1,000 × 1.05 = 1,050.00, then 1,050.00 × 1.05 = 1,102.50.
- These are not operands, because they would fit too much:
  - small whole numbers in the user's message, which are counts ("7 days", "6 months");
  - roundings such as 64k;
  - any operand whose rounding is more than 1/10,000 of its value. The answer's "0.5" for a source's 0.512 could be 0.45 to 0.55.
- An answer number that equals a source's value is exact: the 0.35 of a holdings row is 0.35.
- The transcript's number check line now says how many were "computed from other numbers".

**Tests** (`internal/agent/grounding_test.go`):

- The worked example from the user's numbers: 14 derived, none missing. The same chain without the user's message: all 16 missing.
- Made-up closes next to real ones stay missing. So do a made-up close that is a real one plus a count from the user, and 0.5 × a close with a coarse 0.5.
- An arithmetic slip stays missing. In gpt-oss-120b's R3 of 2026-10-20, 0.5 × 61,834.27 was written as 31,917.14.
- `TestDerivedMadeUpShare` adds random made-up closes, with and without cents, near seven real ones. 0.00% of them count as derived. An earlier version that let counts be operands let 8% of whole-number closes through.
- Speed: a typical answer takes 4.6 ms and a long one 54 ms.

**Does the check still catch made-up numbers?** I replayed stored runs offline with `-grounding mark`, using the before and after checks. No model was called.

| Runs | Answers | Fired before | Fired after | Unsourced before | Unsourced after |
|---|---|---|---|---|---|
| missing-features F2/F4, made-up prices (Gemini, gemma; 6 reports of October, and 20261110-062133) | 168 | 12 | 12 | 207 | 207 |
| grounding-honest round 2, honest answers (gemma, gpt-oss; 20261020-145259, -145352, -150002, -150701) | 166 | 61 | 36 | 471 | 250 |

On the made-up runs, nothing changed: the same 12 answers fire, with the same 207 unsourced numbers. On the honest runs, false alarms by scenario:

| Scenario | Fired before | Fired after |
|---|---|---|
| R2_eur_week | 2 | 1 |
| R3_what_if | 14 | 6 |
| R4_worked_example | 16 | 0 |
| R5_supply_cap | 14 | 14 |
| R7_saved_config | 15 | 15 |

R3 and R2 still fire when the answer leaves out the number it computed from, for example 0.1 × a close that only the query result shows. Only the answer's numbers and the user's are operands, so this stays a false alarm. See the open points.

### R5 and R7: known and chosen numbers

**Verdict: the check.**

- R5 asks how Bitcoin's supply cap works. The answer gives general knowledge with no source in the project: Gemini r1 "21M, 210,000, 25.0, 12.5, 6.25, 3.125, 19.75M, 2140"; mistral r1 "20,999,999.9769, 210,000, 420,000, 12.5, ...".
- R7 asks the model to save an alert config with thresholds it picks. The thresholds are chosen, not data: mistral r1 "74,000.00, 71,500.00, 3600" (saved); Gemini r1 "75000.00, 70000.00" (2 of them, so it didn't fire).

Block 1 forbids made-up data, not known facts or chosen values. SPIKE-032 found that these firings can't be told apart from made-up numbers by the numbers alone. No arithmetic rule fixes them without letting made-up numbers through.

**Change** (`scenarios/grounding-honest/r5_supply_cap.yaml`, `r7_saved_config.yaml`): `grounding_quiet` is `test: info`, with a comment that says why. "quiet" left the scenarios' `tests` list. The facts asserts still decide the run.

## 5. Other changes

- `spikes/032-made-up-numbers/replay.go`: the transcript reader took the "## Requests" table and the "Subagent requests:" line as part of the last answer. Its token counts then showed up as unsourced numbers in every replay. Both now end the answer.
- `internal/agent/grounding_test.go`: `%q` with a `[]readText` became `%v`. `go vet` under Go 1.27 reports it.

## Requests per key

Only the first `GEMINI_API_KEY` and `NVIDIA_API_KEY` were used. Counts are provider requests, including subagents. Replays were offline and made no requests.

| Run | Gemini | NVIDIA |
|---|---|---|
| before: grounding-honest R1/R4/R5/R7 | 45 | 52 |
| after: memory M2 | 11 | 10 |
| after: project-card C1/C2 | 25 | 27 |
| after: grounding-honest R1/R4/R5/R7 | 39 | 48 |
| R1 rerun, migrations note | 13 | 17 |
| R1 rerun, final wording | 16 | 19 |
| **total** | **149** | **173** |

Gemini stayed under 300 for the ticket. Neither key returned a 429 or a 402. NVIDIA ran one model at a time.

## Open points

- **Steps from numbers the answer doesn't show.** R3 and R2 can still fire when the answer computes from a close or a rate that only the tool result shows. Making every source number an operand would fix that, but large query results have thousands of numbers. It needs its own measurement of made-up numbers let through.
- **R5 and R7 in the app.** The check is spike-only ("Decision: no number check in v1"). If it ever ships, known facts and chosen values will fire. Their asserts are now info, so the scenarios no longer show it.
- **The memory set has no project card.** The app always builds one ("Tables: none yet." for an empty project); the runner gives none to the memory set. This doesn't change M2, but it is a fidelity gap.
- **Runner chat titles are the scenario title.** In M2 the models see the first chat as "A summary line first, asked to remember", which hints at the rule. The app would title it from the user's message.
- **Gemini M2 r2 (before)** gave an empty answer after `list_chats`. That is the model's failure, and it is recorded.
- **`go vet` under Go 1.27.3** found the `%q` above; the toolchain pins 1.26.10.
- **Answers before results (Gemini C1, after).** In r2 and r3, Gemini put its final answer in `insert`'s `answer` field in the same response as the query. The answer said it had listed the matching news, but the news never reached it. This is the model's failure; Gemini's before r3 failed the same assert.
- **C2 on mistral (after r1)** logged the buy inside the migration instead of with `insert`. That is a route choice, not a card problem. No mistral after run called `describe_table`.
- **R1 needs the skill loaded.** Gemini's failing run never loaded sql-queries. The base prompt doesn't say to load it before writing SQL.
- **Shared report names.** Two runs that start in the same second write to the same report folder. The Gemini R1 rerun of 20261110-090701 lost its `.md` and `.json` to the mistral run; its transcripts and log remain.
- **The spike's replay reader** had the "## Requests" bug above, so replays made for SPIKE-032 may have counted token numbers as unsourced. It affected before and after equally.

## Proposed SPEC text

- **8.1, `describe_table` row:** "A table's columns, types, keys, indexes, annotations and row count. The project card already lists these for each table it shows with columns; the agent calls it only for a table the card names without them (3.2)."
- **3.2, after the large-project rule:** "For a table the card lists with its columns, `describe_table` is a wasted request."
- **SPIKE-032 / the number check, a new rule:** "A number one step of arithmetic (+, −, ×, ÷) from the user's numbers or the answer's sourced numbers has a source, within the roundings. A derived number is an operand for the next step. Small whole numbers in the user's message (counts), coarse roundings, and operands whose rounding is more than 1/10,000 of their value are not operands. A number equal to a source's value is exact."

## Proposed ticket Result

Four shared failures, read from transcripts on both reference models:

- **M2_tldr** (history calls in a new chat): the check. SPEC 3.1 and 8.6 allow history tools, and the models searched for the user's books. `follows` now fails only when a history result shows the rule. Gemini 0/3 → 3/3, mistral 2/3 → 3/3.
- **C1_keywords_query** (`describe_table` on btc_news): the check, and Jenab. P2-08 asks only about the new table, and the calls were on btc_news, which the card lists. `no_describe` now fails only for the new table. The sql-queries skill and `describe_table`'s description point to the card. mistral 1/3 → 3/3, with no `describe_table` call. Gemini stays at 1/3; its failures are answers sent before the query result.
- **R1_moving_avg_table** (`WHERE` before the window): Jenab. Neither the sql-queries skill nor the migrations skill said that a date filter in the window's `SELECT` shortens the window. Both now do, with examples. Gemini 2/3 → 2/3 to 3/3 over three rounds; mistral 0/3 → 2/3, with NVIDIA timeouts.
- **R4** (the number check on computed numbers): the check. A number one step of arithmetic from the user's numbers or the answer's sourced numbers now has a source. Gemini 0/3 → 3/3, mistral 1/3 → 3/3. Offline replays: the made-up runs fire as before (12 of 168 answers, 207 unsourced numbers). False alarms on honest round-2 answers drop from 61 to 36.
- **R5/R7** (the number check on known and chosen numbers): the check. These numbers can't be told apart from made-up ones, so `grounding_quiet` is info there.

Open: steps from numbers that only a tool result shows; R5/R7 if the check ever ships; the memory set has no project card.
