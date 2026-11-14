# SPIKE-047 — A headless browser with agent-browser
**Type:** Spike
**Status:** Open
**Gate:** 3
**Needs:** none

## Question
`fetch_page` returns `needs_javascript` for pages that only show their text after JavaScript runs, and v1 doesn't render them (SPEC 3.7). [agent-browser](https://github.com/vercel-labs/agent-browser) (Vercel Labs, Apache-2.0, a native Rust command-line program that drives Chrome over the DevTools protocol) could render them in a hidden browser with its own profile. Can Jenab use it this way, under the network rules of 6.7, and is the gain worth shipping it?

A second, smaller question: can the same program run headless browser tasks (snapshot, click, fill) where desktop control (8.10) can't: in turns the app starts, in pipelines, and on macOS and Linux?

Desktop control stays as it is. It is the only way to use the user's own windows, and agent-browser's attach modes (`--cdp`, `--auto-connect`) are out of scope.

## Scope
Its own module in `spikes/047-headless-browser/`, Windows 11 only.

**Part A: rendering for `fetch_page` (the main question)**
1. **Running it:** the pinned program, checked by its SHA-256 before each start, as for agent-desktop (TASK-035). Check:
   - Does the Windows binary run on its own, without Node?
   - Which browser does it use: Edge, which every Windows 11 machine has (`--executable-path`), the user's installed Chrome, or Chrome for Testing in the installer? Record the size of each option and whether rendering differs.
   - Settings: `--json`; a fresh profile in a private state folder; nothing in the user's home (`~/.agent-browser`: socket folder, config, saved state); no version check (`AGENT_BROWSER_NO_VERSION_CHECK=1`); a short idle timeout; `close` at the end.
   - Afterwards no browser or daemon process is left, also after a call is killed and after Jenab is stopped.
2. **Network rules (6.7):** the browser makes its own requests, past Jenab's HTTP client. Route all of them through a small Go proxy in the spike (`--proxy`) that applies 6.7: host approval, and the private-network check on the resolved address after DNS. Test pages that load each of the following, and check that each is blocked or asks:
   - a sub-resource from `127.0.0.1` and from a LAN address;
   - a public name that resolves to a private address;
   - a redirect to an unapproved host;
   - a WebSocket, an `EventSource` and `sendBeacon`;
   - WebRTC.

   Chrome skips the proxy for loopback by default, so check that `--proxy-bypass-list=<-loopback>` (through `--args`) closes that hole. Also check whether QUIC or DNS prefetch go around the proxy. `--allowed-domains` with the approved hosts is a second layer; record what it adds.
3. **Text:** render the page, take its HTML (`get html`) and run it through the same readable-text path (3.7). Use the JavaScript-only pages from SPIKE-015's corpus (`js-coinmarketcap`, `js-excalidraw`) and about 10 more `needs_javascript` pages. Record for each page:
   - the characters of text recovered, and whether the content is the page's real content;
   - the time with a cold browser and with a warm one;
   - the peak memory and CPU of the browser and daemon. Jenab's minimum is 4 GB.
4. **Safety:** `--content-boundaries` and `--max-output` on; an action policy that denies `eval`, downloads and uploads. The text keeps Jenab's untrusted mark (8.8). What happens at a CAPTCHA or a consent wall: does the text say so, or does it look like a real page?

**Part B: headless tasks (proposal only if Part A is a go)**
5. **A task:** give the reference models snapshot, click, fill and wait tools over agent-browser, on TASK-034's fake price site and SPIKE-040's stand-in search site. Record success, requests, tokens and time, next to SPIKE-040's runs through desktop control.
6. **Macros:** compare agent-browser's saved flows (0.42, experimental) with Jenab's recorded macros (8.11). Can Jenab's recorder write macros for headless sessions too, or would it need their format?
7. **Where it would apply:** turns the app starts, pipeline steps (`browser.render`?), and macOS and Linux. Write down what each would need; don't build them.

## Rules
- Only pages the spike serves itself, the corpus pages and the two local sites. Never the user's Chrome, profile, windows or accounts, and no Google searches.
- Never sign in or use the auth vault; password fields stay refused as in 8.10.
- Ask before downloading agent-browser or a browser. Check the files against their published checksums and record the version and SHA-256.
- No new dependencies in Jenab's `go.mod`.
- Models: `gemini-3.5-flash-lite` (`GEMINI_API_KEY`, under 400 requests a day) and `mistral-large-4-1t-a52b` (`NVIDIA_API_KEY`, paced under 40 a minute), only for Part B. Ask before any other key.

## Done when
- Part A's checks and numbers are recorded, with what failed, including every request that got past the proxy.
- A go or no-go for rendering in `fetch_page`. If go: the browser choice, SPEC text for 3.7 and 6.7, and a build ticket.
- Part B's numbers if Part A is a go, and a short proposal for headless tasks.
