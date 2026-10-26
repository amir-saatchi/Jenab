# SPIKE-034 — Topics: threads inside a chat, with their own notes and history
**Type:** Spike
**Status:** In progress (round 1: topics are out; round 2 done on gemma and ministral: sub-sessions with a notice up work; Gemini to run)
**Gate:** 3

## Question
A chat with a lasting role, such as a *SQLite expert*, gets many subjects over time: the Users table, then Orders, then a question about both. In one chat they share one history window. After a few cuts (SPEC 3.6), the agent finds earlier work on a subject only through `search_history`, and the work on other subjects fills the window in between.

SPIKE-029 tried to fix this inside one chat: the agent kept a list of **subjects**. Keeping the list worked, but the agent's grouping was the weak step. It named subjects after the step it had just taken, so a change to existing work made a duplicate. A subject could grow and merge, but it couldn't split. **Topics** make the split visible and let the user approve it.

Two questions:
1. **Does the agent propose a topic at the right moments?** That is, for a new subject the user will come back to. Not for a plain question, and not a second time for a subject that already has a topic.
2. **Do topics beat one long chat** that has cuts and `search_history`? Measured on follow-ups after many turns, on questions that span topics, asked in the main thread, and on cost.

The design to test (proposed on 2026-10-20, changed on 2026-10-27):
- **A topic is one level below a chat.** Any chat can have topics, Mother included. A chat's **main thread** stays a normal chat: small questions, overview, and questions that span topics. A topic can't have topics of its own.
- **A chat is who, a topic is what.** A chat has a role (8.6). Its topics share that role and differ only in their subject.
- **A topic's context:** block 1 with the parent's role, user memory, project memory, the project card, the parent's session notes (read-only), the topic's own notes, and the topic's own history window. The main thread and the other topics aren't in its context. It reaches them through `search_history` and `read_messages`.
- **The topic index** sits in the main thread's context, one line per topic, e.g. `t2 · Users table · 1 open`. Like the chat list in Mother (8.6), it is built at cuts. A topic's context doesn't have the index.
- **Topic notes** follow SPIKE-029 round 3's proposed decisions: a short summary, notes that are only added to, and open items instead of a status. `merge_topics` joins two topics when the user says they are one thing.
- **Who opens a topic:** the user can always open one. The agent only **proposes** one, a new topic or an existing one, with `propose_topic(title | topic_id)`. When the user accepts, their message moves into that topic, and the turn runs there. The agent never opens a topic on its own.
- **Facts go to memory, the state of the work goes to notes:**
  - A topic writes user memory and project memory like any chat (3.3). A fact learned in a topic, such as "email is unique in Users", is true for the whole project, so it goes straight to memory.
  - A topic reads its parent's session notes but never writes them. Only the main thread writes them. Each thread writes only its own notes, so the parent's notes don't become a mix of several threads.
  - The main thread learns what a topic did from the topic index, which is built from each topic's own notes. When the user wants more than the index line, *Send back to parent* puts the topic's last reply into the main thread as a notice. Nothing goes up without it.
- **Parallel turns:** topics of the same chat can run turns at the same time. They share no notes, so only memory and the project's objects can conflict, and the revision checks catch that (3.3).
- **Storage, if adopted:** a row in `chats` with a `parent_chat_id`, so history, notes, *Refresh memory*, *Clear*, undo and search are reused. A topic isn't in the sidebar's chat list; it shows inside its chat.
  - **Undo** stays per thread: a topic's changes belong to the topic's own turns.
  - **Archive:** archiving a chat archives its topics too, and unarchiving it brings them back. Mother can't be archived (8.6), so her topics are archived one by one.

**Also in the design, not measured here** (2026-10-27). These are UI and lifecycle notes for the SPEC change, if topics are adopted:
- **Tree view in the sidebar:** at most three levels.
  - Mother is pinned at the top, and the chats she created (`created_by`) sit under her.
  - Chats the user created sit at the top level.
  - Each chat's topics sit under it, folded by default.
  - Subagent runs stay inside the parent turn, as today (8.6).
- ***Continue as topic*** on a subagent row: it opens a topic under the chat that ran the subagent, so the user can follow up on delegated work.
  - The topic starts with the subagent's task as its first user message and the subagent's result as its first reply.
  - The full transcript stays in the bucket, linked from that reply with `ref` (3.7).
  - The subagent itself stays one-off and one level deep. Only the topic continues.

**Relation to SPIKE-029:** if topics win, they replace subjects, and P2-10 (session notes) depends on this spike instead of SPIKE-029. SPIKE-029's status stays as it is until this spike decides. Condition 4 below runs SPIKE-029's proposed combined design, the round 4 it never ran, so the two can be compared on the same chats.

**Out of scope: report-up.** This is a chat or topic telling its parent, or Mother, that something changed:
- a setting, *Mother hears*, with the values off, notice and wake
- a `tell_mother(message)` tool
- the star shape: only Mother sends to other chats
- a hop limit for chains of turns the app starts

It is a separate question, about cost and loops rather than about context. It depends on `create_chat` and `send_to_chat`, which land in Phase 5. It is a possible later round, once topics are decided.

## Conditions
1. **Baseline:** one chat with today's loop, cuts and `search_history`.
2. **Topics, opened by the user:** the scripted user opens a topic at the first message of each subject and writes each later message on that subject in its topic. The agent doesn't route anything. This measures what topics are worth when the split is perfect (question 2).
3. **Topics, proposed by the agent:** the scripted user writes in the thread they are in, and the agent has `propose_topic`. The user accepts a proposal that matches the message's label and declines any other. This measures question 1 and the end-to-end result.
4. **Subjects:** one chat with SPIKE-029's proposed design: one subject per piece of work, notes only added to, open items, `merge_subjects`, and the index. This is the comparison that decides whether topics replace subjects.

## Tests
- **Two scripted chats of about 18 messages**, run with a low `history_max_turns` so cuts happen during the chat, as in SPIKE-029.
  - A *SQLite expert* chat: Users, Orders and Products tables.
  - A second chat on another theme. SPIKE-029's jobs chat can be reused, with its applications table, follow_up pipeline and Zalando watcher.
- **Each message has a label:** its subject, or `main`. The label decides where the scripted user writes in condition 2, and which proposals they accept in condition 3.
- **Each chat mixes:**
  - **work** on three or four subjects, interleaved
  - **returns** to a subject after several turns on others
  - **changes of mind** on an earlier decision
  - **plain questions** that must not get a topic
  - **cross-topic questions** asked in the main thread, e.g. "how do Users and Orders relate?" or "what's still open everywhere?"
  - **a message on an existing subject sent from the main thread**, to catch duplicate topics
  - **a merge message:** "Products and Inventory are one thing for me"
- **Follow-ups after cuts** that need an older turn or reason, e.g. "why did we make email unique?" or "is the Orders table done?".
- **The scripted user in condition 3:**
  - They write where they are.
  - They go back to the main thread themselves for `main` messages.
  - They never open or switch to a topic on their own, so every move into a topic, new or existing, comes from a proposal.
  - A message whose label differs from the current thread and gets no proposal is answered where it is. That counts as a miss.
- **One parallel step:** two messages to two topics of the same chat, sent at once. Both turns read the parent's notes and write project memory.
- **A decision made in a topic, then asked about in the main thread**, e.g. "did we settle the Orders key?". The main thread has only the index and search, since topics don't write its notes.

## Measures
- **Proposals (condition 3):**
  - new subjects that got a proposal
  - proposals for plain questions or `main` messages
  - duplicate topics: a new topic proposed when one already exists for that subject
  - returns proposed into the right existing topic
  - the cost of a proposal: requests and tokens in the turn that only proposes
- **Payoff:** follow-ups after cuts answered right, and cross-topic questions answered right, per condition.
- **History reads:** `search_history` and `read_messages` calls per run, and how many of them reach another thread.
- **Notes:** whether waiting work shows as an open item, and whether the parent's notes stay unchanged by topics. After the parallel step: memory revision conflicts and what the agent did with them.
- **Reach of the main thread:** decisions made in topics that the main thread answered right, and whether it used the index alone or needed `search_history`.
- **Cost:** requests and prompt tokens per run. Cache reads, where the provider reports them. The length of the beginning that a topic's requests share with the main thread's, as an upper bound on cache reuse where the provider reports nothing (SPIKE-029: Ollama Cloud reports no cache reads).

## Models
The reference models (DEVELOPMENT.md): Gemini `gemini-3.5-flash-lite` and Ollama Cloud `gemma4:31b`.
- The prompts and tools are the same for every model (SPEC 1).
- Which keys to use, including whether to use the reserve keys (`OLLAMA_API_KEY_II`, `Z_API_KEY_II`), is confirmed with the user before any run.
- A free tier's failures (rate limits, quota) are recorded and that model stops; there are no workarounds for them.

## Setup
- **Harness:** `spikes/034-topics/`, built from SPIKE-029's (scripted chats, a fake project world, scoring, a transcript per run). It runs the app's real orchestrator (`internal/agent`) with the real `search_history` and `read_messages`, and fakes for the tools the app doesn't have yet.
- **Topics in the harness:** each topic is a chat in the same project in the store. The harness keeps the parent link.
  - On an accepted proposal, the user's message is sent again in the topic. The turn that only proposed stays in the thread it was sent in, and its cost counts.
  - `propose_topic`, `update_topic`, `merge_topics` and the notes tools are spike code.
- **Per-chat context:**
  - `Deps.Card` is per project, but a topic needs its parent's notes, its own notes, and, in the main thread, the index. These must go into a block that is chosen per chat.
  - Like SPIKE-032's `Grounding`, a spike-only hook in `Deps` may be needed. That changes app code, unlike SPIKE-029, so it is agreed with the user before the harness is written.
- **Safety:** keys come from `.env` and stay in memory. Only synthetic text is sent.

## Done when
Each condition has results per model, with 3 reps per cell: 2 chats × 4 conditions × 2 models × 3 reps = 48 runs. There is a decision on:
- topics: yes or no, and whether the agent proposes them or only the user opens them
- whether topics replace subjects (SPIKE-029), and P2-10's dependency
- the topic index: its line format and where it sits in the context
- the parent's notes: whether the index and *Send back to parent* keep the main thread up to date while topics only read its notes, or topics need to write them
- whether report-up gets its own round

## Result
Ran on 2026-10-27 on gemma4:31b (Ollama Cloud) and ministral-14b-latest (Mistral, standing in while Gemini's free quota was used up): 2 chats of 19 messages × 4 conditions × 3 reps per model, 48 runs, none lost. Gemini `gemini-3.5-flash-lite`, the second reference model, is still to run; it runs with round 2. The harness is in [`spikes/034-topics/`](../../spikes/034-topics/README.md), the findings and tables in its [`results.md`](../../spikes/034-topics/results.md). It adds one spike-only app hook, `agent.Deps.Notes`: a block 5 chosen per chat, which the app leaves out.

| | 1 one chat | 2 user opens topics | 3 agent proposes | 4 subjects |
|---|---|---|---|---|
| Follow-ups after cuts, gemma / ministral | 8/9 / 8/9 | 9/9 / 6/9 | 9/9 / 4/9 | 9/9 / 5/9 |
| Cross-topic questions in main | 15/18 / 15/18 | 9/18 / 6/18 | 15/18 / 10/18 | 15/18 / 12/18 |
| Decisions made in a topic, asked in main | 6/6 / 6/6 | 3/6 / 3/6 | 6/6 / 5/6 | 6/6 / 6/6 |
| Prompt tokens per run (k) | 135 / 122 | 148 / 118 | 153 / 143 | 153 / 161 |

- **The agent doesn't propose topics.** gemma proposed a new topic at 1 of 70 chances and a return once; ministral proposed none in 72. Condition 3 ends up as one chat. gemma did propose a topic on the merge message in all 6 runs, where the user asks for two pieces of work to be kept together.
- **A perfect split doesn't beat one chat.** With the scripted user opening every topic (condition 2), follow-ups after cuts went from 8/9 to 9/9 on gemma and from 8/9 to 6/9 on ministral.
- **Topics cost the main thread its reach.** Cross-topic questions asked in main fell from 15/18 to 9/18 (gemma) and 6/18 (ministral); decisions made in a topic were answered in main 3 of 6 times against 6 of 6. Two causes:
  - An index line names a topic but doesn't say what was done there. Asked "is anything automated yet?", gemma read the applications topic and answered "no" in 3 of 3 runs; the follow_up topic had made the pipeline.
  - Blocks 2–5 are rebuilt at a thread's own cuts, and the main thread has few turns, so its project card is old. ministral answered "Monday, 14 days" from main's card after a topic had changed the pipeline to Friday and 10 days. In one chat the change is in the history window.
- **Subjects (condition 4, SPIKE-029's combined design, its round 4) hold up on gemma, not on ministral.** gemma: every follow-up right, waiting work as an open item 6/6, merge 3/6, 0.5 duplicates a run, +14% prompt tokens. ministral: follow-ups 5/9, waiting 3/6, merge 3/6, and 34 of 37 subject writes sent alone, +32% prompt tokens.
- **Merging works better on subjects than on topics:** one subject after the merge message in 3 of 6 runs per model, one topic in 1 of 6.
- **Parallel turns are safe.** Two topics saving project memory at once gave 3 and 6 revision conflicts; each was merged and saved again, and memory had both facts in every run. The app tells a running turn with a notice when memory changes under it.
- **Cost:** requests per run were about the same in every condition. A topic's first request shares about 2,370 of 2,720 estimated tokens of tools and system text with the main thread's. Mistral reports cache reads (15–20% of prompt tokens); Ollama Cloud reports none.
- **Storage:** the store keeps chat titles unique per project, and proposed topic titles often matched another chat's. Topics as `chats` rows would need a title rule.

**Decisions after round 1** (2026-10-27):
- **Topics are out** (decided with the user). The agent doesn't propose them, and even opened by the user they cost the main thread more than they gain in follow-ups. The design above stays as the record of what was tested.
- **Instead: visible sub-sessions,** tested in round 2. A chat starts one when the user asks, or when the work needs it, like a subagent the user can see, open and continue. It builds on Mother's `create_chat` (8.6), for any chat and one level deep.
- **Topics don't replace subjects.** P2-10 keeps depending on SPIKE-029, not on this spike. Condition 4 is SPIKE-029's round 4: its combined design works on gemma, and SPIKE-029 can take that result. ministral's numbers say a weaker model keeps subjects poorly, so the rule must stay short and the notes optional.
- **The index:** `t2 · Users table · 1 open · chat <id>` in block 5 isn't enough on its own: it doesn't say what was done, and it is as old as the parent's last cut. Round 2 tests a line with the summary.
- **The parent's notes:** a child that only reads them, with the index as the way up, doesn't keep the parent current (3 of 6 decisions). Round 2 tests the two ways up.
- **Report-up:** round 2 tests its smallest form, a notice in the parent without a turn. Telling Mother, `tell_mother` and hop limits stay with Phase 5.
- **Gemini** runs round 1's conditions 1, 2 and 4 with round 2. Condition 3 is dropped for it, since proposing is out.

**Round 2** (planned on 2026-10-27): **visible sub-sessions.** A chat starts a sub-session, which the user sees under it and can open and continue. Creating one is a tool call the agent makes when the user asks, or when the work needs it. The question is the one topics failed: **does the parent stay current when the user works inside a sub-session?** Two ways up are compared.

The design under test:
- **`create_session(title, message)`**, in any chat that isn't a sub-session itself (one level, like subagents). It opens a sub-session under the chat with `message` as its first user message, and the turn starts at once. The tool returns the session's ID.
- **A sub-session** has the parent's role, the parent's session notes (read-only), its own notes and its own history. Like round 1's topics, it has `update_notes` and the project tools.
- **The parent's list of sub-sessions** is in block 5, built at the parent's cuts like Mother's chat list (8.6).
- **Finish notice:** when a turn the parent started ends (the first turn from `create_session`), the parent gets the sub-session's last reply as a notice, as Mother does for `create_chat`. In both conditions. Here the notice waits for the parent's next turn instead of starting a finish turn; SPIKE-023 tested finish turns.
- **The rule** in the parent: use `create_session` when the user asks for a separate session, or for lasting work the user will come back to; answer plain questions yourself. It follows delegation guidance v1 (8.6).

Conditions (both chats, the same messages as round 1):
5. **Notice:** every turn that ends in a sub-session, the user's own included, sends its last reply to the parent as a notice, cut to 1,000 characters. It is written into the parent's history when the parent's next turn starts, like a memory-changed notice (3.6), and starts no turn. The list line is round 1's: `s2 · Orders table · 1 open · chat <id>`.
6. **Summary line:** no notices beyond the finish notice. The list line adds the sub-session's summary from its notes: `s2 · Orders table · 1 open · chat <id> · Orders has an INTEGER key and a user_id reference; …`. The line is as old as the parent's last cut.

Round 1's conditions 1 (one chat) and 2 (topics the user opens, the index only) are the baselines.

The scripted user:
- They open a sub-session themselves at a subject's first message, as in round 1's condition 2, and write each later message on that subject in it. The `main` messages go to the parent.
- In each chat, one subject is asked for instead: *"Do this in its own session: …"*. That is the inventory message in `sqlite` (8) and the Zalando message in `jobs` (6), both with waiting work and later returns. Later messages on that subject go to the sub-session the agent made. If it made none, the user opens one at the next message on that subject.

Measures:
- **Asked creation:** one `create_session` call at the asked message, with the task in its message and no work tool called in the parent's turn; and whether the finish notice was passed on in the parent's answer.
- **Creations nobody asked for:** `create_session` calls at other messages, for a `main` message or a subject that has a sub-session. Every subject gets its sub-session from the user or the ask, so this measures false creations only; SPIKE-023 measured choosing to delegate.
- **Payoff in the parent**, as round 1: cross-topic questions, decisions made in a sub-session and asked in the parent, and whether that turn read history.
- **Payoff in the sub-sessions:** follow-ups after cuts, calls that changed earlier work, waiting work as an open item.
- **Cost:** requests and prompt tokens per run, the parent's prompt tokens on their own, and the tokens the notices add.

**Setup:** the round 1 harness, with two new conditions.
- `create_session` and the sub-session list are spike code; a sub-session is a chat in the same project, titled `sN · Title` for the store's unique titles.
- The notices need a second spike-only app hook, agreed with the user before it is written: `agent.Deps.Notices`, which gives the notices for a chat when its turn starts. The app writes them like its memory-changed notices; the app leaves it nil.

**Runs:** 2 chats × 2 conditions × 3 reps on gemma4:31b, ministral-14b-latest and gemini-3.5-flash-lite: 36 runs, about 40 requests each.
- Gemini also runs round 1's conditions 1, 2 and 4: 18 runs. With a smoke run, that is about 1,300 requests on its free tier of about 1,500 a day, so a run that stops on the quota finishes the next day with `-resume`.
- gemma and ministral take about 500 requests each.

**Done when:** round 2 has results for the three models, and there is a decision on:
- sub-sessions: yes or no, and the rule for when the agent starts one
- the way up: the notice, the summary line, both, or neither; and the notice's size
- what a sub-session's line in the parent's list holds
- what changes in SPEC 8.6: `create_chat` beyond Mother, or a separate `create_session`

**Round 2 result** (2026-10-27, gemma4:31b and ministral-14b-latest; Gemini to run): 2 chats × conditions 5 and 6 × 3 reps per model, 24 runs, none lost. The findings and tables, next to round 1's baselines, are in [`results-round2.md`](../../spikes/034-topics/results-round2.md). The second spike-only hook, `agent.Deps.Notices`, writes the notices; the app leaves it nil.

| gemma / ministral | 1 one chat | 2 user opens topics | 5 sub-sessions, notice | 6 sub-sessions, summary line |
|---|---|---|---|---|
| Asked: one `create_session` with the task, no work in the parent | – | – | 6/6 / 6/6 | 6/6 / 6/6 |
| Cross-topic questions in the parent | 15/18 / 15/18 | 9/18 / 6/18 | 15/18 / 17/18 | 11/18 / 12/18 |
| Decisions made in a sub-session, asked in the parent | 6/6 / 6/6 | 3/6 / 3/6 | 6/6 / 6/6 | 6/6 / 5/6 |
| Follow-ups after cuts | 8/9 / 8/9 | 9/9 / 6/9 | 9/9 / 6/9 | 9/9 / 6/9 |
| Finish notice passed on in the parent's next answer | – | – | 0/6 / 2/6 | 2/6 / 3/6 |
| Prompt tokens per run (k), the parent's own | 135, 135 / 122, 122 | 148, 64 / 118, 37 | 149, 62 / 146, 52 | 157, 69 / 163, 70 |

- **Asked, both models start a sub-session; unasked, almost never.** 12 of 12 asked turns made one `create_session` call with the whole task and did no work in the parent. One call in 24 runs was unasked: ministral on the merge message, instead of joining two sub-sessions.
- **A notice after every sub-session turn keeps the parent current.** With it, the parent answered cross-topic questions as well as one chat or better, and every decision made in a sub-session, without reading history. "Is anything automated yet?", which round 1's topics failed, passed 3/3 on both models.
- **The summary line doesn't.** It is as old as the parent's last cut, and a parent with few turns has few cuts: gemma wrote the pipeline into the follow_up sub-session's notes at message 5 and the parent still said "nothing is automated" at message 10, in 3 of 3 runs.
- **Work in a sub-session holds up** as in round 1's topics: follow-ups 9/9 (gemma) and 6/9 (ministral), calls that changed earlier work 18/18 on both in condition 5. ministral still writes few notes, so its waiting work rarely shows as an open item.
- **The parent rarely passes the finish notice on** when its next message is about something else. The user sees the sub-session; a finish turn, as Mother has (8.6), is the way to pass a reply on.
- **Cost:** with notices, +11% (gemma) and +20% (ministral) prompt tokens per run against one chat; the notices themselves are about 440 and 1,060 tokens a run, and none reached the 1,000-character cut.

**Proposed after round 2,** for the user to decide once Gemini has run: sub-sessions yes, started when the user asks or for lasting work, one level; the way up is a notice after every sub-session turn, cut to 1,000 characters, without a turn; the list line stays round 1's, without the summary. Whether SPEC 8.6 gets a separate `create_session` or `create_chat` beyond Mother is the user's call.
