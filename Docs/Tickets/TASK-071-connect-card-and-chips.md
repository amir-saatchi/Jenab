# TASK-071 — The Connect card keeps the message; memory chips' Undo and Redo
**Type:** Task
**Status:** Done
**Gate:** 3
**Needs:** TASK-065

## Goal
Findings from the review of TASK-064 to TASK-070 (2026-11-17), in what TASK-065 built.

## Scope
1. **The Connect card can drop the kept message** (Medium). Once the new provider is saved, the settings reload and `noProvider()` turns false. If the screen redraws while the provider's model list is still loading, `keepConnect` drops the card, and `Connected` then returns at "the card closed meanwhile" (`internal/native/chat/connect.go:47`). The message isn't sent; only *Retry* sends it. The test misses this because its fake provider answers at once. A connect started from the card sends the kept message once it is done, whenever the card stops showing.
2. **Memory chips offer the wrong action** (Low). After the chip's turn is undone and brought back, or when the chip is clicked before its state has loaded (`internal/native/chat/chips.go`), it can offer *Undo* for an edit that is undone, or *Redo* for one that isn't. A chip offers an action only once its state is read, and reads it again when its turn is undone or brought back.

## Done when
- A test for each, with a fake provider whose model list answers late for item 1.
- `go build ./...`, `go vet ./...` and `go test ./...` pass.
- A live check on the bench build: connect from the card with a model list that answers late, and the message is sent.

## Result
Done.
1. A connect started from the card sends the kept message through *Retry* for its chat, whether or not the card still shows. A chat whose turn went on meanwhile, or that was deleted, is no error.
2. All three chips (memory, desktop, *Refresh memory*) offer *Undo* or *Redo* only once their state is read, read it again after each *Undo turn* or *Redo turn* in their chat, and drop late answers to older reads.

Tests for both; with each fix undone they fail. A live check on the bench build, with a provider whose model list answered 5 s late, sent the kept message and showed the answer. Report: [reports/TASK-071-report.md](reports/TASK-071-report.md).
