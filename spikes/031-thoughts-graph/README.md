# SPIKE-031: Thoughts-Graph

Throwaway code for [SPIKE-031](../../Docs/Tickets/SPIKE-031-thoughts-graph.md), which is done: no Thoughts-Graph mode. The harness grows a graph of thoughts for a how-to task, one model call per expansion, and stores every thought, edge, weight change and call in SQLite. It then compares the graph's answer with one-call answers, using model judges and blind pairs for a person.

It uses the app's provider registry with the models in its own `models.yaml` (the scenario runner's format):
- Gemini: `GEMINI_API_KEY`.
- Ollama Cloud and Z.ai: the second keys, `OLLAMA_API_KEY_II` and `Z_API_KEY_II`.

Build the binary first (`go build -o <somewhere>/thoughtsgraph.exe .`) and run that, so changes to `internal/` during a long run don't affect it. Runs resume: an answer or verdict already in the database isn't made again.

```bash
go run . -list
```

```bash
go run . -model gemini/gemini-3.5-flash-lite,ollama-cloud/gemma4:31b
```

```bash
go run . -judge
```

```bash
go run . -blind 10
```

```bash
go run . -analyze
```

```bash
go test ./...
```

In order: list the tasks; grow the graphs and make the one-call answers; judge every pair; write the blind pairs; print the analysis; run the tests on the fake provider. `go run . -report` rewrites `summary.md` and `judges.md`.

Flags:
- `-mode`: `index` (the model sees the index and names `next`), `path` (the path only; the controller picks), or both, comma-separated.
- `-baselines`: `plain`, `thinking`, or both.
- `-judges`: the judge models, which run with thinking on.
- `-k` (3), `-max-thoughts` (16), `-max-depth` (4): the graph's size.
- `-max-tokens` (4000), `-thinking-tokens` (12000): the output limits.
- `-db`: the database, `results/round2/graphs.db` by default; the reports go next to it.
- `-v`: the provider layer's log.

## Keys and safety

- Keys come from the repo-root `.env` (or the environment), by the `key_env` names in `models.yaml`. They go into an in-memory keyring for the provider registry and are never printed or written.
- Only synthetic text is sent. A quota error stops that model's runs.

## Files

- `problems.go`: the five tasks, in English, German and Persian
- `prompts.go`: the system prompt with the schema, and the define, expand and conclude messages
- `graph.go`: thoughts, the index, paths, and the SQLite store (graphs, thoughts, edges, reweights, calls, answers, judgments)
- `call.go`: calls through the registry, JSON from the text, the schema check and the one retry
- `grow.go`: the controller (define, expand, conclude), its `pick` policy, and the one-call baselines
- `judge.go`: the judges, both orders, `judges.md`, and the blind pairs
- `report.go`, `analyze.go`: one report per graph, `summary.md`, and the analysis
- `round1/`: the round-1 code (tool-call answers, *siblings* and *one* modes)
- `results/round1/`, `results/round2/`: the reports. The `graphs.db` files are git-ignored. In round 2, `blind/` holds the pairs, `key.json` and `verdicts.md`.

Useful queries on a `graphs.db`:

```sql
-- the path from a thought up to the problem
WITH RECURSIVE up(id) AS (SELECT 42 UNION SELECT e.from_id FROM edges e JOIN up ON e.to_id = up.id)
SELECT t.num, t.kind, t.title, t.weight FROM thoughts t JOIN up USING (id) ORDER BY t.depth;

-- what each call of a graph cost
SELECT purpose, focus, tries, failures, input_tokens, output_tokens, ms FROM calls WHERE graph_id = 1;
```
