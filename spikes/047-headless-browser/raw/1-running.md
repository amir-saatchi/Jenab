## 1. Running it

### The pinned program

- SHA-256 of the file used: `ebcef5ebea93c7ccf0fe3f8c18eee908ae9174dfef8e52a3dda6f24a59f630dc` (pinned: `ebcef5ebea93c7ccf0fe3f8c18eee908ae9174dfef8e52a3dda6f24a59f630dc`)
- A copy with one byte added: pinned program: SHA-256 is 37cb662d5b773664d5f12a4ecbee9f1b46ffce0cbb7a5fbd5e47701367772dec, not ebcef5ebea93c7ccf0fe3f8c18eee908ae9174dfef8e52a3dda6f24a59f630d…
- Node on PATH here: true. `--version` with PATH set to System32 only: "agent-browser 0.43.0" (err <nil>)

### Browsers

| Browser | Program | Size on disk | Starts under agent-browser | Cold open + get html | Error |
|---|---|---|---|---|---|
| cft | chrome.exe | 442 MB | true | 740ms |  |
| shell | chrome-headless-shell.exe | 282 MB | true | 830ms |  |
| chrome | chrome.exe | 506 MB | true | 710ms |  |
| edge | msedge.exe | 889 MB | true | 960ms |  |

### The state folder while a session runs

Daemon pid file: 10288. Entries (two levels): config.json, downloads, shots, sock, sock/default.config, sock/default.engine, sock/default.pid, sock/default.port, sock/default.stream, sock/default.target, sock/default.version, tmp, tmp/37d01ef1-a398-4bc3-8a40-b1a40be30320.tmp, tmp/5943e75e-5fb0-4921-afcb-d0c9db89bf26.tmp, tmp/804192af-ca04-4fb2-9b46-27f376548312.tmp, tmp/8a68091e-a1fa-4d45-8242-8c7c9a7c9df5.tmp, tmp/agent-browser-chrome-8bd87fa9-f97f-4b69-a84b-d53738e7a27f, tmp/d29c488b-fe10-49d6-a934-2a4b594deaf7.tmp, tmp/f10c41d1-096d-4530-a9cc-59720d9cc54d.tmp
- sock/default.port: 50838
- sock/default.stream: 11366
- Listening sockets of the tree: 127.0.0.1:11366 agent-browser-win32-x64.exe; 127.0.0.1:11369 chrome.exe; 127.0.0.1:50838 agent-browser-win32-x64.exe
- Processes: map[agent-browser-win32-x64.exe:1 chrome.exe:13]

### Clean-up

- `close`: success true; tree gone after 0s; left: []
- Temporary profiles left in the state folder's tmp after `close`: []
- A call killed after 1.5 s (killed=false) and one after 2 ms (killed=true): left right after: [agent-browser-win32-x64.exe(692) chrome.exe(10540) chrome.exe(15504) chrome.exe(19932) chrome.exe(23940) chrome.exe(24684) chrome.exe(25688) chrome.exe(25808) chrome.exe(31848) chrome.exe(32400) chrome.exe(3300) chrome.exe(7128) chrome.exe(9680) chrome.exe(9772)]
- The next call on that session: success true, title "App"
- With no `close`, the idle timeout (15 s) ended the tree after 15s; left: []
- Fake Jenab killed (job object with kill-on-close: false): 14 processes before, 15 two seconds after; all gone after 16s more; left: []
- Fake Jenab killed (job object with kill-on-close: true): 15 processes before, 0 two seconds after; all gone after 0s more; left: []

### Files outside the state folders

New entries: %TEMP%\c40e86ea-7727-48a3-8ddc-ba335b29272c.tmp

No agent-browser or browser process left at the end.
