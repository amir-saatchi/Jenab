# TASK-043: failures both reference models share

TASK-042's suite left four failures that both reference models share, plus two extra points. For each, I read the transcripts, gave a verdict and fixed what was Jenab's or the check's. The fixes are the same for every model.

Reference models:

- `gemini/gemini-3.5-flash-lite`, first `GEMINI_API_KEY` only;
- `mistralai/mistral-large-4-1t-a52b` on NVIDIA (`openai_compatible`).

"Before" is the code at e1b5286, run from an unchanged copy of the worktree. "After" is this branch.

**A count I got wrong.** At first I thought the Gemini key had passed its daily cap. I counted the suite's 215 Gemini requests against it, and stopped the Gemini runs at 22:43 UTC. But the suite ran on another key (`GEMINI_API_KEY_III`). The first key's day since 08:00 UTC on 11-10 was TASK-042's 136 plus this ticket's runs, under 400. Afterwards the Gemini acceptance run was done (55 requests). The Gemini schema runs with the last migrations line were left for a new day, to keep a margin under the key's cap.

## Summary

| Failure | Verdict | Change |
|---|---|---|
| role-skills: Mother never gives migrations, pipelines or config-guide | the set | the set gets the write tools those skills load with; `check` rejects a skill assert that the chat's skill list can't meet |
| acceptance turn 25: right without `search_history` | the check | a new `from_history` assert: a history card or a history tool brought turn 3 back |
| schema-widen: the news key stays `url`, `prices` loses its CHECK, mistral's `DEFAULT 'BTC'` | Jenab | the main agent no longer designs the change; the schema agent sees each column's CHECK and is told the design is its own |
| local-refresh: YYYY-MM-DD never reaches memory | the check | a new `memory_contains` assert reads user and project memory |
| refresh-memory R1 on mistral: "the answer has no JSON object" | Jenab, and the model | a cut-off answer is named as cut off, and its retry gets more room |
| sql-queries is not loaded before SQL | the model | left open, with a proposal |

## Pass shares

Each cell is runs passed. The schema runs on mistral are slow (single NVIDIA requests took up to 9 minutes).

| Scenario | Gemini before | Gemini after | mistral before | mistral after |
|---|---|---|---|---|
| role-skills R1_redesign_tables | 0/3 | 3/3 | 0/3 | 3/3 |
| role-skills R2_dashboards | 0/3 | 3/3 | 0/3 | 0/3 |
| role-skills R3_gold_news_daily | 0/3 | 0/3 | 0/3 | 0/3 |
| role-skills R4_sql_questions | 3/3 | 3/3 | 2/3 | 3/3 |
| role-skills R5_pipeline_care | 0/3 | 2/3 | 0/3 | 0/3 |
| role-skills R6_exchange_trades | 0/3 | 0/3 | 0/3 | 1/3 (1 timed out) |
| **role-skills** | **3/18** | **11/18** | **2/18** | **7/18** |
| schema-widen-check T4 | 2/3 | 2/3 | 0/3 | 2/3 |
| schema-widen T4 | 0/1 | 0/1 | 1/3 | 3/3 |
| acceptance P1 | 0/1 (suite) | 1/1 | 0/2 (suite) | 1/1 |
| refresh-memory R1 | | | 0/1 | 1/1 |
| local-refresh L4, date format (info) | 2/2 re-scored | 3/3 | 3/3 re-scored | 3/3 |

Runs:

- Before: Gemini 20261110-204441 (role-skills), -204900 (swc), -205118 (sw); mistral -205254, -212540, -213608, refresh R1 -215832 and the diagnostic -221344. Acceptance and local-refresh "before" are the suite's runs of 11-10 (Gemini acceptance -102824; mistral -100751 and -160443), re-scored where the transcript allows it. TASK-042 changed nothing these two sets use.
- After: Gemini 20261110-223607, -223937, -224157, -224223 (local-refresh), acceptance 20261111-005208; mistral -224530, -232022, -233550, refresh R1 -235143, local-refresh 20261111-001312, acceptance 20261111-003757.
- Gemini's swc and sw after runs came before the last `add_column` line in the migrations skill (section 3). mistral's came after it. Gemini's acceptance ran on the final commit.
- Gemini sw ran with -reps 1, as the budget planned.

## 1. role-skills: Mother never gives migrations, pipelines or config-guide

**Verdict: the set.**

Mother picks a new chat's skills from her own skill list (SPEC 8.9). `Usable` leaves out a skill whose `load_with` tools are all missing. The role-skills set fakes only the read tools, so:

- the set's migrations skill (`load_with: [apply_migration]`) was hidden;
- the set's pipelines skill (`load_with: [save_pipeline]`) was hidden;
- the built-in config-guide (`load_with: [save_view, save_page]`) was hidden.

No model could pass R1, R2, R3, R5 or R6, because the skill they need was never offered. Before, every run that missed a skill missed exactly these:

- Gemini R1 r1 (20261110-204441): "skills [database-design sql-queries]: missing [migrations]";
- Gemini R2 r1: "skills [sql-queries]: missing [config-guide]";
- Gemini R3 r1: "skills [web-research]: missing [pipelines]";
- mistral R6 r1 (20261110-205254): "skills [database-design sql-queries]: missing [migrations pipelines config-guide]".

**Changes:**

- `scenarios/role-skills/_set.yaml`: fake `apply_migration`, `save_pipeline` and `save_view`, with the real tools' areas, and rules that answer "applied", "saved". A comment says why.
- `internal/scenario` (`Set.usableSkills`, `Set.checkSkills`): `check` now fails a `created_skills`, `skill_loaded` or `skill_loaded_before` assert whose skill the chat's skill list can't hold. The skill list is built as the run builds it: the built-ins, the set's skills, then `Usable` with the real and faked tools. The old set fails `check` with "R1_redesign_tables: asserts[1] (created_skills): the skill list of its chat has no migrations; give the set a tool of its load_with, or a skill of that name".
- `RunSet` and `check` share one function for the set's skill list (`Set.skillSet`).

After, the remaining failures are the model's choices:

- Gemini R3 r1–r3 (20261110-223607): "skills [web-research]: missing [pipelines]". pipelines is now in the list; the model gives only web-research for a daily news job.
- Gemini R6 r1: "skills [database-design pipelines config-guide]: missing [migrations]".
- Gemini R5 r2: config-guide where it doesn't fit.
- mistral R2 r1–r3 (20261110-224530): "skills [config-guide sql-queries database-design]: missing [], not fitting [database-design]". config-guide is now given; database-design is extra.
- mistral R5 r1–r3: pipelines is given, plus migrations, which doesn't fit.
- mistral R3: web-research only, like Gemini.
- mistral R6 r1 timed out at 10 minutes; r2 missed migrations.

R3 is shared: both models give a daily job to a chat with web-research only. The pipelines skill's description names "triggers and schedules". See the open points.

## 2. acceptance: turn 25 is right without `search_history`

**Verdict: the check.**

At turn 25 the user asks about the purchase from turn 3. Turn 3 is out of the history window, so the assert asked for a `search_history` call. But SPEC 3.6 adds history cards: after a cut, Go puts short cards of earlier turns on the turn's first user message. The card for turn 3 carries the answer.

- The scripted run (`TestAcceptanceRestart`) shows the card at turn 25: "[Turn 3 of this chat, before the recent turns. … I bought my first 0.35 BTC on 12 March 2021 for 48,200 euros, on the exchange Bitstamp…]".
- Real runs answered with no tool call: Gemini 20261110-102824 "You mentioned that you bought your first **0.35 BTC** for **48,200 euros** on the exchange **Bitstamp**"; mistral 20261110-160443 the same. Turn 25's own messages were 173 and 251 tokens, so the facts came from the card, not from the turn.

So the answer is right, and right in the way SPEC 3.6 wants. The assert asked for one route out of two.

**Changes:**

- `internal/scenario/record.go`: for each turn, the record reads the first request from `Traces.Turn` (`List` drops request texts). It keeps the history cards on the turn's user message, and the text of the turns before it without cards. The transcript shows the cards as "> history cards:" under the user's message.
- `internal/agent/cards.go`: `IsHistoryCards(text)` tells a card part from the user's text, so the card format stays in one place.
- A new assert `from_history` (match, turn). It passes when a history card, or a `search_history` or `read_messages` result in the turn, matches. It fails when the window already holds the match, since then the turn tests nothing about history. It is skipped when no request of the turn was recorded.
- `scenarios/acceptance/p1_restart_history.yaml`: `from_history` on 48,200 is the history test; the `search_history` call is info.
- Tests: `TestSplitCards`, `TestFromHistory`, `TestIsHistoryCards`; `TestAcceptanceRestart` passes with the new assert.

This doesn't loosen the test: an answer that guesses still fails `answer_contains`, and a turn whose window already holds turn 3 now fails.

After, mistral (20261111-003757): pass. `from_history`: "a history card brought back 48,200"; no `search_history` call (info). The transcript shows the card under turn 25: "[Turn 3 of this chat, before the recent turns. … User: Some background: I bought my first 0.35 BTC on 12 March 2021 for 48,200 euros, on the exchange Bitstamp. …]". After, Gemini (20261111-005208): pass, 55 requests. `from_history`: "a history card brought back 48,200"; no `search_history` call (info). The answer: "You bought your first **0.35 BTC** on **12 March 2021** on **Bitstamp** at a price of **48,200 EUR/BTC**". One request got a 503 "overloaded" and the retry went through; it was not a quota error.

The suite's runs can't be re-scored with `from_history`: their transcripts don't hold the requests. Their turn-25 answers were right with no tool call, as above.

## 3. schema-widen: the news key, the lost CHECK and the default

**Verdict: Jenab.**

The user says: "Move the prices into one general table prices(coin, date, price), keeping every BTC row … give each news item a coin". SPEC 8.2 and the database-design skill want `btc_news` widened to the key `(coin, url)`, and every old column and check kept.

The main agent designed the migration itself and handed the design to the schema agent as the goal. The schema agent followed it:

- Gemini swc r1 (before, 20261110-204900): "Create a general table `prices (coin TEXT NOT NULL, date TEXT NOT NULL, price REAL, _run_id TEXT, _fetched_at TEXT)` with primary key `(coin, date)` … Also, add a `coin` column to `btc_news`". The steps: `widen_table` and `add_column`. The news key stayed `url`.
- mistral swc r1 (before, 20261110-212540): "1) Create a new table prices(coin TEXT NOT NULL, date TEXT NOT NULL, price REAL, …) … 3) Add a col…". The steps: two `create_table`, two `add_column`. `price REAL` has no CHECK, so `prices` lost `CHECK (price >= 0)`.
- Gemini sw r1 (before, 20261110-205118): "Create a new table prices with columns coin TEXT NOT NULL, date TEXT NOT NULL, price REAL … Add a coin TEXT column to btc_news". `prices` lost `_run_id` and `_fetched_at`: "no such column: _run_id".
- mistral sw r2 (before, 20261110-213608): `DEFAULT 'BTC'` on the news coin, so new rows are labelled BTC.

The runs that passed loaded database-design and asked for a widen with `(coin, url)`.

Two Jenab gaps made this easy:

- `request_schema_change`'s description asked for "the goal in plain words" but didn't say not to design. The schema agent's prompt said nothing about a goal that brings its own design.
- `describe_table`, and so the schema agent's table list, didn't show CHECKs: `price REAL CHECK (price >= 0)` showed as "- price REAL". A model that rebuilds a table from that list can't keep a check it can't see.

**Changes:**

- `internal/store/tables.go`: `Column.Check`, read from the table's CREATE statement with the parser the migrations already use. A table CHECK leaves it empty.
- `internal/tool/data.go`: `Describe` prints " CHECK (…)" after the column. `TestDescribeTable` already had `price > 0`, which the old output hid.
- `internal/agent/schema.go`, `request_schema_change`: "Don't design it: leave out keys, types, defaults and steps the user didn't ask for."
- `internal/agent/prompts/schema.md`: a widened table keeps its columns "with their types, NOT NULL, defaults and checks". New rules: "The goal tells you what the user wants; the design is yours. … When a new column tells the rows apart for the wider goal, widen the table with `widen_table` … even if the goal says to add a column or keep the old key." And: "Give no column a default that labels the old rows".
- `internal/skill/builtin/migrations/SKILL.md`: the `create_table` route keeps "every old column, with its type, NOT NULL, default and check". The `add_column` row: "Not for a column that tells the rows apart, such as a coin for news tracked per coin: that is `widen_table`". I added that second line after the Gemini after run below.
- Tests: `TestDescribeChecks` (a check survives a widen; a table CHECK leaves the columns' empty).

After, on Gemini (20261110-223937, -224157, before the `add_column` line):

- the prices part passes in every run: `widen_table` on `btc_prices`, with its columns and check;
- the main agent still writes "add a coin column to btc_news", which is the user's "give each news item a coin". In swc r3 and sw r1 the schema agent made that an `add_column`, so the news key stayed `url`. In sw r1 it also gave the column `"default":"'BTC'"`, against its new rule.

After, on mistral (20261110-232022, -233550, with all the changes):

- swc r1: the goal still said "Add a coin column to btc_news", but the schema agent widened it: "widen btc_news the same way with coin 'BTC' while keeping its name and url key widened to (coin, url)".
- swc r2 and r3 built `prices` with `create_table`, and now with `"check":"price >= 0"`. r2 widened the news; r3 added a column, so its key stayed `url`.
- sw r1–r3 passed. No default labelled old rows.

So the CHECK and the lost columns are fixed on both models. The news key still depends on the model reading "give each news item a coin" as a key change; the guidance now says so in three places. A check of Gemini with the last line needs another day's budget.

## 4. local-refresh: YYYY-MM-DD never reaches memory

**Verdict: the check.**

The user says "For all my chats, write dates as YYYY-MM-DD." That is a user-wide convention, and SPEC 3.5 puts it in user memory. The info assert read only project memory (`_jenab_memory`). In every finished suite run the format was in user memory: Gemini 20261110-115206 and -115241, mistral -112402, -165037 and -194739 ("## User memory at the end": "conventions: Write dates as YYYY-MM-DD"). mistral -113629 timed out.

**Change:** a new assert `memory_contains` (match) reads user and project memory at the end; the record keeps project memory for it. `l4_refresh_short.yaml` uses it, still as info. Test: `TestMemoryContains`.

Re-scored, the suite runs pass 5 of 5. After: Gemini 3/3 (20261110-224223: "user memory: conventions: Write dates as YYYY-MM-DD."). mistral 3/3 (20261111-001312).

## 5. refresh-memory R1 on mistral: "the answer has no JSON object"

**Verdict: Jenab, and the model.**

JSON mode is sent and honoured: the openai provider sends `response_format: json_schema`, and every answer with text was a clean JSON object.

The failing answers had no text at all. I added the stop reason, output tokens and thinking tokens to the runner's `-schema-log` (it had only the text) and ran R1 again (20261110-221344). The map calls:

| answer | stop | output | of it thinking | text |
|---|---|---|---|---|
| valid | end | 661 | 574 | 285 chars |
| valid | end | 2,567 | 2,466 | 341 chars |
| valid | end | 2,303 | 2,248 | 195 chars |
| valid | end | 1,892 | 1,722 | 636 chars |
| empty | max_tokens | 4,096 | 4,095 | none |
| retry, valid | end | 4,019 | 3,793 | 824 chars |
| empty | max_tokens | 4,096 | 4,095 | none |
| retry, empty | max_tokens | 4,096 | 4,095 | none |

mistral thinks on NVIDIA although the request asks for no thinking. `openai_compatible` has no standard way to turn thinking off, so Go sends nothing for it. A long think uses the whole 4,096-token cap of a map call, the answer is empty, and Go reported "the answer has no JSON object". The retry had the same cap and the same fate.

The model's part: it thinks for 4,000 tokens to sort a few chat lines. Jenab's part: Go ignored the stop reason, so its retry message and its error were wrong, and the retry had no more room than the first try.

**Change** (`internal/agent/refresh.go`):

- `call` reports whether the answer stopped at the cap.
- A cut-off answer's retry says "the answer reached the output limit of 4096 tokens before it ended; think briefly", and gets the merge's cap, 8,192. Every refresh already sends that cap to the same model, so no model-specific limit is needed.
- A retry cut off again fails with "the answer was cut off at the output limit of 8192 tokens", not "no JSON object".
- Test: `TestRefreshCutOff`.

After, mistral R1 (20261110-235143): pass. In the schema log, one map call stopped at max_tokens (4,096, of it 4,095 thinking). Its retry, with the 8,192 cap, used 6,165 tokens and gave a valid answer. The merge used 6,406 tokens and was valid. The memory then had the goal, decisions and the Monday report day.

The model still thinks where it was asked not to. That is recorded, not worked around: the fix gives any model's cut-off answer the same second chance.

## 6. sql-queries is not loaded before SQL

**Verdict: the model** (left open).

I counted the suite's 36 result folders of 11-10 (read only, no requests). In a transcript with a `query` call, was sql-queries loaded before the first one?

| Model | Transcripts with a query | sql-queries loaded first | `query` calls without it | of them errors |
|---|---|---|---|---|
| Gemini | 16 | 0 | 55 | 3 |
| mistral | 33 | 8 | 80 | 0 |

The skill list already says "Before you do that kind of work, load its skill" (SPIKE-025's wording). The three Gemini errors were all ones the skill covers, and each error message led to a fix in the next call:

- acceptance 20261110-102824: "missing named argument "start_date"";
- grounding 20261110-141651: "only one read-only SELECT is allowed";
- schema-widen 20261110-123916: "`sqlite_master` is reserved".

So the cost is small: 3 errors in 135 calls. I didn't change it. A fix would change every chat that queries, and measuring it needs runs across many sets. See the open points.

## Other changes

- `cmd/jenab-scenarios`: `-schema-log` lines now have `stop`, `output`, `thought` and `thinking_chars`, and their time is UTC.

## Requests per key

Only the first `GEMINI_API_KEY` and `NVIDIA_API_KEY` were used. Counts are provider requests, including subagents and refresh calls. The sql-queries count and the re-scoring were offline.

| Run | Gemini | NVIDIA |
|---|---|---|
| before: role-skills | 41 | 47 |
| before: schema-widen-check | 22 | 19 |
| before: schema-widen | 6 | 20 |
| before: refresh-memory R1 | | 31 |
| diagnostic: refresh-memory R1 with the new schema log | | 20 |
| after: role-skills | 41 | 50 (7 in the timed-out run) |
| after: schema-widen-check | 23 | 14 |
| after: schema-widen | 7 | 13 |
| after: refresh-memory R1 | | 15 |
| after: local-refresh | 10 | 10 |
| after: acceptance, stopped at 22:43 | about 15 | |
| after: acceptance | 55 | 52 |
| a Gemini start I stopped, see below | about 2 | |
| **total** | **about 222** | **291** |

- The Gemini numbers with "about" are estimates: the runner writes no counts for a run that is stopped.
- One Gemini start overlapped the NVIDIA diagnostic. I stopped it after about 2 requests, to keep one model at a time.
- The ticket stayed under 300 on Gemini and under 400 on NVIDIA. The first Gemini key's day since 08:00 UTC on 11-10 is about 358 (TASK-042's 136 plus this ticket's), under 400. Neither key returned a 429 or a 402.

## Open points

- **Gemini swc -reps 3 and sw with the last add_column line: not run (budget), for a new day.**
- **The news key on Gemini.** After the changes, Gemini still made `add_column` on btc_news in 1 of 3 swc runs and in sw r1, once with a default that labels the rows. The `add_column` row now says a column that tells rows apart is a widen; that line was measured only on mistral.
- **R3: a daily job without pipelines.** Both models give the gold-news chat web-research only, 0/3 each, before and after. Only pipelines run on a schedule (SPEC 7). The delegation skill could say: "Work that repeats on a schedule needs pipelines." I didn't add it, since I couldn't measure it on Gemini.
- **Extra skills on mistral.** R2 and R5 now fail only because mistral adds database-design or migrations. That is the model's choice: each assert names the skills that fit the chat, and these don't.
- **Thinking on `openai_compatible`.** mistral thinks on NVIDIA even with `Thinking: false`. There is no standard field to turn it off, so Go sends none, by design (no model-specific setup). The refresh now copes with a cut-off answer, but the thinking still costs time: single requests took up to 9 minutes.
- **sql-queries before SQL.** A proposal: `query`'s description could say "Load the sql-queries skill before writing anything beyond a simple SELECT". It would change every chat that queries, so it needs its own measurement across sets.
- **A table CHECK.** `describe_table` reads column CHECKs from the table's DDL. A table-level CHECK isn't shown, and the column checks of such a table aren't either (the test covers it).
- **Runner request counts for stopped runs.** A run stopped from outside writes no report, so its requests aren't counted.

## Proposed SPEC text

- **8.1, `describe_table` row:** "… a table's columns, with their types, NOT NULL, defaults and CHECKs, keys, indexes, annotations and row count."
- **8.2, the main agent's goal:** "The goal says what the user wants, with the names they gave and the data to keep or move. The main agent doesn't design the change: it leaves out keys, types, defaults and steps the user didn't ask for. The schema agent designs it; where it designs differently from the goal, its plan says so."
- **8.2, widen:** "A column that tells the rows apart for the wider goal is added with `widen_table`, never `add_column`, so it joins the key. No default labels the old rows."
- **3.5, the map and merge calls:** "An answer cut off at the output cap, with no usable JSON, is retried once with the merge's cap (8,192 tokens), and the retry says it was cut off. Cut off again, the refresh fails with "cut off at the output limit"."
- **Scenario runner (testing section):** "`from_history` passes when a history card or a `search_history`/`read_messages` result in the turn holds the match, and fails when the turn's window already holds it. `memory_contains` reads user and project memory at the end. `check` fails a skill assert whose skill the chat's skill list can't hold."

## Proposed ticket Result

Four shared failures and two extra points, read from transcripts on both reference models:

- **role-skills** (Mother never gives migrations, pipelines or config-guide): the set. It faked no write tool, so `Usable` hid the skills that load with them. The set now fakes `apply_migration`, `save_pipeline` and `save_view`, and `check` rejects a skill assert the skill list can't meet. Gemini 3/18 → 11/18, mistral 2/18 → 7/18. Still failing: R3 on both (no pipelines for a daily job) and extra skills on mistral.
- **acceptance** (turn 25 right without `search_history`): the check. The SPEC 3.6 history card carries turn 3. A new `from_history` assert accepts a card or a history tool, and fails if the window already holds the fact. Gemini 0/1 → 1/1, mistral 0/2 → 1/1.
- **schema-widen** (news key, lost CHECK, `DEFAULT 'BTC'`): Jenab. The main agent designed the migration in the goal, and `describe_table` hid CHECKs. `request_schema_change` now says not to design; the schema agent's prompt and the migrations skill say the design is its own, how to widen, and to keep checks; `describe_table` shows CHECKs. mistral swc 0/3 → 2/3 and sw 1/3 → 3/3; Gemini 2/3 → 2/3 and 0/1 → 0/1, with only the news key left.
- **local-refresh** (date format): the check. "For all my chats" goes to user memory. A new `memory_contains` assert reads both memories: 5/5 re-scored, 3/3 after on each model.
- **refresh-memory R1 on mistral** ("no JSON object"): Jenab, and the model. JSON mode is sent and honoured; mistral thinks on NVIDIA and used the whole 4,096-token cap. Go now names a cut-off, and the retry gets the merge's 8,192-token cap. 0/1 → 1/1.
- **sql-queries before SQL**: the model; left open. Gemini loaded it before the first query in 0 of 16 transcripts, mistral in 8 of 33; 3 of 135 queries without it failed, each fixed in the next call.

Open: Gemini's schema runs with the last migrations line (a new day's budget); the news key on Gemini; R3's missing pipelines on both models.
