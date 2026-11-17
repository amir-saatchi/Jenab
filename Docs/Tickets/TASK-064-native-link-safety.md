# TASK-064 — Links in Markdown open only safe addresses
**Type:** Task
**Status:** Done
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

## Result
Done. Report: [reports/TASK-064-report.md](reports/TASK-064-report.md).
1. Every `kit.Markdown` now handles its own clicks, by one rule for answers, user messages, files and pages:
   - a `#heading` link scrolls;
   - a link `Own` takes (`jenab://`) goes to the app;
   - any other link opens only if `kit.SafeLink` allows it. That is MyGo's `ui.SafeToOpen`: http, https and mailto.
   - Everything else opens nothing. `OnLink` now gets only safe links.
2. A `#heading` link scrolls to its heading, in the view whose link was clicked only, once. The chat's shared `Markdown` doesn't scroll other rows. MyGo brings the heading into view at the scroll's bottom edge.
3. `Markdown.click` holds the only `c.OpenURL` in `internal/native`. The other places that open something from data are:
   - link bubbles and `LinkText`, which go to `jenab://` links or bucket keys, in the app;
   - the files panel's *Open with the system app*, under the rules of SPEC 4.7.

   Links, toasts, tooltips and notices open no URLs. The report has the table.

Headless tests in light and dark cover:
- https and mailto opening;
- `file:`, `javascript:`, a program's name, a relative path, a `\host` path, a `//host` path and a link with a control character opening nothing;
- a `jenab://` link going to the app;
- a `#` link scrolling.

A live check with the bench build on a scratch profile clicked each unsafe link in an answer. Nothing started, and Jenab kept the focus. The heading link scrolled to its heading. SPEC 4.5 has the rule.
