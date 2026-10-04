# P1-09 — Tools, web pages and refs
**Type:** Feature
**Status:** Done
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

## Result
- **`tool`:** `Tool`, `Spec` (with `Mother` for Mother-only tools), `Effects`, `Preflighter`, `Call`, `Env`, `Func[A]`, `Run` and `Registry`. Arguments are checked with JSON Schema (`santhosh-tekuri/jsonschema/v6`), and the model gets every problem at once. `Run` turns the tool's own timeout into an error the model can fix; a cancel or the caller's deadline stays as it is.
- **Previews and refs:** `Output` stores a result over `tool_preview_tokens` under `cache/tool/<chat>/<message>-<n>`; `Stub` makes the stub. Offsets are bytes, and the footer gives the next offset.
- **Tools:** `Builtin` returns the ten tools. `list_chats` is in `tool`, not `agent`; the agent gives each chat's status.
- **`web`:** `Client.Fetch`, `Page`, `CheckURL`, `BlockedError`, `StatusError`, `ErrNotPage`. The address check is in the dialer, so it covers DNS answers and every redirect. There is no proxy. Hosts allowed to be private get their own client, closed after the fetch.
- **`store.SearchText`:** finds lines of a stored text the way history search compares words; `search_ref` uses it.
- **New modules:** readeck `go-readability/v2` v2.1.2, `html-to-markdown/v2` v2.5.2, `jsonschema/v6` v6.0.3, `x/net` v0.59.0 (with `x/text` v0.42.0 and `x/sys` v0.48.0).
- **Changes from the plan:**
  - **Charset order:** the BOM comes first, then the header, then `<meta>`, as browsers do; invalid bytes become U+FFFD. The fuzz test found invalid UTF-8 passing through a page with no charset.
  - **`data:` URLs** are removed before the Markdown step: one inlined screenshot on the Cloudflare blog was 680 KB of base64 (686,000 characters of text became 7,600).
  - **Table bounds:** the Markdown converter pads every row to the widest and copies spanned cells, so one `rowspan=999999 colspan=999999` cell asked for 7 GB. A table over 250,000 cells, 1,000 columns, 50,000 spanned cells or 2 MB of copied text becomes plain paragraphs. The fuzz seeds found it.
  - **Redirects:** 10 are followed, the 11th fails; the first version stopped at the 10th.
  - **`For(k)`** has no `loaded` list until SPIKE-028. `Env` has no `Workspace` until the `workspace` package exists.
- **SPIKE-015 check:** the 35 saved pages through `Fetch` from a local server: 95 of 109 main-text phrases (the spike's readeck run: 98; the other 3 are on the 15 MB WHATWG page, which is cut at 5 MB), 5 of 118 boilerplate leaks and 34 of 35 titles, the same as the spike. All six Persian pages, including the windows-1256 copies, keep every phrase. Excalidraw returns `needs_javascript`.
- **Tests:**
  - Table tests for every tool; `fetch_page` against a local server through a fake resolver.
  - Blocked: redirects to `127.0.0.1`, to a name for it and to `file:`; a name for `10.1.2.3`; loopback, private, link-local, unspecified and multicast addresses, also as IPv4-mapped IPv6 (`::ffff:0.0.0.0`); other schemes and user names in URLs.
  - `FuzzReadable` (278,000 runs, no panic, no input over 1 s, always valid UTF-8).
  - Mutation checks: 50 on `web`, `tool` and `SearchText`. 30 made the tests fail at first. 8 missed ones got tests (mapped addresses, the redirect cap, XHTML and `+xml` types, rows widened by rowspans, a stub without a size, `objectError`, the caller's deadline). 5 changed nothing, so that code was removed: our own title fallback (two of them), a keep-alive setting, a nested-table branch and a check in `read_ref`. 7 didn't compile and were reworded; all of those fail the tests.
