# TASK-048 — The scenario runner stops a model when its quota is gone
**Type:** Task
**Status:** Done
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

## Result
Done on 2026-11-16. No live requests; all checks ran locally on Windows 11.

- **How it stops:** each run's backends are wrapped already, to count requests. The wrapper now also marks the run when a request comes back with the provider layer's `provider.Quota` kind (`errors.As`, no error text). After that run, `RunSet` starts no more runs on its model. Runs already under way, with `-parallel`, end as they do.
- **Across sets:** one `scenario.Quotas` lives for the whole `jenab-scenarios run`, so a model stopped in one set gets nothing in the later sets either.
- **Report:** the runs left are `skip (quota: <provider>/<model>)` and count as skipped. A run has `quota: true` in the JSON when it got the quota. The set line ends with `; stopped on its quota at <scenario> r<rep>`, or `earlier, at <set>, <scenario> r<rep>` in a later set. The Markdown ends with a "Stopped on quota" section. The console also prints one line when the model stops.
- **`-usage-log`:** one line `{"time", "model", "quota_stop": "<set>, <scenario> r<rep>"}` when the model stops. It isn't added to the totals.
- **Not changed:** `rate_limited` and the other kinds are retried, or fail, as before; only `quota` stops a model. The app's own code isn't touched. A quota on any request of the run stops the model, not only on the request the run ended on: the quota belongs to the provider, so later requests would fail the same way.
- **Tests:** `TestQuotaStopsTheModel` (`internal/scenario/quota_test.go`) scripts two models on one provider. m1 answers `quota` on its 3rd request: runs 1 and 2 pass, run 3 ends broke as before, runs 4 and 5 are skipped, and m1 gets 3 requests in all. m2's 5 runs all pass. A second set with the same `Quotas` skips all of m1's runs and runs all of m2's. With the skip turned off, the test fails (m1 gets 5 requests). `TestCountingMarksQuota` checks that only `quota` marks a run. The command's tests check the skip line and the usage-log line.
- **Checks:** `go build ./...`, `go vet ./...` and `go test ./...` pass. In one full `go test ./...`, `TestRunTimeout/in a refresh's map` hit its 2 s limit during setup, under the load of all packages at once, before its first request. That is a timing flake, not this change: it passed 5 of 5 alone and in the package's own run.
