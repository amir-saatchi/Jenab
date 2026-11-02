# SPIKE-039 harness: desktop control with agent-desktop

Runs agent-desktop 1.0.2 (Vercel Labs, Apache-2.0) both ways Jenab could run it on Windows, as a
child process and as a DLL loaded without cgo, and gives `gemini-3.5-flash-lite` a few desktop
tools locked to one window. The ticket is `Docs/Tickets/SPIKE-039-desktop-control.md`; the
results are in `results.md`.

Its own module, standard library only (no `go.sum`). Build and run from this folder:

```
go build -o out/spike039.exe .
out/spike039.exe bench  -scratch <dir>                    # CLI vs DLL times, which window each reaches
out/spike039.exe sizes  -scratch <dir> -env ../../.env    # snapshot sizes and tokens, 4 apps
out/spike039.exe hang   -scratch <dir>                    # a frozen window; Edge at a cold start
out/spike039.exe agent  -scratch <dir> -env ../../.env -tasks editor,calculator -reps 3
out/spike039.exe agent  -scratch <dir> -env ../../.env -api openai -base-url https://api.mistral.ai/v1/ \
                        -key-name MISTRAL_API_KEY -model ministral-14b-latest   # any OpenAI-compatible API
out/spike039.exe agent  -scratch <dir> -env ../../.env -tasks chrome -reps 3   # a live Google search
```

`-scratch` holds agent-desktop's state, the Edge profile and the test folder. `out/` (run output)
and `vendor/` are git-ignored.

## Getting agent-desktop

Not committed. From <https://github.com/vercel-labs/agent-desktop/releases/tag/v1.0.2>, into
`vendor/v1.0.2/dl/`, checked against `checksums.txt` (and GitHub's digests):

| File | Size | SHA-256 |
|---|---|---|
| `agent-desktop-v1.0.2-x86_64-pc-windows-msvc.tar.gz` | 1,578,236 | `2186c76e7df197c0e67365e45d36d869f74714c56bb8a73817e2a66f3e18e222` |
| `agent-desktop-ffi-v1.0.2-x86_64-pc-windows-msvc.zip` | 1,391,475 | `baf08bdca22cced212f258455cad9835b06678a3e1969a3b21cb501c377d35dc` |

Extract the first into `vendor/v1.0.2/cli/` and the second into `vendor/v1.0.2/ffi/`. Nothing
from the archives runs except `agent-desktop.exe`, by its full path, and the DLL, loaded by path.

## What it opens, and what it never touches

- **The test editor** (`spike039 editor`): a plain Win32 window with a menu, a multi-line Edit
  box, two buttons and a character count. It stands in for Notepad, which on this machine would
  reopen the user's saved tabs. `-freeze-after`/`-freeze-for` block its UI thread once.
- **Calculator**, **one File Explorer window** on a test folder in the scratch dir, and **Edge**
  with a fresh profile in the scratch dir on a page served from `127.0.0.1` (a `file://` URL
  would put the user's profile path into every snapshot).
- Windows are found by the process the spike started or by an id it did not see before the
  launch, closed with `WM_CLOSE` to that window, and a process is killed only if the spike
  started it. `explorer.exe` is never killed.
- Explorer's navigation pane shows the user's own folders, so its snapshot text is never sent to
  a model: its tokens are estimated from the other apps' ratio.

## Notes

- **State folder:** agent-desktop refuses a state folder (`AGENT_DESKTOP_HOME`) that inherits
  access for other principals ("private file DACL grants an untrusted principal"). It sets a
  private DACL when it creates the folder itself, so point it at one that does not exist yet.
- **Keys:** `GEMINI_API_KEY`, or the one `-key-name` names for `-api openai`, read into memory
  from `-env`, never printed. Gemini requests are counted per Pacific day in
  `out/requests-<date>.txt` (the spike stops at 380); others per UTC day in
  `out/requests-<host>-<date>.txt`, spaced by `-gap` (3 s).
- **DLL:** `dll.go` mirrors the structs of `agent_desktop.h` (ABI 6, x64) and checks their sizes
  against the library's `ad_*_size` at load. Every call runs on one locked OS thread.

`out/spike039.exe calcprobe -scratch <dir> [-button "Keep on top"] [-reps 2]` starts Calculator,
clicks one button (Four by default) and watches the window, its owner and list-windows for 65 s
(no model calls).

`out/spike039.exe chromeprobe -scratch <dir>` starts Chrome with a fresh profile in the scratch
dir, searches Google once from the address bar, clicks "Reject all" if asked and saves full
snapshots to `out/chrome-probe` (no model calls). The `chrome` task searches live on google.com:
run it only when that is allowed. It never clicks accept (a guard refuses it for every model)
and stops at a CAPTCHA.
