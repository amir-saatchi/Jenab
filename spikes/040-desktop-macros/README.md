# SPIKE-040 harness: desktop macros and recipes

SPIKE-039's harness (copied, trimmed) plus a macro format, a replayer and a store, so a model can
save what worked and replay it without itself. The ticket is
`Docs/Tickets/SPIKE-040-desktop-macros.md`; the results are in `results.md`.

Its own module, standard library only (no `go.sum`). Build and run from this folder:

```
go build -o out/spike040.exe .
# making: the model does the task once by hand and is told to save a macro and a recipe
out/spike040.exe agent -scratch <dir> -env <.env> -mode macros -nudge -params 0 -tasks editor,calculator,chrome
# replays with other values (sets 1-5), the macro made to look 3 days old before sets 4 and 5
out/spike040.exe agent -scratch <dir> -env <.env> -mode macros -params 1-5 -old 4,5
# Mistral (or any OpenAI-compatible API)
out/spike040.exe agent ... -api openai -base-url https://api.mistral.ai/v1/ -key-name MISTRAL_API_KEY -model ministral-14b-latest -gap 4s
# recorded macros: Jenab keeps the steps that worked, save_macro only names params and logs
out/spike040.exe agent ... -mode macros -record -nudge -params 0 -tasks site,editor,calculator
# a saved macro replayed without a model, and graded
out/spike040.exe replay -scratch <dir> -model <model> -tasks chrome -params 1
# breaks: Calculator in another mode, Chrome's own UI in German, the stand-in site in German or
# without its cookie dialog
out/spike040.exe agent ... -calc-mode "Date calculation" | -chrome-ui de | -site-lang de | -site-consent=false
# recipes: shown but nothing saved (-mode recipes), or SPIKE-039's plain prompt (-mode plain)
out/spike040.exe store out/store-<model>.json list
out/spike040.exe store out/store-<model>.json undo macro <app> <name>
out/spike040.exe regrade out/agent/<run>      # grade Chrome runs again from their logs
```

`-scratch` holds agent-desktop's state and the Chrome profiles. `out/` (run output, the stores)
and `vendor/` are git-ignored.

## Getting agent-desktop

Not committed. From <https://github.com/vercel-labs/agent-desktop/releases/tag/v1.0.2>, into
`vendor/v1.0.2/dl/`, checked against `checksums.txt`:

| File | Size | SHA-256 |
|---|---|---|
| `agent-desktop-v1.0.2-x86_64-pc-windows-msvc.tar.gz` | 1,578,236 | `2186c76e7df197c0e67365e45d36d869f74714c56bb8a73817e2a66f3e18e222` |

Extract it into `vendor/v1.0.2/cli/`. Only `agent-desktop.exe` runs, by its full path.

## What is where

- `macro.go`: the format (`Step`, `Macro`, `Recipe`), the store (a JSON file standing in for the
  `registry.db` tables), Undo, and the checks `save_macro` runs.
- `replay.go`: the replayer. Each step finds its element with `agent-desktop find --role --name
  --exact`, waiting up to 10 s (5 s for an optional step), then runs the same guards as the
  model's own actions (consent, password, key list, window lock; in a browser the site lock and
  the CAPTCHA check after each action). A failure returns the log so far and a snapshot with ids.
- `session.go`: the tools and guards; `agent.go`: the prompt, the skill, the tasks and the loop.
- `record.go`: recorded macros (`-record`): the steps that worked, kept by the session, and the
  `save_macro` that names params and logs.
- `site.go`: the stand-in search site on `http://127.0.0.1:8740/` (task `site`), used after
  Google's second CAPTCHA: a cookie dialog, a search box and results made from the query, in
  English or German.
- `chrome.go`: Chrome with a fresh profile in the scratch folder (`--user-data-dir`,
  `--no-first-run`, `--no-default-browser-check`), its UI in English unless `-chrome-ui`.
- `probe.go`: no-model probes (`find` output; Google's consent language; `PROBE=prep` readies a
  profile without the consent dialog).

## What it opens, and what it never touches

The test editor (its own process), Calculator (a new window; its mode is put back to Standard
when a test changed it) and Chrome with a fresh profile in the scratch folder. Windows are
closed with `WM_CLOSE`, and a process is killed only if the spike started it, by PID. Live
google.com searches only; consent is only ever declined (a guard refuses accept buttons, in
German and English, for the model and for replays); a CAPTCHA stops the run, two stop the batch. `out/captchas.txt` keeps every CAPTCHA, and the
`chrome` task refuses to start once it holds two.
Keys are read from the env file into memory and go only into auth headers.
