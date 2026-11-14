# SPIKE-047 report: a headless browser with agent-browser

**Branch:** `claude/spike-047-headless-browser-c16623`. Commits: d9a175c (Part A, the spike module and its results) and this report.
**Ran:** 2026-11-14, Windows 11 Pro, from PowerShell. **Models:** none. Part B wasn't run (below), so no model request was spent.
**Downloads,** after the user agreed: agent-browser 0.43.0 (`agent-browser-win32-x64.exe`, SHA-256 `ebcef5eb…630dc`, matches the release's digest), Chrome for Testing and chrome-headless-shell 157.0.8103.78 (MD5 matches Google's `x-goog-hash`; our SHA-256 is in the spike's README). The user allowed the installed Chrome with a fresh, empty profile only. Edge was already on the machine.

The details are in [`spikes/047-headless-browser/results.md`](../../../spikes/047-headless-browser/results.md), and the outputs they come from are in its `raw/` folder.

## 1. Result: no-go for rendering in `fetch_page`, for now

Jenab can render pages with agent-browser under 6.7's rules, but on the real corpus the gain was nil. The user accepted the no-go, and with it skipping Part B (the ticket runs Part B only on a go).

- **On the corpus, rendering gained nothing.** CoinMarketCap now sends its text in the HTML: `fetch_page` reads 23,460 characters today, where SPIKE-015 got 907. Excalidraw is a canvas app, and rendering gives its menus (312 characters). The two news pages read the same either way.
- **The spike's own JavaScript-only pages** (11 kinds) worked for 7 with *open → network idle → get html → readable text*. Shadow DOM and iframes need the accessibility tree (`snapshot`), lazy sections stayed partly missing, and canvas can't be read.
- **The cost:** a 126 MB download (282 MB on disk), a third-party daemon with three local ports, about 2–3 s per page and 80–670 MB private memory while it runs, and a proxy and policy layer to keep correct across agent-browser and Chrome versions.

## 2. What the spike found (Part A)

1. **It runs on its own.** The native exe needs no Node. With `AGENT_BROWSER_CONFIG`, `AGENT_BROWSER_SOCKET_DIR`, `TEMP` and `TMP` in a private folder and the version check off, nothing was written to the user's folders. A kill-on-close job object ended all 15 processes within 2 s when the "Jenab" stand-in was killed. Without the job object, the tree lived until the idle timeout.
2. **Network:**
   - Through Jenab-style proxying with `--proxy-bypass-list=<-loopback>`, every probe went through the proxy and was refused, except **WebRTC STUN over UDP**, which reached loopback and the LAN in all four browsers.
   - `--force-webrtc-ip-handling-policy=disable_non_proxied_udp` closes that hole **only in chrome-headless-shell**; the full browsers ignore the switch.
   - **`read <url>` goes around the proxy** and reached a loopback canary.
   - QUIC and DNS prefetch stayed inside the proxy.
   - The browser's own resolver requests used only the host cache and the hosts file. I first read them as a DNS leak, then corrected that after looking at the task types.
3. **Browser choice:** chrome-headless-shell. It is the only one that makes no requests of its own and honours the WebRTC switch. It also uses 6–7 times less private memory: 73–79 MB against 424–529 MB. Chrome for Testing, Chrome and Edge contact Google or Microsoft by themselves; Chrome also calls `accounts.google.com`.
4. **Safety:**
   - The action policy matches agent-browser's internal action names. A deny list with the category names left `eval`, `wait --fn`, `network route`, `state save` and `cookies set` working, so it must be an allow list. The allow list in results.md refused everything else that was tried.
   - `--max-output` doesn't cut `get html` or `snapshot`.
   - The stream server is always on until `stream disable`.
   - The daemon's port comes from the session name (`default` is always 50838).
   - A refused navigation still reports `success`, showing the proxy's 403 page.
   - A consent wall and a CAPTCHA render into text that looks like a page or like `needs_javascript`.

## 3. Problems along the way

- **Edge doesn't start from this session's Git Bash** (exit code 0, no output), even without agent-browser; from PowerShell it works. The cause wasn't found, so every measurement ran from PowerShell.
- **The process sampler counted an unrelated process** twice: a stale parent PID whose ID Windows had reused. That made two rows of the text table show impossible CPU. Those rows are left out of the ranges, and the sampler now checks that a child started after its parent.
- `--allowed-domains` made `open` time out with Chrome for Testing, Chrome and Edge; it worked only in the shell. I didn't find why.
- **Unrelated empty GUID `.tmp` files** keep appearing in `%TEMP%`. They also appeared on other days and before the spike started, so they don't come from agent-browser. A `spike045-*` folder there belongs to another session.

## 4. Not done

- **Part B** (headless tasks, macros, where it would apply): skipped with the no-go, as the user chose.
- No SPEC change and no build ticket, since it's a no-go. results.md section 5 lists what a later build would need.
- Packet capture: that mDNS sockets bound on port 5353 send nothing with the WebRTC policy on wasn't proven.
- Whether agent-browser's daemon checks who connects to its port wasn't tested.

## 5. Follow-ups

- Count `needs_javascript` results in real use (benchmark and chat logs). If they're common, results.md section 5 is the plan.
- The ticket's second question, headless tasks in pipelines, app-started turns and on macOS and Linux, is a stronger reason to ship a browser than `fetch_page`. If that is taken up, rendering comes cheap on top.
