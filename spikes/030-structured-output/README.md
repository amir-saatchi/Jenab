# SPIKE-030: structured output

How Jenab gets JSON that matches a schema from any model. See `Docs/Tickets/SPIKE-030-structured-output.md`.

The harness calls the app's own provider registry and backends (`internal/provider`). It checks answers with the schema library the app uses for tool arguments. The only addition is under the backends: an HTTP layer that adds `tool_choice`, `response_format` or Ollama's `format` to the request body.

## Methods
- `tool`: a `submit` tool whose parameters are the schema; the prompt asks for the call.
- `tool-forced`: the same, plus `tool_choice` naming the tool (or `"required"`). Ollama's `/api/chat` has no `tool_choice`, so it is skipped there.
- `text`: the schema in the prompt; the first JSON value is read from the reply, also inside a code fence or after a sentence.
- `native`: as `text`, plus the API's JSON mode: `response_format` `json_schema` (or `json_object`), or Ollama's `format`.

Each method checks the answer against the schema and the index checks. A wrong answer is sent back once with what is wrong. For tools, it goes back as the tool's error result.

## Tasks
`data/tasks/`, with the right answers fixed before any run:
- `select-*`: pick and rank the 5 of 30 headlines about Ethereum, with traps (Ethiopia, Ethernet, diethyl ether, Ethan, ethics).
- `extract-*`: an event page; some fields are missing and must be `null`.
- `decide-*`: 20 customer messages, each with complaint, topic and urgency (±1 counts as right).
- `wide-en`: 40 fields; `nested-en`: an invoice with nested lists; `inject-en`: a page that tells the model to ignore the format; `long-en`: select over 700 generated headlines (about 15,000 tokens).

English, German and Persian for select, extract and decide.

## Run
```
go run . -probe                         # what each API accepts → results/probe.json
go run . -reps 3                        # main run → results/<time>.jsonl
go run . -thinking -methods <best>      # thinking on
go run . -resume results/<time>.jsonl   # finish an interrupted run
go run . -report results/*.jsonl        # → results.md
```
Keys come from `../../.env` and only go to each provider's own host. `go test .` checks the fixtures, the checks, the scoring and the retry offline.
