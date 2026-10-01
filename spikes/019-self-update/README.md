# SPIKE-019 — Self-update with the Wails v3 updater

Throwaway code for [SPIKE-019](../../Docs/Tickets/SPIKE-019-self-update.md). Results: [results.md](results.md).

```powershell
cd spikes\019-self-update
go run ./driver            # about 10 minutes, unattended; small windows open and close by themselves
go run ./driver -only '^b' # only the verification cases (regexp on the scenario name)
go run ./driver -reps 6    # downtime repetitions per variant (default 6)
go run ./driver -nobuild -out out-extra1 -repeat 4 -only '^(b02-bit-flip|b12-connection-cut)$'
go run ./driver -nobuild -out out-extra2 -only '^(c9-shutdown-slower-than-30s|c9b-next-start)$'
```

`-nobuild` reuses `bin/`. `-out` picks the output folder, which is wiped at start. `-repeat N` runs the selection N times, suffixing the names with `-iN`. The last two lines are the supplemental runs cited in results.md. c9 takes about 45 s because the helper waits 30 s for the parent.

Needs Go 1.26 and the WebView2 runtime. No cgo (`CGO_ENABLED=0`), no Wails CLI, no npm/bun, no admin rights, no network except the Go module proxy. Don't use the machine meanwhile if you want clean downtime numbers.

What the driver does:
1. Makes two ed25519 test key pairs in `keys/` if missing (`release-ed25519.key` and `attacker-ed25519.key`, gitignored; `release-ed25519.pub` is the key built into the app).
2. Builds the test app into `bin/` with `-ldflags "-X main.version=… -X main.pubKeyB64=…"`: 1.0.0, 1.0.1, 1.0.2, 1.0.0 without a key, six 1.0.1 builds that differ only in a nonce (never-seen hashes for Defender), and 1.0.0 with 256 MiB of random bytes appended (slow backup copy, for killing the helper mid-swap).
3. Starts the feed in-process on `127.0.0.1` (random ports): endpoint manifests, a fake of the GitHub releases API with a 302 to another host name for downloads, and the same on https with a self-signed certificate that only the app's injected `HTTPClient` trusts (nothing is installed in a certificate store). Every request is logged with URL, query and all headers.
4. For each scenario: copies a build to `%LOCALAPPDATA%\Programs\BurrowSpike019\BurrowSpike019.exe` (plain folder copy, no installer, registry or shortcuts), writes a config JSON, starts the app with `SPIKE019_CONFIG` pointing at it, waits for all processes of the scenario to exit and records the log, the install folder, new `%TEMP%\wails-update-*` entries, the updater helper's log and all exit codes.
5. Writes `out/summary.md` (tables), `out/runs.jsonl` (facts per scenario), `out/logs/<scenario>.jsonl` (every event of every process), `out/feed.jsonl` (every request).
6. At the end kills any process of ours that is still alive (by handle, never by name), removes the `%TEMP%\wails-update-*` entries this run created, and deletes the install folder.

## Files

- `app/`: the test app (Wails v3 beta.26, a 360×220 window with a plain HTML page).
  - `main.go`: config, updater setup (`WindowNone`, our own events), the scenario (check → download → verify → wait until idle → `Restart`), the `requireSig` provider wrapper, the "still alive" probe after a rejection.
  - `db.go`: `app.db` with `PRAGMA user_version`; 1.0.0 knows schema 1, 1.0.1/1.0.2 schema 2. A newer schema is refused after a read-only look; an older one is backed up with `VACUUM INTO` and migrated in one transaction.
  - `sched.go`: wall-clock job check against `jobs.json` (every second instead of every minute), catch-up on start.
  - `child.go`: `--child N`, the stand-in for the Starlark child: same exe, a loop with a heartbeat; optionally in a kill-on-close Job Object.
  - `page.html`: subscribes to all `wails:updater:*` events in JS and echoes each one back to Go.
- `internal/feed/`: the local feed server.
- `internal/spike/`: config type and JSONL logger shared by app and driver.
- `driver/`: builds, scenarios (`scenarios.go`), process tracking (`run.go`), summary (`report.go`), and `driver lock …`, the process that holds the installed exe open during the lock tests.

App config fields (`internal/spike/spike.go`): `mode` = `off` / `notify` / `stage` / `automatic` / `poll`, `busyMs`, `child`, `jobOffsetMs`, `shutdownDelayMs`, `helperTimeoutMs`, `requireSignature`, `failMigration`, … The relaunched version reads the same config because the helper passes the environment on (but no command-line arguments).
