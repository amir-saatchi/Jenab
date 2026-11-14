# SPIKE-047 — a headless browser with agent-browser

Throwaway code for [SPIKE-047](../../Docs/Tickets/SPIKE-047-headless-browser.md). Windows 11 only. Results are in [results.md](results.md).

Nothing here is downloaded at run time. The program and the browsers were downloaded once, by hand, after the user agreed, and checked against the published checksums:

| File | Source | Size | Checksum |
|---|---|---|---|
| `agent-browser-win32-x64.exe` 0.43.0 | github.com/vercel-labs/agent-browser releases, v0.43.0 | 20,043,776 bytes | SHA-256 `ebcef5ebea93c7ccf0fe3f8c18eee908ae9174dfef8e52a3dda6f24a59f630dc` (matches the release's published digest) |
| `chrome-win64.zip` 157.0.8103.78 (Chrome for Testing) | storage.googleapis.com/chrome-for-testing-public | 209,854,321 bytes | MD5 `XK8XW84hYqn3HhjK4XBk7w==` (matches `x-goog-hash`); SHA-256 `5f6ec40206407112702e163bf694d07f9fdc8ccbd346c70ce61fa62f716493f9` |
| `chrome-headless-shell-win64.zip` 157.0.8103.78 | the same | 126,047,001 bytes | MD5 `lFiXwqF5Zgu7AHYKZ3RjtA==` (matches `x-goog-hash`); SHA-256 `2138c1583d25d6f3abec5269943b2f430207d87a539895aa9e90cc4697ec0e1f` |

Google publishes MD5 and CRC32C for Chrome for Testing, not SHA-256; the SHA-256 is ours, for pinning.

```powershell
go build -o bin/spike047.exe .
$a = @('-ab', '<dir>\agent-browser-win32-x64.exe', '-cft', '<dir>\chrome-win64\chrome.exe',
       '-shell', '<dir>\chrome-headless-shell-win64\chrome-headless-shell.exe', '-home', '<private state dir>')
bin\spike047.exe @a -part run                                   # 1. running it
bin\spike047.exe @a -part net -browsers shell,cft,chrome,edge   # 2. network rules (-variants V6,V7 for some only)
bin\spike047.exe @a -part text -browsers shell,cft,chrome,edge  # 3. the spike's JavaScript-only pages
bin\spike047.exe @a -part live -browsers shell -livepages 'id=url,…'  # 3b. SPIKE-015 corpus pages, live
bin\spike047.exe @a -part extract -browsers shell               # 3c. other ways to take the text
bin\spike047.exe @a -part safety -browsers shell                # 4. safety
bin\spike047.exe @a -part policy -browsers shell                # 4b. the action policy as an allow list, --max-output, the daemon's port
```

Run it from PowerShell or another normal Windows environment. Started from the Git Bash shell of a Claude Code session, Edge exits at once with code 0 and no output (also without agent-browser); from PowerShell it works. The cause wasn't found (not `HOME`, `TERM`, `MSYSTEM` or a job object).

- `ab.go`: runs agent-browser as Jenab would: SHA-256 checked before each start (the file stays open, shared for reading only, until the process has started), a private state folder (`privdir_windows.go`, the same ACL check as agent-desktop's), a trusted config file through `AGENT_BROWSER_CONFIG`, `AGENT_BROWSER_SOCKET_DIR` and `TEMP` inside the state folder, no version check, no console window, a time limit per call
- `proxy.go`: the 6.7 proxy: host approval, and the private-network check on the resolved address (`addr.go` is a copy of Jenab's rules). Where Jenab would ask, it records the question and denies
- `site.go`: the spike's pages (network probes, 11 JavaScript-only pages, consent and CAPTCHA pages) and the canaries: listeners on loopback and the LAN address, which the proxy never allows, so any hit is a request that got past it
- `procs_windows.go`: the process tree (CLI, daemon, browser), its peak memory and CPU, and its sockets, every 100 ms
- `net.go`, `text.go`, `extract.go`, `safety.go`, `policy.go`, `run.go`: the parts; `edgeflags_windows.go`: the Edge start-up probe (`-part edgeflags [-job]`)
- `raw/`: the outputs the results come from, with the user name, scratch folder and addresses replaced
- `readable.go`: a copy of `internal/web/readable.go`, so rendered HTML goes through fetch_page's own readable-text path
