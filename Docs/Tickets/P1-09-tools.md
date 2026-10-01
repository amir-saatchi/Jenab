# P1-09 — Tools, web pages and refs
**Type:** Feature
**Status:** Open
**Gate:** 3 (Phase 1)
**Needs:** P1-05, P1-07
**Requirements:** R-22, R-63, N-23, N-33

## Goal
The tool layer and the Phase 1 tools (CODE-OUTLINE 6, SPEC 3.7, 8.1).

## Scope
- **`tool` package:** `Tool`, `Spec`, `Effects`, `Preflighter`, `Call` and `Env`, `Func[A]`, `Registry`.
- **Previews and refs (3.7):** results over `tool_preview_tokens` go to `cache/tool/…`; the preview and stub formats.
- **Tools:** `search_history`, `read_messages`, `read_ref`, `search_ref`, `fetch_page`, `bucket_list`, `bucket_read`, `bucket_put`, `bucket_delete`, `list_chats`.
- **`web` (page fetching only):**
  - the readable-text steps from 3.7 (SPIKE-015), pages stored under `cache/pages/<host>/<hash>.txt`, and `needs_javascript`
  - the network rules from 6.7: http and https only, private addresses blocked after DNS and on every redirect, and the 5 MB cap (6.6)
  - Search and feeds come in Phase 4. `html.extract` comes with pipelines (Phase 4).
- **Untrusted content:** results from pages carry the `Untrusted` effect (N-23).

## Done when
- Each tool has table tests.
- `fetch_page` is tested against a local test server with sample pages (CI doesn't use the internet); SPIKE-015's real pages are a manual check.
- Redirects to `127.0.0.1`, and a name that resolves to a private address, are blocked.
