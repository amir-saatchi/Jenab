# SPIKE-034 — harness plan

A draft for review before any harness code. It becomes the README once the
harness is written. Built from SPIKE-029's harness (`spikes/029-subjects/`).

## What it reuses and what is new

- **From SPIKE-029:** the run loop on the real orchestrator, the fake project
  world (tables, rows, pipelines, views, canned `web_search`), `runs.jsonl`,
  a transcript per run, `-resume` and `-report`, the scoring style (regex
  checks on answers and call arguments).
- **From the app, real:** `search_history` (scope `chat` or `project`),
  `read_messages` (any `chat_id`), and now also `update_memory` with
  revisions and the user and project memory blocks 2–3 (P2-09 landed after
  SPIKE-029). So the parallel step's memory conflicts are the app's own.
- **Topics:** each topic is a chat in the same project, with the parent's
  role. The harness keeps the parent link and the topic ids (`t1`, `t2`, …).
- **Notes:** kept by the harness, saved per chat with the store's
  `SaveNotes`, so *Clear* and the store's revision rule apply.
- **New fake world tool:** `alter_table(name, add_columns?, change_columns?,
  primary_key?, unique?)`, for changes of mind on a table.
  `create_table` gets an optional `unique` list.

## Conditions

| | Threads | Agent's tools beyond the work tools | Block 5 |
|---|---|---|---|
| 1 baseline | one chat | – | – |
| 2 topics, user opens | main + topics | `update_notes`, `merge_topics` | main: own notes + topic index; topic: parent's notes (read-only) + own notes |
| 3 topics, agent proposes | main + topics | condition 2 + `propose_topic` | as condition 2 |
| 4 subjects | one chat | `update_subject`, `get_subject`, `merge_subjects` | subject rule + index |

Condition 4 is SPIKE-029's proposed combined design (rule "v5"): round 2's
size, one subject per piece of work named with a noun; round 3's `summary` +
`notes` that are only added to; open items instead of a status;
`merge_subjects`.

## The spike tools

The tool registry is shared by all chats (`Tools.For(kind)`), so one tool
acts on the thread it is called in (`env.Chat`).

- **`update_notes(title?, summary?, note?, open?)`** writes the calling
  thread's own notes: the main thread's session notes, or the topic's notes.
  `summary` replaces, `note` is added with its message id, `open` replaces
  the list (send all of it, `[]` when nothing waits). `title` renames a
  topic; in the main thread it is an error. No tool writes another thread's
  notes. (The ticket calls the topic's version `update_topic`; one name for
  both threads is simpler for the model, since the tool set is the same in
  every chat.)
- **`propose_topic(title | topic_id, reason?)`** asks the user to move the
  current message into a new topic or an existing one. It works like a card:
  the scripted user answers inside the call.
  - Accepted: the result says the message moves to topic `tN` and the turn
    should end with one short line. After the turn the harness creates the
    topic if new (title from the proposal), and sends the message again in
    it. The proposing turn's cost counts.
  - Declined: the result says so, and the agent answers where it is in the
    same turn.
  - Allowed in a topic too, for a sibling or an existing topic; a topic
    never gets topics of its own.
- **`merge_topics(ids, title)`** joins topics: the first id is kept, the
  others' notes and open items move into it in time order, their ids lead to
  it, and the index shows one line. The merged topic's chat stays readable
  through `read_messages`.

## Block 5 (needs the hook, see "Question for the user")

Built at a cut, like blocks 2–4, so the index is as of the last cut.

- **Main thread:** the topics rule, its own session notes, and the index,
  one line per topic: `t2 · Users table · 1 open`.
- **Topic:** `Topic t2 "Users table" of chat "SQLite expert"`, the rule,
  the parent's session notes marked read-only, and the topic's own notes. No
  index.
- **Condition 4:** the subject rule and the subject index, in the same
  place, so 3 and 4 are compared at one position.

## The scripted chats

18 steps each; step 12 is the parallel step, two messages. Labels decide where
the scripted user writes (condition 2) and which proposal they accept
(condition 3). `history_min_turns` 2, `history_max_turns` 4, as in SPIKE-029.

**`sqlite` — "SQLite expert".** Role: *You are a SQLite expert. You design
and change this project's tables and answer database questions.*

| # | Label | Kind | Message (short) | Check |
|---|---|---|---|---|
| 1 | users | work, new | users table: id key, email, name, created_at; email unique because people log in by email | |
| 2 | users | work | add Ana, Ben, Cem | |
| 3 | orders | work, new | orders: id, user_id, total, created_at; text key `ORD-0001`, as I read numbers to customers on the phone | |
| 4 | main | question | INTEGER PRIMARY KEY vs rowid? | no topic |
| 5 | products | work, new | products: sku key, name, price | |
| 6 | orders | work, return | user_id must point to users.id | call |
| 7 | main | question | is SQLite fine for 10,000 orders a year? | no topic |
| 8 | inventory | work, new, waiting | inventory table (sku, quantity); pipeline restock_check 07:00 for sku < 5, but wait: email or Slack not decided | open item |
| 9 | main | cross | how do users and orders relate? | answer |
| 10 | products | change, from main | add discount to products, default 0 | call; no duplicate |
| 11 | orders | change of mind | plain INTEGER key after all; the shop prints its own number | call |
| 12a ‖ 12b | users ‖ orders | parallel, memory | "remember: timestamps are UTC ISO-8601 text" ‖ "remember: money in whole cents, INTEGER" | both in project memory |
| 13 | main | cross, decision in topic | did we settle the key for orders? | answer: integer |
| 14 | main | merge | products and inventory are one thing: the catalog | one topic / subject |
| 15 | main | question | should I turn on WAL? | no topic |
| 16 | users | follow-up, from main | why did we make email unique? | answer: log in |
| 17 | products | follow-up | is the restock check done? | answer: waiting on email/Slack |
| 18 | main | cross | what's still open everywhere? | answer: restock |

**`jobs` — "Job search"** (SPIKE-029's jobs chat, reordered and extended).
Role: *You help me run my job search: applications, follow-ups and job
alerts.*

| # | Label | Kind | Message (short) | Check |
|---|---|---|---|---|
| 1 | apps | work, new | applications table, company + role key | |
| 2 | apps | work | add Zalando, N26, SAP | |
| 3 | apps | work, memory | "rejected" → status closed; remember for the project | memory |
| 4 | main | question | how long before following up? | no topic |
| 5 | follow_up | work, new | pipeline follow_up, Mondays 09:00, waiting > 14 days | |
| 6 | zalando | work, new, waiting | daily Zalando careers check; link comes later, wait | open item |
| 7 | main | question | thank-you email tips? | no topic |
| 8 | board | work, new, from main | view board grouped by status | |
| 9 | apps | change, return | N26 rejected me | call: closed |
| 10 | main | cross | which applications are still waiting, and is anything automated for them? | answer |
| 11 | follow_up | change, from main | Fridays, 10 days | call |
| 12a ‖ 12b | apps ‖ zalando | parallel, memory | "remember: only Berlin or remote" ‖ "remember: alerts only for roles with 'data' in the title" | both in project memory |
| 13 | main | merge | follow_up and the board are one thing: my Friday review | one topic / subject |
| 14 | main | cross, decision in topic | what day does follow_up run now, and after how many days? | answer: Friday, 10 |
| 15 | zalando | change, return from main | here's the link, go ahead | call |
| 16 | main | question | data engineer salary in Berlin? | no topic |
| 17 | apps | follow-up, from main | what happened with N26, why closed? | answer |
| 18 | main | cross | what have we set up, and is anything waiting on me? | answer |

## The scripted user

- **Conditions 1 and 4:** every message in the one chat; the parallel step
  is sent as two messages one after the other.
- **Condition 2:** the first message of a label opens its topic (title from
  the label, e.g. "Users table"); every later message of that label goes to
  its topic; `main` messages go to the main thread. The merge message goes to
  the main thread, where the agent has `merge_topics`.
- **Condition 3:** as the ticket says. They write where they are; they go
  back to the main thread themselves for `main` messages; they never open or
  switch to a topic on their own. A proposal is accepted when it matches the
  label: a new topic when the label has none, or the label's topic id.
  Anything else is declined. **One exception, the parallel step:** if both
  labels have topics, the user sends each message in its topic at once;
  otherwise the two go one after the other where the user is, with
  proposals as usual. The run records which happened.

## Scoring

- **Proposals (condition 3):** new-subject chances (a non-main message whose
  label has no topic yet) that got a matching proposal; proposals on `main`
  messages or inside the right topic (noise); a new topic proposed for a
  label that has one (duplicate); returns (label has a topic, user is
  elsewhere) proposed into the right topic; requests and tokens of the
  proposing turns.
- **Payoff:** answer and call checks per condition, split into follow-ups
  after cuts and cross-topic questions.
- **History reads:** `search_history` and `read_messages` per run, and how
  many reach another thread (`scope: project`, or a `chat_id` other than the
  caller's).
- **Notes:** the waiting work (`sqlite` 8, `jobs` 6) shows as an open item in
  its thread's notes (conditions 2–3) or subject (condition 4). The parent's
  notes can't be changed by a topic by construction; the harness checks that
  their revision moved only in main-thread turns.
- **Parallel step:** memory revision conflicts (tool errors from
  `update_memory`), whether the agent merged and saved again, and whether
  project memory ends with both facts.
- **Reach of the main thread:** the decision-in-topic question (`sqlite` 13,
  `jobs` 14) answered right, and whether that turn read history.
- **Cost:** requests, prompt and output tokens, cache reads where reported,
  and the shared prefix of a topic's first request with the main thread's at
  the same time (system blocks compared as text, in estimated tokens).

## Runs and budget

48 runs: 2 chats × 4 conditions × 2 models × 3 reps. SPIKE-029's 15-message
chats took 30–45 requests a run. Estimate here, per run: condition 1 ≈ 35,
2 ≈ 38, 3 ≈ 45 (proposal turns and resends), 4 ≈ 40; about **40 a run, about
950 per model** (range 800–1,200), plus a smoke run.

## What the build changed from this plan

- **Notes live in the harness**, not in the store: `ChatsDB.SaveNotes`
  exists, but nothing reads notes back into a context yet (P2-10), so the
  harness keeps them in memory and block 5 renders them.
- **The index line names the topic's chat**: `tN · Title · N open · chat
  <id>`, so `read_messages` and `search_history` can reach it.
- **Topic chats are titled `tN · Title`.** The store keeps chat titles
  unique per project, and a proposed topic title often matched the main
  chat's or another topic's. Storing topics as `chats` rows needs this, or a
  topic title that isn't a chat title.
- **A topic has no index of its siblings.** It sees its own notes and the
  main thread's; a message that belongs to a sibling topic can only be
  proposed back to main's topics from main. This is the design under test,
  not a harness limit.
- **The app adds a notice mid-turn** (a user message with a `notice` part)
  when project memory changes under a running turn. The parallel step meets
  it; the dry model looks past it.
- **Request texts come from `Traces.Turn`**, since `Traces.List` leaves them
  out.
- **The dry model** (`-dry`) is rule-based and checks only that every path
  of the harness runs; its scores mean nothing. Its runs took 38–48
  requests, in line with the estimate above.

## Round 2 — visible sub-sessions (plan)

The ticket's Result has the design, conditions and measures. What the
harness adds:

### Conditions

`-conds 5,6` (round 1's 1–4 stay as they are):

| | Sub-sessions opened by | Way up | List line in the parent's block 5 |
|---|---|---|---|
| 5 notice | the user; one per chat by asking the agent | finish notice + a notice after every sub-session turn | `s2 · Title · 1 open · chat <id>` |
| 6 summary | the same | finish notice only | `s2 · Title · 1 open · chat <id> · <summary>` |

### App hook (needs the user's OK)

```go
// Notices is SPIKE-034 round 2's way up: notices for a chat, written
// into its turn before each request, where memory-changed notices are
// written (3.6). It returns them once. Spike-only: the app leaves it nil.
Notices func(ctx context.Context, p *project.Project, ch chat.Chat) ([]chat.Notice, error)
```

In `runner.request`, after `r.memoryNotices(ctx, t)`: the notices are written
as one `RoleUser` message of `PartNotice` parts in turn `t.n`, like the
memory notices. About 15 lines, and a test in `card_test.go` style: a
notice reaches the next request once, and nil writes nothing.

The kind is a new `NoticeSessionUpdate = "session_update"`, or the
existing `task_finished`, which says it starts a finish turn. A new const
is clearer; it stays spike-only with the hook.

### Spike code

- **`sessions.go`** (new): the parent and its sub-sessions, built on
  `threads` (notes, `update_notes`, block 5 for each thread), with:
  - `create_session(title, message)`, parent only. It creates the chat
    (`sN · Title`), records it under the message's label, and sends
    `message` through the orchestrator (`o.Send`, the turn runs at once).
    It returns "Session sN started; its reply comes back as a notice".
    The harness waits for that chat as well as the parent before the next
    message.
  - A queue of notices per parent. When a sub-session's turn ends (the
    harness reads it in `read()`), its last reply, cut to 1,000
    characters, joins the queue as `Session sN · Title: <reply>`:
    always for the turn `create_session` started, and in condition 5 for
    every turn. `Deps.Notices` hands the queue over and empties it.
  - The list line, with the summary in condition 6.
  - The rule (block 5 of the parent): delegation guidance v1's wording,
    shortened to sub-sessions.
- **`scenarios.go`:** an `Ask` prefix on `sqlite` 8 and `jobs` 6, used
  only in conditions 5–6: "Do this in its own session: ". The other
  messages and every check stay the same, so round 1's conditions 1–2 are
  the baseline.
- **`run.go`:** the scripted user of round 1's condition 2 with the asked
  subject: the message goes to the parent; later messages on that label
  go to the sub-session the agent made, or to one the user opens if it
  made none.
- **`score.go` / `report.go`:** new rows:
  - asked creation (one call, task in its message, no write tool in the
    parent's turn);
  - creations nobody asked for, and duplicates;
  - the finish notice passed on;
  - the parent's prompt tokens;
  - notice tokens.
  The other rows are round 1's; the topic-proposal rows show "–".
- **`dry.go`:** calls `create_session` when a message starts with "Do
  this in its own session".

### Budget

About 40 requests a run; the asked turn adds one sub-session turn.
- gemma and ministral: 12 runs each, about 500 requests.
- Gemini: 12 runs plus round 1's conditions 1, 2 and 4 (18 runs), and a
  smoke run (conditions 5–6, `sqlite`, 1 rep): about 1,300 requests. If
  the quota stops it, `-resume` the next day.

### What the round 2 build changed from this plan

- **`sessions.go` is a mode of `threads`,** not a copy: sub-sessions are
  `sN` threads with the same notes, `update_notes` and block 5, and their
  own rules and wording. `create_session` puts the new sub-session under
  the asked message's label, unless the label has one already.
- **`merge_sessions`** joins sub-sessions, as `merge_topics` joined
  topics, so the merge message and the checks after it work as in round 1.
  The parent's rule names it.
- **The finish notice's check** is a pattern on the parent's answer in the
  turn that got the notice (`Passed`: `inventory|restock` in `sqlite`,
  `zalando` in `jobs`). That turn answers the next `main` message, so this
  measures whether the parent passes the news on unasked.
- **The asked message's waiting check** reads the sub-session's open items
  when the parent made one, and the parent's when it didn't.
- **The parent's answer stays the message's answer;** the sub-session's
  first reply is kept apart (`sub` in `runs.jsonl`) and shown in the
  transcript.
- **The notice kind** is the new `agent.NoticeSessionUpdate`
  (`session_update`), next to the hook.
- **Dry runs** (`-dry -conds 1,2,3,4,5,6`): 12 runs, 38–47 requests each,
  none failed. `sessions_test.go` checks the list line, the notice cut and
  that each notice is given once.
