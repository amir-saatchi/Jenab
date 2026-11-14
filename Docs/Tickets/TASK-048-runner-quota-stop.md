# TASK-048 — The scenario runner stops a model when its quota is gone
**Type:** Task
**Status:** Open
**Gate:** 3
**Needs:** —

## Goal
`jenab-scenarios run` goes on sending requests after a provider says the daily quota or the credit is used up. On 2026-11-14, `gemini-4-hydrogen` allowed 20 requests a day: the first 20 went through, and the runner then sent 112 more that all came back 429, breaking every run in 18 sets. On a paid key, or a key shared with other work, this wastes money or the next day's budget, and the report shows "broke" where the truth is "not run".

## Scope
1. **Stop on `quota`.** The provider layer already gives a used-up daily quota or credit the kind `provider.Quota`, which is never retried (`internal/provider/error.go`). When a run ends on that kind, the runner starts no more runs on that model in this invocation. Other models go on.
2. **Report them as skipped.** The runs left are listed as `skip (quota: <provider>/<model>)`, counted in "skipped", not in "broke". The run that hit the quota stays as it ended. The per-set line and the end of the report say the model stopped on its quota, and at which run.
3. **Per-minute limits don't stop it.** `rate_limited` (429 with a short wait) is retried as today; only `quota` stops a model.
4. **`-usage-log`:** the stop is one line in the log, so a later reader sees where the requests ended.
5. **README:** one line in `scenarios/README.md`.

## Rules
- The same for every provider and model (SPEC 1). No error text is matched in the runner; it uses the provider layer's kind.
- The app's own behaviour doesn't change.

## Done when
- A Go test with a scripted model that returns `quota` on its 3rd request: the run that hit it ends as now, every later run on that model is skipped, and a second model's runs all go ahead.
- The report and the set lines show the skips.
