# TASK-064 — Links in Markdown open only safe addresses
**Type:** Task
**Status:** Open
**Gate:** 3
**Needs:** TASK-061

## Goal
A review of the native app (2026-11-17) found that a link in a model's answer or in a Markdown file opens whatever it points to. `kit.Markdown` (`internal/native/kit/markdown.go`, `prepare`) sets its own `OnLink` when `Own` is set, which turns off MyGo's filter (`linkAllowed`, `ui.SafeToOpen`: http, https and mailto only). A link `Own` declines then goes to `c.OpenURL`, which on Windows ends in `ShellExecuteW`. So `[docs](calc.exe)`, `file:///…`, `\\host\share\x.exe` or `javascript:` opens or runs with one click. The chat (`chat/screen.go`) and the file view (`files/view.go`) both set `Own`. A fetched page can make the model write such a link (prompt injection). The React app showed only safe links.

## Scope
1. **One rule for every link the kit opens:** `jenab://` links go to the app (`Own`), as now. Any other link opens only if `ui.SafeToOpen` allows it (http, https, mailto). Everything else does nothing when clicked, as MyGo's default does.
2. **`#heading` links scroll to the heading,** as MyGo's default does, instead of going to `OpenURL`.
3. **Every other place that opens a URL from data** (link bubbles, `LinkText`, tooltips with *Open*, the files panel, notices) uses the same check. List them in the report.

## Rules
- The same rule for all content: answers, user messages, files and pages.
- No new dependency.

## Done when
- Tests: `https:` and `mailto:` open; `file:`, `javascript:`, a bare program name, a relative path, a `\\host` path, a `//host` path and a link with a control character don't; a `jenab://` link goes to the app; a `#` link scrolls.
- The report lists every call to `OpenURL` in `internal/native` and how it is guarded.
