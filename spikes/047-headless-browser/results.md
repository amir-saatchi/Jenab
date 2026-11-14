# SPIKE-047 results — a headless browser with agent-browser

Windows 11 Pro (26200), 2026-11-14. agent-browser 0.43.0; Chrome for Testing and chrome-headless-shell 157.0.8103.78; installed Chrome and Edge as found on the machine. All measurement runs were started from PowerShell (see README: Edge doesn't start from this session's Git Bash). Raw outputs, with the user name, LAN address and link-local address replaced, are in [raw/](raw/).

"LAN" below is the computer's own private LAN address; the canaries listen there and on loopback, and the proxy never allows them, so **every canary hit is a request that got past the proxy**.

## 1. Running it

### The program

- **Pinned:** `ab.go` checks the SHA-256 before every start, holding the file open with read-only sharing until the process has started (as agent-desktop does). A copy with one byte added was refused: "SHA-256 is 37cb662d…, not ebcef5eb…".
- **No Node:** `agent-browser-win32-x64.exe --version` with `PATH` set to `System32` only printed "agent-browser 0.43.0". It is one 20 MB native program; the npm package is only a wrapper.
- **Settings that keep it out of the home folder:** a trusted config file through `AGENT_BROWSER_CONFIG` (that replaces the search for `~/.agent-browser/config.json` and `./agent-browser.json`), `AGENT_BROWSER_SOCKET_DIR` in the private state folder, `AGENT_BROWSER_NO_VERSION_CHECK=1`, and `TEMP`/`TMP` pointed into the state folder: agent-browser makes its throwaway Chrome profile under `%TEMP%\agent-browser-chrome-<uuid>`, so this is what puts the profile in the private folder. Every `AGENT_BROWSER_*`, proxy and provider variable from Jenab's own environment is removed. The config file sets `json`, `executablePath`, `downloadPath`, `screenshotDir`, `idleTimeout`, `contentBoundaries`, `maxOutput`, `proxy`, `args`, `actionPolicy`, `noWebmcp`, `hideScrollbars`.
- **Nothing in the user's folders:** after every part, no `~/.agent-browser` and nothing new at the top of the home folder, `%LOCALAPPDATA%` or `%APPDATA%`. Empty GUID-named `.tmp` files appeared in `%TEMP%` during the runs, but the same kind of file was there from other days and from two hours before the spike started, so they come from another program; inside the state folder agent-browser's own GUID `.tmp` files disappear with `close`.

### Browsers

All four start under agent-browser (Edge only from a normal Windows environment, see README). Cold *open + get html* of a small page, from PowerShell:

| Browser | Program | Size on disk | Download | Cold open + get html | Processes | Its own traffic |
|---|---|---|---|---|---|---|
| **chrome-headless-shell** 157.0.8103.78 | `chrome-headless-shell.exe` | 282 MB | 126 MB zip | 0.83 s | 8 | none |
| Chrome for Testing 157.0.8103.78 | `chrome.exe` | 442 MB | 210 MB zip | 0.74 s | 16–17 | Google |
| installed Chrome (fresh profile) | `chrome.exe` | 506 MB | — | 0.71 s | 16 | Google, incl. `accounts.google.com` |
| Edge (fresh profile) | `msedge.exe` | 889 MB | — (every Windows 11 has it) | 0.96 s | 20 | Bing, MSN, SmartScreen, Office, Rewards |

Rendering didn't differ: the readable text of all eleven test pages was the same, character for character, in all four (section 3). The differences are memory (3), the browser's own requests (2) and the WebRTC switch (2).

### A running session

- **The state folder** while a session runs: `config.json`, `policy.json`, `downloads/`, `shots/`, `sock/` (`default.port`, `.pid`, `.stream`, `.config`, `.engine`, `.target`, `.version`) and `tmp/` with the throwaway profile.
- **Listening sockets**, all on 127.0.0.1: the daemon (port **50838 in every run**: the port comes from the session name, `default`, not from the socket folder), agent-browser's stream server (a random port; it can inject input, see 4) and Chrome's DevTools port (random, `--remote-debugging-port=0`).
- **Processes:** the daemon plus the browser's processes (13 for Chrome for Testing). Each CLI call is a short-lived process.

### Clean-up

| Case | Result |
|---|---|
| `close` | the whole tree was gone at once; no profile left in `tmp/` |
| a CLI call killed after 2 ms | the daemon and the browser stay; the next call on the session works |
| no `close`, idle timeout 15 s | the tree ended after 15 s |
| "Jenab" (a stand-in process) killed, no job object | the tree outlived it until the idle timeout (16 s) |
| "Jenab" killed, in a job object with kill-on-close | all 15 processes gone within 2 s |

So Jenab needs **both** a kill-on-close job object (the CLI calls inherit it, and so do the daemon and browser it starts; nothing breaks away) and a short idle timeout, and `close` at the end of each fetch.



## 2. Network rules (6.7)

The proxy (`proxy.go`) is a forward HTTP proxy on 127.0.0.1: `CONNECT` for https, absolute URLs for http, upgrade tunnels for WebSocket. For each request it applies host approval (only in the "host approval" variants: the test site and one second test host are approved), then resolves the name and runs Jenab's private-network check (`addr.go`, copied from `internal/web`) on every address, then dials the checked address itself. Where Jenab would ask, it records the question and refuses. The test site `site.spike047.test` is a private-network exception that the proxy maps to 127.0.0.1.

The probe page (`/net`) loads, from loopback, `localhost`, `[::1]`, `0.0.0.0`, the LAN address, `localtest.me` (a public name for 127.0.0.1) and `<LAN>.nip.io` (a public name for the LAN address): images, an iframe, `fetch`, a `fetch` from a blob worker, `sendBeacon`, `EventSource`, two WebSockets, a 302 to loopback and to a name, `dns-prefetch` and `preconnect` to unique public names, and an `RTCPeerConnection` with STUN on the LAN and loopback and TURN over TCP on the LAN.

### What got past the proxy

| Variant | chrome-headless-shell | Chrome for Testing | installed Chrome | Edge |
|---|---|---|---|---|
| V0 no proxy (control) | all 19 probes reached the canaries | all 19 | all 19 | all 19 |
| V1 `--proxy`, no bypass flag | **loopback: images (127.0.0.1, localhost, [::1]), fetch, worker fetch, beacon, WebSocket, EventSource, redirect; STUN to loopback and LAN** | same | same | same |
| V2 `--proxy` + `--proxy-bypass-list=<-loopback>` | **STUN/udp to loopback and LAN** | same | same | same |
| V3 V2 + host approval | **STUN/udp to loopback and LAN** | same | same | same |
| V4a `--allowed-domains` only | none: agent-browser's page script blocks fetch, WebSocket, EventSource, beacon and `RTCPeerConnection` ("blocked while domain filtering is active") | `open` timed out (os error 10060) | timed out | timed out |
| V4b V2 + `--allowed-domains` | none (as V4a) | timed out | timed out | timed out |
| V5 V2 + `--force-webrtc-ip-handling-policy=disable_non_proxied_udp` | none | **STUN to loopback and LAN** (flag ignored) | **same** | **same** |
| V6 V5 + host approval | none | **STUN** | **STUN** | **STUN** |

- **Loopback without `<-loopback>` (V1):** Chrome sends requests for `127.0.0.1`, `localhost` and `[::1]` direct, as documented. `0.0.0.0`, the LAN address and public names that resolve to private addresses did go to the proxy, which asked and refused. With `<-loopback>` (V2 on) every request to loopback went to the proxy.
- **WebRTC** is the one hole the proxy can't close: STUN goes out over UDP straight from the browser, to loopback and to the LAN (V2, V3). TURN over TCP went to the proxy (a `CONNECT` to the LAN address, refused). `--force-webrtc-ip-handling-policy=disable_non_proxied_udp` closes it **only in chrome-headless-shell**; the full browsers take the policy from a profile preference or an enterprise policy and ignore the switch. agent-browser's `--allowed-domains` script blocks `RTCPeerConnection` too, but with the full browsers `open` never returned (below).
- **`--allowed-domains`** works in chrome-headless-shell: fetch, WebSocket, EventSource, beacon and WebRTC throw `SecurityError` in the page, and nothing reached the proxy. It is a page-script and request-interception layer, so it adds a second, independent check, but it doesn't see the resolved address, so it can't replace the proxy's private-network check. With Chrome for Testing, Chrome and Edge, `open` timed out every time (the daemon never answered); I didn't find why. It can't be combined with a profile or "unsafe" browser args, so it doesn't change the flags above.
- **QUIC:** no QUIC session in any proxied run (Chrome doesn't use QUIC through an HTTP proxy). Without the proxy, Chrome for Testing, Chrome and Edge opened QUIC sessions to Google or Microsoft by themselves (V0 net logs: 8 for Chrome, 4 for Edge).
- **DNS prefetch and preconnect** went through the proxy (a `CONNECT` to the unique `p9…` names, which the proxy's lookup didn't find) and never appear in the browser's own resolver.
- **DNS lookups by the browser itself: none that leave the computer.** With the proxy on, the net log still shows a resolver request for every host a page asks for (`HOST_RESOLVER_MANAGER_REQUEST` from `NETWORK_QUALITY_ESTIMATOR`, also for hosts the proxy refused), but each one runs only the task types 4 and 9, Chromium's host-cache and hosts-file lookups, and names not in them end at once with `ERR_NAME_NOT_RESOLVED` (`site.spike047.test`, `localtest.me`, the nip.io name). No DNS task ran. As a test, `--host-resolver-rules="MAP * ~NOTFOUND, EXCLUDE 127.0.0.1"` (V7) changed nothing in what got past the proxy; without the `EXCLUDE` the browser can't reach the proxy itself (`ERR_PROXY_CONNECTION_FAILED`). It isn't needed.
- **Other UDP** in the proxied runs: a UDP "connect" to `[2001:4860:4860::8888]:443` (Chrome's IPv6 reachability check: 25 `IPV6_REACHABILITY_CHECK` events; a UDP connect sends no packet), sockets bound on `0.0.0.0:5353` and `[::]:5353` (mDNS, used for WebRTC's `.local` host candidates; with the WebRTC policy no candidate was made), and UDP connects to the computer's own link-local address. None of it reached a canary. I didn't capture packets, so "sends nothing" for mDNS is not proven.

### The browsers' own requests (V2, through the proxy)

| Browser | Hosts it contacted by itself, with no page asking |
|---|---|
| chrome-headless-shell | **none** |
| Chrome for Testing | www.google.com (new tab page and its `async/newtab_ogb` calls), www.gstatic.com, ogads-pa.clients6.google.com, play.google.com (`log`) |
| installed Chrome (fresh profile) | the same, plus accounts.google.com (`ListAccounts`), clients2.google.com (network time) |
| Edge (fresh profile) | www.bing.com, ntp.msn.com, edge.microsoft.com (incl. network time), nav-/data-/telem-edge.smartscreen.microsoft.com, substrate.office.com, mss.office.com, prod.rewardsplatform.microsoft.com |

With host approval on (V3, V6) all of these became "ask host" and were refused, so in Jenab they'd be cards nobody asked for. `--disable-features=Translate,NewTabPagePreloading,NtpOneGoogleBar,PreconnectToSearch` didn't stop Chrome for Testing's www.google.com calls (earlier probe).

### Top-level navigation (host approval on, `<-loopback>`)

The same in all four browsers. No canary hit in any of them.

| Test | `open` says | `get url` after | Proxy |
|---|---|---|---|
| open a loopback URL | success | the loopback URL | asked (host), refused |
| open a LAN URL | success | the LAN URL | asked, refused |
| open a public name for loopback | success | that URL | asked, refused |
| 302 to loopback | success | the loopback URL | site allowed, then asked, refused |
| 302 to an unapproved https host | `net::ERR_TUNNEL_CONNECTION_FAILED` | `chrome-error://chromewebdata/` | asked (`CONNECT example.com:443`), refused |
| 302 to an approved host | success | the approved host | both allowed |
| script navigates to the LAN | success | the LAN URL | asked, refused |
| script navigates to an unapproved https host | success | `chrome-error://chromewebdata/` | asked, refused |

**A refused http navigation still "succeeds"**: the page is the proxy's 403 page, under the refused URL. Jenab must take the outcome of the main-frame request from its own proxy, not from agent-browser's `success`.

### `read <url>` goes around the proxy

`read` fetches the URL from the agent-browser process itself, not the browser, and ignores `--proxy`: `read http://127.0.0.1:<port>/r1-read-loopback` **reached the loopback canary four times** (`/r1-read-loopback`, `/r1-read-loopback.md`, `/r1-read-loopback/llms.txt`, `/llms.txt`), and `read` of the test site failed because that name only exists in the proxy. So `read` must be denied by the action policy (section 4 checks that it is).

## 3. Text from JavaScript-only pages

### The spike's own pages, all four browsers

Eleven pages the spike serves (`site.go`), each built so that fetch_page's plain HTTP path finds nothing or a skeleton. The rendered HTML (`get html html`) goes through the copy of fetch_page's readable-text path (`readable.go`). Characters are those of the readable text; "phrases" are sentences from the article that must be in it.

The text was the same in all four browsers, character for character:

| Page | What | Plain (today) | After load | After network idle | Settled | Phrases |
|---|---|---|---|---|---|---|
| spa-inline | empty root; the article is inline JSON rendered by script | 0 | 725 | 725 | 725 | 2/2 |
| spa-fetch | empty root; the article comes from a JSON API | 0 | 513 | 513 | 513 | 2/2 |
| spa-delayed | content appears 2.5 s after load | 0 | 0 | 0 | 725 | 1/1 |
| spa-hash | a hash router picks the content from the fragment | 0 | 482 | 482 | 482 | 2/2 |
| shadow | a web component renders the article in its shadow root | 0 | 0 | 0 | **0** | 0/1 |
| iframe | the article is in a same-origin iframe | 0 | 0 | 0 | **0** | 0/1 |
| persian | a Persian article (RTL, ZWNJ) rendered by script | 0 | 436 | 436 | 436 | 2/2 |
| table | a price table from a JSON API | 0 | 472 | 472 | 472 | 3/3 |
| lazy | one section on load, four more when scrolled into view | 0 | 288 | 288 | 288 | **1/2** |
| skeleton | skeleton placeholders, replaced after an API call | 18 | 18 | 517 | 517 | 2/2 |
| canvas | text drawn only on a canvas, as Excalidraw does | 0 | 0 | 0 | **0** | 0/1 |

- 7 of 11 pages give their whole article with *open → wait for network idle → get html*; `spa-delayed` needs a short wait for the text to settle; `skeleton` shows why the load event alone isn't enough.
- **`get html` misses shadow roots and iframes**: the serialized HTML doesn't contain them. **Lazy sections** need scrolling. **Canvas** text isn't in the DOM at all. Section 3c tries other ways.
- The Persian page kept its ZWNJs and RTL text unchanged through the path.

### Time, memory and CPU (ranges over the 11 pages)

Time is to the text after network idle (*open → wait --load networkidle → get html*). "Cold" starts a new daemon and browser; "warm" opens the same page again in the running browser. Memory and CPU are peaks for the whole tree: the CLI calls, the daemon, the browser and its child processes, sampled every 100 ms (CPU over 500 ms windows, 100% = one core).

| Browser | Cold | Warm | Peak working set | Peak private bytes | Peak CPU | Processes |
|---|---|---|---|---|---|---|
| **chrome-headless-shell** | 2.0–2.2 s | 1.3–1.4 s | **234–254 MB** | **73–79 MB** | 156–221% | 8 |
| Chrome for Testing | 2.0–2.5 s | 1.3–2.0 s | 896–953 MB | 488–529 MB | 536–683% | 16–17 |
| installed Chrome | 1.8–2.6 s | 1.3–1.5 s | 826–852 MB | 424–443 MB | 530–716% | 16 |
| Edge | 2.2–2.5 s | 1.3–1.5 s | 870–888 MB | 441–458 MB | 660–781% | 20 |

Two rows were left out of the ranges: Chrome on `persian` and Edge on `lazy` showed 21 processes and CPU of 713,044% and 134,494%. Windows reuses process IDs, so an unrelated old process whose dead parent's ID had been reused by the daemon was counted in the tree. The sampler now checks that a child started after its parent (`bornAfter`); the live pages (3b) were measured with that fix.

The full browsers use about 3.5 times the working set and 6–7 times the private memory of the shell: more than 400 MB private, on a 4 GB minimum machine. chrome-headless-shell stays under 80 MB private and 260 MB working set on these pages.

### 3c. Other ways to take the text (chrome-headless-shell)

Characters · phrases found, after open, network idle and settling, on one warm browser:

| Page | `get html` → readable | `get text body` | `snapshot` | html after `scroll down 4000` ×3 |
|---|---|---|---|---|
| spa-inline | 725 · 2/2 | 760 · 2/2 | 962 · 2/2 | 725 · 2/2 |
| spa-fetch | 513 · 2/2 | 541 · 2/2 | 711 · 2/2 | 513 · 2/2 |
| spa-delayed | 725 · 1/1 | 760 · 1/1 | 962 · 1/1 | 725 · 1/1 |
| spa-hash | 482 · 2/2 | 513 · 2/2 | 683 · 2/2 | 482 · 2/2 |
| shadow | 0 · 0/1 | 1,422 · 1/1 | **962 · 1/1** | 0 · 0/1 |
| iframe | 0 · 0/1 | 381 · 0/1 | **749 · 1/1** | 0 · 0/1 |
| persian | 436 · 2/2 | 433 · 2/2 | 586 · **1/2** | 436 · 2/2 |
| table | 472 · 3/3 | 368 · 3/3 | 1,604 · 3/3 | 472 · 3/3 |
| lazy | 288 · 1/2 | 320 · 1/2 | 427 · 1/2 | 337 · 1/2 |
| skeleton | 517 · 2/2 | 548 · 2/2 | 719 · 2/2 | 517 · 2/2 |
| canvas | 0 · 0/1 | 603 · 1/1 | 8 · 0/1 | 0 · 0/1 |

- **The accessibility tree** (`snapshot`) has the text of shadow roots and iframes, which `get html` doesn't. But it is a tree of roles, not prose. It split the Persian sentence with the ZWNJ, so one phrase was missed, and it adds role labels. It is a fallback for when the readable text is under the threshold, not the main path.
- **`get text body` is not the visible text:** on `canvas` it "found" the phrase only because the page's own `<script>` source was in it, and the 1,422 characters on `shadow` include the script's JSON. It returns `textContent`, scripts included, so it isn't usable.
- **Lazy sections:** scrolling with `scroll` didn't trigger the page's `IntersectionObserver` for all four sections (one more section came in). Rendering for `fetch_page` should accept that lazy content is partly missing.
- **Canvas** text (Excalidraw) can't be read without OCR.

### 3b. The SPIKE-015 corpus pages, live (chrome-headless-shell)

Two modes. **Loose**: every public host allowed, the private-network check only. **Strict**: only the page's own host approved; every other host is refused without asking (that is what Jenab would do for sub-resources, below).

| Page | Plain fetch today | Rendered, loose | Rendered, strict | Requests (loose / strict) | Cold, warm (loose / strict) | Peak private (loose / strict) |
|---|---|---|---|---|---|---|
| js-excalidraw | 0 chars (`needs_javascript`) | 312 chars: the app's menus and help ("Pick a tool & Start drawing!") | 312, the same | 8 to 5 hosts / 20, 19 refused | 4.3, 6.3 s / 9.7, 5.0 s | 148 / 144 MB |
| js-coinmarketcap | **23,460 chars** (the page is server-rendered now; SPIKE-015 got 907) | 23,460, the same | 23,460, the same | 46 to 18 hosts / 211, 210 refused | 26.8, 12.6 s / 3.3, 1.8 s | 670 / 120 MB |
| news-spiegel-de | 1,105 | 1,105, the same | 1,105 | 17 to 5 hosts / 33, 32 refused | 5.0, 2.8 s / 2.9, 1.9 s | 200 / 113 MB |
| news-heise-de | 2,074 | 2,279–2,497 (more text, added by content from other hosts) | 2,074 | 25 to 13 hosts / 13, 12 refused | 7.3, 11.3 s / 2.8, 1.9 s | 203 / 128 MB |

- **On the real corpus, rendering gained nothing.** Excalidraw is a drawing app whose content is on a canvas; rendering gives its menus, not content. CoinMarketCap now sends its text in the HTML, so `fetch_page` already reads it. The two news pages read the same either way. No page showed a consent wall or a CAPTCHA to the headless shell.
- **Strict mode gave the same text** on all four pages, and was faster and lighter: CoinMarketCap 3.3 s and 120 MB private instead of 26.8 s and 670 MB. The page asks for many other hosts (210 refused requests on CoinMarketCap), so **asking per sub-resource host is impossible**: each would be a card.
- No socket of the tree went anywhere but the proxy, in all eight runs.

## 4. Safety (chrome-headless-shell)

### Action policy

The first run used a policy that denied `eval`, `download`, `upload`, `read`, `network` and `state`. **Only `read`, `download` and `upload` were refused**; `eval document.title`, `wait --fn`, `network route`, `state save` and `cookies set` all ran. The policy matches agent-browser's internal action names (`evaluate`, `waitforfunction`, `route`, `state_save`, `cookies_set`, …), not the category names its docs use for `--confirm-actions`; a name it doesn't know is silently ignored. So **the policy must be an allow list**. With

```json
{"default": "deny", "allow": ["navigate", "waitforloadstate", "content", "innerhtml", "snapshot", "url", "title", "scroll", "wait", "stream_disable", "close"]}
```

`open`, `wait --load`, `wait <ms>`, `get html`, `get url`, `get title`, `snapshot`, `scroll` and `stream disable` worked, and every other command tried was refused with "Action '…' is not in the allow list": `evaluate`, `waitforfunction`, `route`, `requests`, `state_save`, `cookies_get`, `cookies_set`, `storage_get`, `read`, `gettext`, `click`, `fill`, `type`, `press`, `upload`, `download`, `screenshot`, `tab_new`. A click on a download link saved nothing in the download folder.

### Other settings

- **`file://`:** `open file:///C:/Windows/win.ini` failed (`net::ERR_FAILED`); nothing of the file was returned.
- **Content boundaries:** with `contentBoundaries` on, the JSON gets `"_boundary": {"nonce": "<random hex>", "origin": "<page URL>"}` (`origin` was `"unknown"` for `get html`). Jenab takes the HTML out of the JSON and runs its own readable-text path, so the text keeps Jenab's own untrusted mark (8.8) as any `fetch_page` result does; the boundary adds nothing for that path.
- **`--max-output`** (config file, flag or environment variable, set to 300 or 500) **didn't cut `get html` or `snapshot`** in JSON mode (2,145 characters came back). The `read` help ties it to `read`'s output; plain-text output wasn't tested. Jenab must cap the HTML itself (6.6's 5 MB).
- **The stream server** ("Streaming is always enabled", a WebSocket on a random 127.0.0.1 port that can inject input) is on from the start; `stream disable` closed it (afterwards only the daemon's and Chrome's DevTools ports listened). There's no setting to keep it off, so for a moment after each start it listens. The daemon's port, the stream port and Chrome's DevTools port are reachable by every local process. I didn't test whether the daemon checks who connects.
- **The daemon's port comes from the session name:** `default` → 50838 in every run, `jenab-a1` → 49264, `jenab-b2` → 49232. Two programs using the default session name on one computer would share a port, so Jenab must use its own random session name per fetch.

### Consent walls and CAPTCHAs (the spike's pages)

| Page | What the rendered text is | Characters | Today's rule (`needs_javascript` under ~200) |
|---|---|---|---|
| consent-overlay: the article with a full-page consent dialog over it | **the article**; readability dropped the dialog | 513 | a normal page, correct |
| consent-wall: the content appears only after "Accept" | **only the wall**: "To continue reading, accept cookies and tracking by us and our partners, or subscribe…" | 214 | **looks like a real page** (just over 200) |
| captcha: a bot check that never finishes | "Verify you are human by completing the action below. … Ray ID: …" | 189 | `needs_javascript`, wrongly: rendering won't help |

So the text says what it is, in words a model can read, but the status doesn't. A rendered result needs its own checks: a page whose text is mostly consent or bot-check wording (the words in `consentWords`, and the title "Just a moment...") gets a status that says so, such as `blocked_by_consent` or `bot_check`. That needs a list of phrases in many languages, so it is a heuristic. The live news pages showed no wall to the headless browser.


## 5. Go or no-go for rendering in `fetch_page`

**No-go for now.** Jenab *can* render pages with agent-browser under 6.7's rules (the list below closes every hole the spike found), but on the real pages the gain was nil, and the cost is a 126 MB download, a third-party daemon with three local ports, and a proxy and policy layer to keep correct across agent-browser and Chrome versions.

- **Can it be made safe? Yes, with chrome-headless-shell only.** Through a proxy that applies 6.7, with `<-loopback>`, nothing got past it but WebRTC's STUN, and `--force-webrtc-ip-handling-policy=disable_non_proxied_udp` closed that in the shell (the full browsers ignore the switch). `read` goes around the proxy but an allow-list action policy refuses it. A kill-on-close job object and the idle timeout leave no process behind.
- **Is the gain worth it? Not on the evidence.** Of SPIKE-015's two JavaScript-only pages, CoinMarketCap now sends its text in the HTML (`fetch_page` reads 23,460 characters today) and Excalidraw is a canvas app, where rendering gives its menus. On the spike's own pages rendering worked for 7 of 11 page types, but shadow DOM, iframes, lazy sections and canvas needed more (3c), and a consent wall or a bot check renders into text that looks like a page (4).
- **What would change it:** evidence that `needs_javascript` is common in real use. That count is cheap to get: `fetch_page` already returns the status, so the benchmark and the chat logs can count it. The second question of the ticket (headless tasks in pipelines, app-started turns, macOS and Linux) is a stronger reason to ship a browser. If that is taken up, rendering for `fetch_page` costs little on top, and the settings below are the plan.

### If it is taken up later: what the build needs

1. **Browser:** chrome-headless-shell (Chrome for Testing channel), pinned version and SHA-256, downloaded on first use after the user agrees (126 MB zip, 282 MB on disk). Not Edge or the installed Chrome: they reach Microsoft or Google by themselves, ignore the WebRTC switch, and use 6–7 times the memory.
2. **agent-browser:** the pinned native exe, SHA-256 checked before each start (as agent-desktop); `AGENT_BROWSER_CONFIG` to a config file Jenab writes, `AGENT_BROWSER_SOCKET_DIR` and `TEMP`/`TMP` in a private state folder, `AGENT_BROWSER_NO_VERSION_CHECK=1`, every other `AGENT_BROWSER_*` and proxy variable removed, a random session name per fetch, `idleTimeout` 60 s, `close` at the end, all calls inside a kill-on-close job object.
3. **Network:** Jenab's own proxy on 127.0.0.1 applying 6.7 (private-network check on the resolved address, exceptions, every redirect), with `--proxy-bypass-list=<-loopback>` and `--force-webrtc-ip-handling-policy=disable_non_proxied_udp` in `args`. The page's host is approved as for any `fetch_page`; sub-resources from other public hosts are **refused without a card** (strict mode gave the same text on all four live pages, faster). Private addresses are refused unless an exception. The main-frame outcome (refused, redirected to an unapproved host) comes from the proxy's log, not from `open`'s `success`.
4. **Action policy:** `default: deny` with the allow list from section 4. Never `read`, `evaluate`, state, cookies, downloads or uploads.
5. **Text:** *open → wait --load networkidle → poll `get html` until it stops changing (max a few seconds) → readable text*, the HTML capped at 5 MB by Jenab (agent-browser's `--max-output` doesn't apply); if the result is still under the threshold, try the `snapshot` text; label consent walls and bot checks.
6. **`stream disable`** right after the first `open`.
7. **Time and memory:** about 2–3 s per page cold, 80–670 MB private while it runs.
