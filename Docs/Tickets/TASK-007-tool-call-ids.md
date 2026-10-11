# TASK-007 — Tool-call IDs and arguments that every API accepts
**Type:** Bug
**Status:** Done
**Gate:** 3

## Goal
A chat's tool calls can be sent to any provider, also after the chat changes its model. Found in the harness review of 2026-10-10.

## Today
- **Repeated IDs.** Chat Completions makes `call_0`, `call_1`, … when the provider sends no ID (`openai/chat.go:178-180`), so each step reuses the same IDs. Gemini and Ollama already use `newCallID()`.
  - In-turn trimming marks calls by ID (`agent/context.go:146, 199`). Stubbing an old `call_0` also stubs the newest one, which breaks "all but the two newest" (SPEC 3.7).
  - Anthropic refuses repeated `tool_use` IDs, so the chat breaks if it moves to a Claude model.
- **IDs sent as they came.** Anthropic accepts only `^[a-zA-Z0-9_-]+$`, and Kimi's IDs look like `functions.read_file:0`. A chat that moves from Kimi to Claude gets a `bad_request` on every request while those calls are in the window.
- **Arguments that aren't an object.** `provider.ToolCall` accepts any valid JSON (`provider/provider.go:115`), so `null`, `[]` or `"x"` become `Args`. Anthropic then gets `input: null` and refuses every request while the call is in the window. Only Gemini checks for an object (`gemini/gemini.go` `isObject`).

## Scope
- **Missing IDs.** `newCallID()` moves into `provider` and every backend uses it for a missing ID.
- **ID mapping.** Each encoder maps a stored ID to a form its API accepts:
  - The mapping is a pure function of the ID, so the cached prefix stays the same and results still match their calls.
  - An ID that already fits is unchanged.
  - The rule is per API kind (Anthropic, Chat Completions, Responses, Gemini, Ollama), never guessed from the base URL.
- **Non-object arguments.** `provider.ToolCall` treats arguments that aren't a JSON object as `Invalid`. The model gets the error and the call is stored with `{}`, as in TASK-003.

## Not in scope
- Rules for one compatible provider, such as Mistral's 9-character IDs, unless a reference model needs them.

## Done when
- Two steps without provider IDs give different IDs.
- A history with `functions.read_file:0` encodes to a valid Anthropic request, and the result still matches its call.
- `null`, `[]` and `"x"` as arguments are `Invalid` in every backend.
- A fake-provider test moves a chat from a Chat Completions model to Anthropic mid-history, and the next request encodes.
- `go test ./...` passes.

## Result
- **Missing IDs:** `provider.NewCallID()` gives every call without an ID a new random one, in every backend. Chat Completions no longer makes `call_0`, `call_1` in each step.
- **ID mapping:** `provider.CallIDRule` maps an ID the API can't take to `call_` plus a hash of the ID, so it is the same in every request and the result still matches. An ID that fits is unchanged.
  - Anthropic: only `^[a-zA-Z0-9_-]+$`.
  - Chat Completions: at most 40 characters, OpenAI's limit; any characters, since Kimi reads its own `functions.read_file:0` IDs.
  - Responses, Gemini and Ollama: no known rule, so IDs go as they are. Ollama sends no IDs at all.
- **Arguments:** `provider.ToolCall` makes anything that isn't a JSON object `Invalid`, and the model is told "the arguments must be a JSON object, not an array". Every encoder also sends `{}` for arguments stored as something else before this change.
  - Gemini's `null` in `step.start` and Ollama's `null` mean "no arguments" in their protocols, so they stay `{}` and valid.
- **Not done:** chats saved before this change may already hold a repeated `call_0`. The mapping can't tell those apart; Phase 1 has no user chats yet.
- **Tests:** `TestCallIDRule`, `TestNewCallIDIsNew` and `TestObjectArgs` in `provider`; `TestNonObjectArgs` in every backend; `TestChatCallIDsNew` and `TestChatLongCallIDs` (Chat Completions); `TestCallsFromOtherProviders` (Anthropic); `TestMoveToAnthropic` in `backends`, which plays three Chat Completions steps (Kimi's IDs, then no IDs twice) and checks the Anthropic request; `TestNonObjectToolArgs` in `agent`.
- SPEC 3.8 and 8.3 are updated.
