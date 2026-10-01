# SPIKE-019 — Results

Wails v3.0.0-beta.26 `pkg/updater` tested end to end on Windows 11 Pro 26200 (Go 1.26, `CGO_ENABLED=0`, 14.6 MB exe). Defender was on and left unchanged: real-time, on-access, IOAV and behaviour monitoring were all enabled, AMProductVersion 4.18.26080.4, read-only query. The feed ran in-process on 127.0.0.1. The install was a plain folder copy to `%LOCALAPPDATA%\Programs\BurrowSpike019\`.

Raw data:
- `out/summary.md` is the full run (2026-09-28, reps 6).
- `out-extra1/` repeats b02/b12 four times each.
- `out-extra2/` is c9.

These folders are gitignored, so re-run to regenerate them (see README).

Legend: **P** = works as needed, **F** = fails or needs our own code, **~** = works with a caveat.

## Summary

| Q | Verdict | One line |
|---|---|---|
| 1 Feed | P | The endpoint and github providers both work against localhost. Plain http is accepted for any host. https with our own CA works through `HTTPClient`. |
| 2 Verification | ~ | The rejections are correct and the old version keeps running. But verification is **fail-open**: a release with no signature, or with nothing at all, is installed even with a key built in. We need a provider wrapper (tested, works). |
| 3 Swap | ~ | No UAC. Downtime is **~250 ms to the new process and ~1 s to page ready**. A lock without FILE_SHARE_DELETE held for more than ~10 s leaves the app **not running** (helper exit 14). A helper killed mid-swap never broke the install. |
| 4 Safe moment | P | Restart is ours to call, so deferring while busy is trivial. The scheduler catch-up after restart works. |
| 5 Data | P | Backup + one-transaction migration works. A failed migration rolls back. The old version refuses a newer db without touching it. |
| 6 Child processes | P | Helper detection is env-only, so a `--child` process is never taken for the helper. A plain child survives the update on the old image. A kill-on-close job child dies with the parent. |
| 7 UI / privacy | ~ | The Go and JS events are enough for our own UI. The check sends only arch/platform/version + Go UA. Mode off sends 0 requests. **`CheckInterval` polling re-downloads the whole artifact every tick.** |

## Q1 — Feed

| Case | Result |
|---|---|
| a1 endpoint, `http://127.0.0.1` | 1.0.0 → 1.0.1 installed and relaunched |
| a2 github provider, BaseURL = fake API, `SHA256SUMS` asset, download via 302 to another host name | 1.0.0 → 1.0.1 installed and relaunched |
| a3 endpoint https, self-signed cert trusted only via `HTTPClient` (own `RootCAs`) | staged OK |
| a4 same without the CA | check error: `tls: failed to verify certificate: x509: certificate signed by unknown authority` (stage `check`) |
| a5 github, no sums asset | **accepted and staged unverified** |
| a6 github, wrong digest in SHA256SUMS | `updater: digest mismatch` (stage `verify`) |
| a7 github + our `requireSig` wrapper | rejected at check: `burrow: release 1.0.1 is not signed (ed25519 signature required)`. The github provider cannot carry signatures at all. |
| a8 download throttled to 5 MB/s | 29 progress events over about 3 s (~10/s), seen in both Go and JS |

Plain http is accepted by both providers, for any host. Nothing enforces https.

## Q2 — Verification (endpoint, sha256 + ed25519 over the sha256 digest)

In every rejected case:
- the error event carries `{stage, message, provider}`;
- the app kept running (db query OK) and the installed exe hash was unchanged.

| Case | Outcome | Exact error / event |
|---|---|---|
| b01 good signature | staged | — |
| b02 bit flip in artifact | rejected | `updater: digest mismatch` (verify) |
| b03 signed with the wrong key | rejected | `updater: ed25519 signature did not verify` (verify) |
| b04 digest only, key built in | **accepted** | — (fail-open) |
| b05 = b04 + `requireSig` wrapper | rejected | `burrow: release 1.0.1 is not signed …` (check) |
| b06 no digest, no signature | **accepted** | — |
| b07 signature, no key built in | rejected | `updater: signature requires a public key but none configured` (verify) |
| b08 manifest digest ≠ file | rejected | `updater: digest mismatch` (verify) |
| b09 same version | no update | `wails:updater:no-update`, Check returns nil |
| b10 older version (0.9.0) | no update | `wails:updater:no-update` |
| b11 truncated file (served complete, short) | rejected | `updater: digest mismatch` (verify) |
| b12 connection cut mid-download | rejected | `unexpected EOF` (stage `download`) |
| b13 only a linux/darwin artifact | rejected | `endpoint: manifest 1.0.1 has no artifact for windows/amd64` (check) |
| b14 signature but no `signatureAlgo` | rejected | `artifact has a signature but no signatureAlgo` |
| b15 manifest `schemaVersion: 2` | rejected | `manifest schemaVersion 2 is newer than supported version 1` (check) |
| b16 `1.0.1-rc.1` | **accepted as newer** | semver prerelease > 1.0.0; no channel filter |
| b17 signature only, no digest | accepted | the signature is verified over the digest computed while downloading |
| b18 wrong `size` field | accepted | `size` is not enforced (only used for progress) |
| b19 truncated, no verification fields | **accepted** | a broken exe would be installed |

**Bug found (b12).** A failed download leaves `%TEMP%\wails-update-*\.artifact` behind. This happened 5 of 5 times (7.3 MB each); verify failures (b02) left nothing, 0 of 5. The cause is in `download.go`:
- `download()` has named results `(path, dir string, err error)`;
- every error path does `return "", "", err`, which sets `dir = ""` before the deferred `os.RemoveAll(dir)` runs.

This is not Defender. A standalone truncated-download-then-delete test removed the file every time.

## Q3 — Swap

### Downtime (ms)

Measured from the old process exit to the new version's `main()`, to `ApplicationStarted`, and to the page `ready` event. Six reps per variant.

| Variant | exit→main median / max | exit→ApplicationStarted | exit→page ready | Restart()→old exit | Restart()→helper main |
|---|---|---|---|---|---|
| same binary each time | 242 / 270 | 285 / 319 | 945 / 1062 | 126 / 141 | ~65 |
| never-seen binary each time (fresh hash for Defender) | 257 / 320 | 301 / 375 | 981 / 1092 | 123 / 146 | ~65 |
| 270 MB exe (padded) | 1517 | 1666 | 2535 | 227 | 108 |

- No UAC prompt in any run (unsigned exe, per-user folder). SmartScreen was never involved, because the files carried no Mark-of-the-Web.
- The fresh hashes cost about +15 ms median, so no measurable Defender cost.
- About 200 ms of the gap is the helper's full **backup copy + fsync** of the old exe, so it scales with exe size.

### Locks and kills

| Case | Helper exit | Relaunch | Install dir after | Next start |
|---|---|---|---|---|
| c2 exe opened **without** FILE_SHARE_DELETE, held 20 s | **14** | **none, the app stays down** | exe = 1.0.0 intact, `.bak` = 1.0.0; staged file leaked in %TEMP% | c2b: 1.0.0 starts OK |
| c3 same lock, 3 s | 0 | 1.0.1 | exe = 1.0.1, `.old.<n>` = 1.0.0 | — (exit→main 3269 ms, page 3898 ms; swap on attempt 7) |
| c4 lock **with** FILE_SHARE_DELETE, 20 s | 0 | 1.0.1 | exe = 1.0.1, `.old.<n>` | — (normal: 366 / 1096 ms) |
| c5 helper killed while the parent is still in OnShutdown | killed | none | unchanged 1.0.0; staged file leaked | 1.0.0 OK |
| c6 helper killed 0 ms after parent exit (270 MB exe) | killed | none | unchanged | 1.0.0 OK |
| c6 killed at 100 ms | killed | none | exe intact + partial `.bak` (21 MB) | 1.0.0 OK |
| c6 killed at 250 ms | killed | none | exe intact + partial `.bak` (121 MB) | 1.0.0 OK |
| c6 killed at 500 / 1000 ms | killed | none | exe intact + full `.bak` (killed during fsync / before the rename) | 1.0.0 OK |
| c6 killed at 2000 ms | (already done) | 1.0.1 | exe = 1.0.1, `.old.<n>` = padded 1.0.0 | — |
| c7 `HelperReadyTimeout` 1 ms | helper exits | — | unchanged | Restart returns `updater: helper did not signal readiness`; the **old app keeps running** (probe OK) |
| c9 OnShutdown takes 35 s (helper waits 30 s for the parent) | **17** | **none**; the old app exits 5 s later and nothing is running | unchanged; staged file leaked | 1.0.0 OK |
| c8 chain 1.0.0→1.0.1→1.0.2 | 0, 0 | yes | 1.0.2 + one `.old` (1.0.1). The 1.0.0 `.old` was swept by the second update. | — |

Notes:
- Rollback is always possible by hand. Either the target is untouched, or `.bak` / `.old.<n>` holds the previous version.
- The only window where the target is missing is the few microseconds between the rename-aside and the rename-in. It was not hit.
- Exit 14 in c2 is a design flaw. The swap never touched the target, but `restoreFromBackup` first tries `RemoveAll(target)`, hits the same lock and gives up without relaunching. A working 1.0.0 was sitting there.
- Exit 17 is similar: nobody relaunches the app.
- Burrow needs to cover this itself. Options: a watchdog or "next start" check, keeping OnShutdown well under 30 s, or retrying the relaunch.
- Defender: no quarantine, no prompts, no delays. We could not observe whether it held handles on the files; nothing it did caused a failure.

## Q4 — Safe moment

- **d1:** a 4 s busy pipeline was running when the update was staged. The app logged `restart-deferred`, the pipeline ended at +3.8 s, then `idle-now` and Restart. The updater never restarts on its own when `Window: WindowNone` is set and we call `DownloadAndInstall` + `Restart` ourselves.
- **Catch-up:** the job was due at restart + 300 ms, which falls inside the downtime. It ran in 1.0.1 at start with trigger `catch_up`, 97 ms late, exactly once.
- **d2:** a job due at +6 s ran in 1.0.1 with trigger `schedule`, 385 ms late (1 s tick).

## Q5 — Data (modernc.org/sqlite v1.59.0, `PRAGMA user_version`)

| Case | Result |
|---|---|
| a1 1.0.1 starts on a v1 db | `VACUUM INTO backups/app-v1-<ts>.db` (8 KB, 15–18 ms), then migration in one transaction (2–5 ms). `user_version` = 2, integrity ok. |
| e1 1.0.0 starts on a v2 db | exits with code 3. Message: `app.db has schema version 2, but Burrow 1.0.0 only understands up to 1. It was not opened or changed. Install the newer Burrow again or restore a backup from …\backups`. db sha unchanged, integrity ok. |
| e3 migration fails before COMMIT | rolled back: `user_version` still 1, columns unchanged, backup present |
| e4 retry after e3 | migrates OK. Backups pile up (one per attempt), so pruning is our job. |

## Q6 — Child processes (same exe, `--child`)

| Case | Result |
|---|---|
| f-child-plain | The swap succeeded on attempt 1 while the child ran from the old image. The child kept beating as **1.0.0 for 11.6 s** after the parent exited. `os.Executable()` in the child returns the original path, which now holds 1.0.1. The child keeps `.old.<n>` open until it exits. |
| f-child-job (Job Object, KILL_ON_JOB_CLOSE) | The child was killed when the parent exited (0 beats after exit). The swap was unaffected. |
| helper detection | The helper runs with args `[]` and `WAILS_UPDATER_HELPER=1`. Children have `--child` and no helper env, so there is no interference. |

`HandleHelperMode` runs inside `application.New`. Anything in `main()` before that call also runs in the helper process: db open, scheduler, logging. **Call `updater.HandleHelperMode()` as the first line of `main()`**, as the spike does.

## Q7 — Events, privacy, disabling

- **Events:** Go and JS both receive every event; JS gets them about 5 ms later. Events are `check-started`, `update-available`, `no-update`, `download-started`, `download-progress` {written, total, rate}, `download-complete`, `verifying`, `installing`, `update-ready` and `error` {stage, message, provider}. That is enough to drive our own UI with `Window: WindowNone`.
- **What a check sends (endpoint):** `GET /m/manifest.json?arch=amd64&platform=windows&version=1.0.0` with `Accept: application/json`, `Accept-Encoding: gzip` and `User-Agent: Go-http-client/1.1`. No machine ID, no cookies. The download uses `Accept: application/octet-stream`.
- **What a check sends (github):**
  - Headers: `Accept: application/vnd.github+json`, `X-Github-Api-Version: 2022-11-28`, and `Authorization: Bearer <token>` if a token is set.
  - The token goes to the API, sums and asset URLs on the same host. It is stripped on the cross-host redirect, but `Referer` and `X-Github-Api-Version` still go to the CDN host.

| Mode | Requests |
|---|---|
| g1 off (updater not initialised) | 0 in 8 s |
| g2 one Check, `CheckInterval: 0` | exactly 1 in 8 s |
| g3 `CheckInterval: 2s` | 6 in 7 s: manifest **and a full 14.6 MB download on every tick**, even though the update was already staged/Ready |

Leaving `CheckInterval` at 0 disables all background traffic. Our own scheduler should call `Check` and then decide.

## From the source (beta.26)

- **Staging:**
  - `os.MkdirTemp("", "wails-update-*")` in `%TEMP%`: first `.artifact`, then renamed to the artifact filename.
  - Removed only by the next `DownloadAndInstall` in the same process or by the helper after a successful swap.
  - Every exit without Restart leaks the staged exe: stage-only mode, rejected Restart, helper exit 13/14/17.
  - The helper log `%TEMP%\wails-update-<parentPID>.log` is never deleted.
- **Old binary:** the helper copies the target to `<exe>.bak` (full copy + fsync), then renames the target to `<exe>.old.<nanos>`, then renames the new exe in. It falls back to copy across volumes and makes up to 20 attempts, 500 ms apart. `.bak` is deleted after a good launch. `.old.<n>` stays until the next update sweeps it.
- **Relaunch:** no args. The environment and working directory are inherited, with the helper env removed.
- **Exit codes:**

  | Code | Meaning |
  |---|---|
  | 10 / 11 | stat failed |
  | 12 | backup failed (the original is relaunched) |
  | 13 | swap failed, backup restored and relaunched |
  | 14 | restore failed, no relaunch |
  | 15 / 16 | launch of the new version failed (restored / not restored) |
  | 17 | parent still alive after 30 s, no relaunch |

- **Semver:** `golang.org/x/mod/semver`. "Newer" is strictly greater. Same or older gives `no-update`, so a downgrade needs a manual install. Prereleases count as newer than the previous release. The comparison is done inside each provider, not centrally.
- **Restart when the helper can't start:** it returns an error (`helper did not signal readiness` or a spawn error) and does **not** quit, so the app keeps running. On success, `Restart` → `Quit` is synchronous: OnShutdown runs before `Restart` returns.
- **Spawn:** the helper is started with DETACHED_PROCESS | CREATE_NO_WINDOW | CREATE_NEW_PROCESS_GROUP | CREATE_BREAKAWAY_FROM_JOB. Breakaway is dropped when the job doesn't allow it.

## Surprises

1. Verification is fail-open. A digest-only release, or one with no verification at all, is installed even with a public key built in. The github provider cannot carry signatures, and without a sums file it installs unverified.
2. A lock without FILE_SHARE_DELETE held for more than ~10 s (for example a scanner or backup tool), or an OnShutdown longer than 30 s, leaves **nothing running**. The helper has no "just relaunch the untouched old exe" path.
3. `CheckInterval` re-downloads the full artifact on every tick, even when an update is already staged.
4. A failed download leaks the partial file (the named-return bug in `download.go`). Staged files leak on every path without a swap.
5. The `Config.Window` comment says "not yet wired", but it is wired: a nil Window opens the builtin window from `CheckAndInstall` and periodic checks. Set `WindowNone` explicitly.
6. The helper runs our `main()` up to `application.New`.
7. Prereleases are offered to stable users. There is no channel concept.

## Not tested

- The microsecond window between the rename-aside and the rename-in (target missing). A crash there would leave no exe, but `.bak` would still be present.
- Real GitHub or a real CDN. Only the local fake API and redirect were used.
- `%TEMP%` on a different volume from the install dir (the copy fallback).
- The builtin update window (`BuiltinWindow`) and BYOWindow.
- A failure to spawn the helper process itself. Only the readiness timeout was tested.
- A code-signed exe, SmartScreen with Mark-of-the-Web, and an MSI or other installer install.
- Sleep or hibernate during the swap. A machine without a WebView2 runtime.
