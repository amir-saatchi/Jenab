# P1-10 — Agent loop
**Type:** Feature
**Status:** Done
**Gate:** 3 (Phase 1)
**Needs:** P1-06, P1-08, P1-09
**Requirements:** R-14, R-20, R-21, N-40, N-43, N-44

## Goal
The orchestrator: turns, steps, the context builder and the history window (CODE-OUTLINE 7, SPEC 3.1, 3.6, 8.3).

## Scope
- **Orchestrator:** a runner per chat, turns and steps, the coalescer, and the `Publisher` for `chat:delta`, `chat:part` and `chat:status`.
- **Context (3.1):** the blocks that exist in Phase 1: the system prompt, the role, the skill list, and the chat list in Mother. Memory, the project card and session notes come in Phase 2.
- **History window (3.6):** cuts, stubs, in-turn trimming, and the "earlier turns" line.
- **During a turn (8.3):**
  - user messages join at the next step
  - *Stop* closes open tool calls as cancelled and keeps the text so far
  - `turn_max_requests`, with a last request without tools
  - provider failures: retries with the wait shown in the chat, *Retry now* and *Cancel*, an error card after 10 minutes, and a clear message for `quota` and `request` errors (8.3)
  - the prompt rule for requests with several parts
- **Titles:** generated after the first turn with the `fast` model.
- **Later:** background tasks and finish notices come in Phases 4 and 5 (R-15), and subagents with `max_subagents_per_chat` and the limit in the tool description in Phase 5. The runner leaves room for them.

## Done when
- Integration tests with `provider/fake` and a real store:
  - a 30-turn chat cuts as configured
  - a message sent during a turn joins at the next step
  - *Stop* leaves a history both protocol shapes accept
  - a restart mid-chat keeps the history
  - a chat turn starts while every background slot is taken
  - a rate-limited turn shows the wait and finishes after it
- The coalescer and the stall handling are tested with `synctest`.

## Result
- **`agent`:** `Orchestrator` with `Send`, `Retry`, `Stop`, `Clear`, `Delete`, `Wrote`, `Live`, `ChatStatus`, `Refuse` and `Wait`; `Publisher` (delta, part, status; `OpenPage` comes with the pages); `Skills` for block 1, nil until P1-12. A chat has at most one runner goroutine: it runs the queued turn, then a turn queued meanwhile, holds a project lease and shows once in the project's activity.
- **Answers are stored once:** an answer is kept in memory while it streams and written in one `AppendMessage` when complete, so a failed or retried try leaves nothing behind. Each try makes its IDs when it starts, under the chat's lock like `Send`, and messages sort by (turn, ID), so a message sent during the try comes after the answer and its results. `Status.Streaming` names the answer being streamed; `Live` gives its parts and text so far to a chat opened mid-turn.
- **Context:** the base prompt and the prompt rule from SPIKE-023, Mother's line and chat list, the role, the skills, then the earlier-turns line; the last block is cached. These blocks are built at cuts and kept until the next one, so the cache holds between them and a role change waits for the next cut.
- **History window:** cut past 8 turns or 24,000 tokens, after 5 minutes idle, and on a chat's first turn in a new process or after *Clear*. Results of turns before the cut become stubs; a call a crash left open gets a "no result" result, so every provider accepts the history. In-turn trimming keeps the two newest previews.
- **During a turn:**
  - A message sent while the model answers gets another step, also after the request cap (one more request without tools).
  - *Stop* keeps finished text and the text so far; the last kept text part is marked stopped. Thinking, calls and blank text of the unfinished answer are dropped, and open calls get "cancelled by user". A message after *Stop* starts a new turn. The app's end writes "cancelled: the app closed", a project's close "cancelled: the project closed", and no error card; a project's close waits up to 5 s for that.
  - Retries show the wait in `chat:status` (`retry`), with the pause's kind, until the wait is over. *Retry now* also ends the provider's pause and wakes every call waiting on it. A message sent during the wait goes into the next try. After 10 minutes, or when the provider asks to wait past them, a `turn_failed` notice; *Retry* continues the same turn with a new `turn_max_requests`, after a restart too. `quota` (with the provider's message), `request`, too-large and unknown-model errors get their notice at once.
  - An answer without calls that hit the output limit, was refused or has no text gets an `answer_cut` notice.
  - Calls in the last request's answer are closed unrun.
- **Titles:** after the first turn of an untitled chat, with `fast` (or the chat's model), beside the next turn and one at a time; background priority, 200 output tokens, stopped by the app's end or the project's close, dropped if the chat was cleared meanwhile. Cleaned to one line of at most 100 characters in NFC, without quotes, direction marks or joiners at the ends.
- **Additions in other packages:** `ChatsDB.LastTurn`, `ChatsDB.LastActivity` (one index lookup per chat, a new `messages_activity` index), `Registry.Resume` (wakes waiting calls), `Event.Paused` and the wait-over `EventWait`, `fake.Provider.Replace`, `chat.Status.Retry` and `Streaming`, `NoticeTurnFailed` and `NoticeAnswerCut`, `tool.Env.ChatStatus` (was in `tool.Deps`). The Anthropic encoder skips blank text blocks; `history_max_turns` must be at least `history_min_turns` + 1.
- **Changes from the plan:**
  - The runner is one goroutine per chat with work, not a loop over an inbox: user messages are stored at once, the turn sees them at its next step, and a message after *Stop* queues the next turn.
  - *Clear* goes through the Orchestrator and fails during a turn (`ErrInTurn`): before, a cleared chat kept its turn count and window.
- **Not wired yet:** the app, `tool.Env.ChatStatus`, and `Registry.Apply` when settings change (P1-13); `PrivateHosts`, the preflight and the Untrusted mark (P1-11).
- **Open risks:**
  - Anthropic may refuse the last request (no tools) when the history has tool calls; to check with a real key.
  - A too-large request gets its notice; nothing shrinks it by itself yet.
  - Mother's delegation guidance waits for Phase 5.
- **Tests:** every Done-when case, plus the request cap, titles, Mother's context, tool failures and panics, in-turn trimming, the idle cut, *Clear*, shutdown mid-turn, Retry now, giving up after 10 minutes, errors that are not retried and stalled streams; the coalescer, the stall and retry waits with `synctest`. After the review: a message after *Stop*, a second *Stop*, the title at shutdown, one title at a time, the title as background work, *Clear* during the title, *Delete*, `Wrote`, `Live` and `Streaming`, a panicking publisher, a message during a retry wait, *Retry* after a restart, the overload pause, giving up when the wait is too long, cancelling a retry wait, the `answer_cut` notices, a role change waiting for the cut, and the turn of a crash repair.
- **Mutation checks:** 51 on `agent`; 35 made the tests fail at first. Of the 16 missed:
  - 9 got tests (the stub boundary at a cut, the trim threshold, the line for one earlier turn, *Retry now* not counted as a failure, `bumpSeq`, a runner ending while the next turn runs, two text parts of one answer, `cutBytes`).
  - 4 led to fixes. A cleared chat kept its turn count and window, so *Clear* now goes through the Orchestrator. A message during the last request was never answered. A project's close didn't wait for the turn's last writes.
  - 1 changed nothing, so the code was removed (a Mother check before the title).
  - 2 stay: a zero-length timer (the same as sending at once) and a guard for a stale timer that fake time can't reach.
  - 17 more on the changed code all make the tests fail.
- **Review:** four review agents read the new and changed packages. Every finding is fixed (above), except:
  - `EstimateTokens` stays at 4 bytes a token. Persian text comes out at about one token for two letters, close to real tokenizers; a higher rate would flag requests as too large too early.
  - The 2 kept from the first mutation round, and 4 guards from the second (below).
- **Mutation checks after the review:** 40 on the new code; 27 made the tests fail at first. Of the 13 missed:
  - 4 were broken mutants; redone, all make the tests fail.
  - 3 got tests (a message joining a queued turn, one title at a time, the title's background priority).
  - 2 changed nothing, so the code was simplified (a status publish after a failed try, a check that a failed turn has nothing queued).
  - 4 stay: guards for a turn that is queued but not yet picked up (in `Clear`, `Retry` and the runner after the app's end) and for *Stop* coming between an answer's end and the turn's last check. Tests can't hold the code in that moment.
