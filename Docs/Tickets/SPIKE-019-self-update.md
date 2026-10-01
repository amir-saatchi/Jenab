# SPIKE-019 — Self-update with the Wails updater
**Type:** Spike
**Status:** Done
**Gate:** 2

## Question
Can Jenab update itself safely with the updater in Wails v3 beta.26 (`pkg/updater`), tested fully on this machine without GitHub?

1. **Feed:**
   - The endpoint provider reads a manifest served by a local HTTP server on `localhost`.
   - The GitHub provider is pointed (`BaseURL`) at a local fake of the releases API.
2. **Verification:**
   - SHA-256 plus an ed25519 signature, checked against a public key built into the app.
   - These must be rejected: a corrupted download, a wrong signature, a missing signature, and the same or an older version.
   - After a rejection, the running version keeps working.
3. **Swap:**
   - Install per user (`%LOCALAPPDATA%\Programs\Jenab`), so there is no admin prompt.
   - Measure how long the app is down between exit and relaunch.
   - Find out what happens when the swap fails (file locked, antivirus scan, power loss), and whether the old version can be restored.
4. **Safe moment:**
   - Restart only when no pipeline run, migration or write request is in progress.
   - After the restart, missed scheduled runs are caught up (SPEC 6.2).
5. **Data:**
   - Every `.db` has a schema version.
   - A new version backs up and migrates forward on first start.
   - An older version refuses to open a newer database.
6. **Child processes:** the updater's helper mode and the Starlark child (SPIKE-016) use the same exe. Does an update work while a child is running?
7. **UI and privacy:**
   - Our own React UI, driven by the updater's events (the built-in window is not wired in beta.26).
   - A setting with three choices: automatic, notify only, off.
   - What the update check sends.

**Not in scope:**
- OS code signing (Authenticode, Apple notarization)
- real GitHub releases (download redirects, API rate limits); to be checked once the public repository exists
- winget and Homebrew builds
- macOS

## Done when
A test app, version 1.0.0, updates itself to 1.0.1 from a local feed and rejects every bad release. There is a decision on the install location, the signing key setup and data versioning.

## Result
Run on 2026-09-28 with Wails v3.0.0-beta.26 `pkg/updater` on Windows 11 Pro 26200: Go 1.26.0, `CGO_ENABLED=0`, a 14.6 MB exe, Defender on and unchanged. The feed ran on 127.0.0.1, and the install was a folder copy into `%LOCALAPPDATA%\Programs\JenabSpike019`. Code and full output: `spikes/019-self-update/` (`results.md`). Build output is git-ignored and rebuilt by the driver.

| Question | Verdict | Result |
|---|---|---|
| 1. Feed | pass | The endpoint provider and the GitHub provider (a fake API, `SHA256SUMS`, a redirect to another host) both updated 1.0.0 to 1.0.1. Plain http is accepted for any host. https with our own certificate works when it is trusted only through `HTTPClient`. |
| 2. Verification | **partial** | Every bad release below was rejected, and the old version kept running unchanged. But verification is **fail-open**: with a key built in, a release with only a digest, or with no checks at all, still installs. The GitHub provider can't carry signatures, and without a sums file it installs unverified. |
| 3. Swap | **partial** | No UAC prompt. Downtime is **257 / 320 ms** (median / max) from the old exit to the new `main()`, and **981 / 1,092 ms** to page ready. Defender added nothing measurable. See the failure table below. |
| 4. Safe moment | pass | With `WindowNone` we call `Restart` ourselves, so "busy" simply waits. A job due during the downtime ran once in 1.0.1, as a catch-up 97 ms late. |
| 5. Data | pass | 1.0.1 made a backup with `VACUUM INTO` (15–18 ms) and migrated in one transaction (2–5 ms), raising `user_version` to 2. A failure injected before commit rolled back fully. 1.0.0 on the newer database exited with a clear message, and the file was unchanged. |
| 6. Child processes | pass | Helper mode is detected from an environment variable only, so our `--child` flag is safe. A child in a kill-on-close Job Object dies with the parent. A plain child keeps running the old image and holds the `.old` file open. |
| 7. UI and privacy | **partial** | Go and JS both receive every event (JS about 5 ms later), enough for our own UI. A check sends only `platform`, `arch`, `version`, the Go user agent and Accept headers. "Off" sends 0 requests. **With `CheckInterval` set, every tick re-downloads the whole release,** even when one is already staged. |

**Rejected releases** (error event `{stage, message, provider}`)

| Case | Error |
|---|---|
| bit flip, digest mismatch, truncated file | `updater: digest mismatch` |
| signed with another key | `updater: ed25519 signature did not verify` |
| signature but no key in the app | `updater: signature requires a public key but none configured` |
| signature without `signatureAlgo` | rejected |
| connection cut | `unexpected EOF` (download stage) |
| no artifact for windows/amd64 | `endpoint: manifest 1.0.1 has no artifact for windows/amd64` |
| manifest `schemaVersion` 2 | rejected as too new |
| same or older version | `no-update` event |

- **Accepted, but they should not be:**
  - digest only
  - no verification fields at all
  - a truncated file with no verification fields, which would install a broken exe
- **Accepted, and needing a rule:**
  - `1.0.1-rc.1` counts as newer, since there are no release channels
  - the `size` field is not checked
- A small wrapper around the provider that requires a signature closes the fail-open cases (tested).

**Swap failures**

| Case | Outcome |
|---|---|
| exe opened by another process that allows delete | no effect |
| exe opened without delete sharing, held 3 s | swap succeeded on attempt 7 (3.3 s to `main()`) |
| same lock, held 20 s | the helper gave up after about 10 s with exit 14. **Nothing was relaunched,** although the old exe was intact. |
| `OnShutdown` longer than 30 s | the helper exits with 17 and **nothing is relaunched** |
| helper killed at 0–1,000 ms, or during `OnShutdown` | the exe stayed intact, sometimes next to a partial `.bak`. The next start always worked. |
| helper not ready in time | `Restart` returns an error, and the old app keeps running |
| 1.0.0 → 1.0.1 → 1.0.2 | works; one `.old.<n>` (the previous version) is kept until the next update |

**Also from the source**
- **Cleanup bug in `download.go`:** `download()` has named results, and `return "", "", err` clears `dir` before the deferred `os.RemoveAll(dir)` runs. So every failed download leaves `%TEMP%\wails-update-*\.artifact` behind (5 of 5 runs).
- **Other leftovers in `%TEMP%`:** a staged exe on every path that doesn't swap, and the helper log `wails-update-<pid>.log`, which is never deleted.
- **Backup copy:** the helper copies the whole old exe before swapping, so downtime grows with exe size. A 270 MB exe took 1.5 s to `main()`.
- **The helper runs our `main()` up to `application.New`,** so nothing heavy may run before it.
- **The `Config.Window` comment says "not yet wired", but it is:** a nil `Window` opens the built-in window.
- **Not tested:**
  - real GitHub or a CDN
  - `%TEMP%` on another drive
  - failing to start the helper itself
  - code-signed exes, SmartScreen and installers
  - sleep during the swap
  - macOS

## Decision
**Decided (2026-09-28):**
- **Updater:** Wails `pkg/updater` with the **endpoint provider**. It reads our own manifest, published next to the release files. The GitHub provider is not used, because it can't carry signatures.
- **Signing:** releases are signed with an ed25519 key kept offline, and only its public key is built into the app. Jenab wraps the provider so it **fails closed**:
  - it requires a signature
  - it requires https (except `localhost` in development)
  - it ignores prerelease versions unless the user picks the beta channel
- **Checks:** Jenab runs its own timer (at start, then every 6 hours) and never uses `CheckInterval`. It downloads each version once and uses `WindowNone` with its own React UI. Settings: automatic, notify only, off.
- **Restart:**
  - Only when idle: no run, migration or write in progress.
  - `OnShutdown` has a 10-second deadline, well under the helper's 30 s.
  - `main()` does nothing heavy before `application.New`.
- **Watchdog:** before `Restart`, Jenab starts a small detached watchdog (the same exe). If no instance is running after 60 s, it relaunches the installed exe. This covers the helper's exits 14 and 17.
- **Cleanup on start:**
  - delete `%TEMP%\wails-update-*` and the helper logs
  - delete older `.old.*` files
  - keep only the last 2 pre-update database backups
- **Install location:** per user, in `%LOCALAPPDATA%\Programs\Jenab`.
- **Data:**
  - Every `.db` stores its storage format version in `PRAGMA user_version`.
  - A new version backs up with `VACUUM INTO` and migrates in one transaction.
  - An older version refuses a newer database and asks the user to update.
- **Children:** Starlark children stay in a kill-on-close Job Object (SPIKE-016), so none survive an update.
- **Upstream:** reported to Wails on 2026-09-28: [#6185](https://github.com/wailsapp/wails/issues/6185) (download cleanup), [#6186](https://github.com/wailsapp/wails/issues/6186) (fail-open verification), [#6187](https://github.com/wailsapp/wails/issues/6187) (no relaunch on exits 14 and 17), [#6188](https://github.com/wailsapp/wails/issues/6188) (`CheckInterval` re-download). We don't wait for fixes.
- **Later (Phase 5):** code signing (Authenticode, Apple notarization), SmartScreen, a check against the real release host, and macOS.
