# TASK-034 — A fake price site for manual runs
**Type:** Task
**Status:** Done
**Gate:** 3
**Needs:** P2-13

## Goal
In the built app, `fetch_page` reaches the real internet. Manual runs, such as P2-14's, need daily crypto pages that are fixed and checkable, offline and free. The scenarios already have them: `scenarios/bench-bitcoin/pages/` and `scenarios/bench-bitcoin-long/pages/`, 19 days from 5 to 23 October 2026. Serve them on this machine.

## Scope
1. **A small tool**, `cmd/jenab-fakesite`, with no new dependencies. It listens on `127.0.0.1` only, port 8099 by default (`-addr`), and serves:
   - `/` an index page linking every day;
   - `/crypto/<YYYY-MM-DD>` that day's page as HTML, with the Markdown's headings, paragraphs and tables as HTML elements, so the app's page reader gets the same facts as the scenarios;
   - a 404 page for any other day or path.
2. **The pages are the scenario files**, read from the repository at start (`-pages` for other folders). No copies to keep in step: where both sets have a day, the files must be the same, or the tool says which differ and stops.
3. **Logs** each request (time, path, status) so a manual run shows what the agent fetched.
4. **In the app:** the private network block (SPEC 6.7) refuses `127.0.0.1`. Check how a project allows a host, and write down the exact steps. If the project settings have no way to do it yet, say so in the result rather than adding one here.
5. **Docs:** a short section in `Docs/DEVELOPMENT.md` (Main-Session writes it): how to start it, the address to give the agent, and the host exception.

## Not in scope
- Live prices, other coins or days than the fixtures, an API in JSON.
- Changes to the app's fetch rules.

## Done when
- `go run ./cmd/jenab-fakesite` serves all 19 days, and Go tests check the index, a day's HTML (a table cell and a heading), a 404 and the same-file check.
- In the built app, with the host allowed, a chat fetches one day and reads Bitcoin's close from it.

## Result
Done on 2026-11-01. DEVELOPMENT.md has the section.

- **Built:** `cmd/jenab-fakesite`, standard library only: a loopback-only server (`0.0.0.0`, `:8099` and `localhost` are refused), an index, `/crypto/<day>` as HTML, a 404 page, and a log line per request. A small converter turns the pages' headings, paragraphs, tables, lists, bold and code into escaped HTML.
- **Same files:** the 12 days in both sets are the same, CRLF and LF counted alike; a difference names the files and stops the tool.
- **Checked:** Go tests for the 19 days, the index, a day's headings and Bitcoin's row, 404s, the same-file check, the loopback rule and escaping. A round trip through the app's page reader (`web.Client.Fetch`) gives each page's text exactly, minus its title, for all 19 days, and is blocked without the exception.
- **Not done:** the in-app check. The project settings can't add a private-network exception: `project.SetPrivateHosts` exists but only tests call it, and nothing in the UI does. Until a setting exists, the exception can only be written into the project's `_jenab_meta` (`private_hosts`) with the app closed. TASK-036 adds the setting.
