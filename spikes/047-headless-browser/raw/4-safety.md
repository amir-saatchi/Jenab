## 4. Safety (shell)

Action policy: `{ "default": "allow", "deny": [ "eval", "download", "upload", "read", "network", "state" ] }`; content boundaries on; max output 200,000.

- open: true
- get html, raw output starts: `{"_boundary":{"nonce":"0fbc86696334bcde71087c712a7bf75b","origin":"unknown"},"data":{"html":"<head><meta charset=\"utf-8\"><title>Tidal power in the Severn estuary</title></head><body><div id=\"root\"><article><h1>Tidal power in the Severn estuary</h1><p>The Severn estuary has one of the largest tidal ranges in the world, about fourteen metres at Avonmouth on a spring tide.</p><p>Engineers have pr…`
- get html, raw output ends: `…cript></body>","lifecycle":{"effectiveLaunch":{"browserLaunched":true,"engine":"chrome","launchHash":17754053663369384505},"launched":false,"relaunchedBrowser":false,"restartedBackground":false,"restoreStatus":"not_configured","reused":true,"saveStatus":"not_attempted"}},"error":null,"success":true}`
- get text body, raw output starts: `{"_boundary":{"nonce":"2b742c4d1548c8176d11706e9cd093a4","origin":"http://site.spike047.test:8945/js/spa-inline"},"data":{"lifecycle":{"effectiveLaunch":{"browserLaunched":true,"engine":"chrome","launchHash":17754053663369384505},"launched":false,"relaunchedBrowser":false,"restartedBackground":false…`

| Command | Success | Error / result |
|---|---|---|
| `eval document.title` | true | {"lifecycle":{"effectiveLaunch":{"browserLaunched":true,"engine":"chrome","launchHash":17754053663369384505},"launched":… |
| `wait --fn document.title.length > 0` | true | {"lifecycle":{"effectiveLaunch":{"browserLaunched":true,"engine":"chrome","launchHash":17754053663369384505},"launched":… |
| `read` | false | Action 'read' denied by policy: Action 'read' is denied by policy |
| `network route **/* --abort` | true | {"lifecycle":{"effectiveLaunch":{"browserLaunched":true,"engine":"chrome","launchHash":17754053663369384505},"launched":… |
| `state save <scratch>\state\shell-safety-1\state.json` | true | {"lifecycle":{"effectiveLaunch":{"browserLaunched":true,"engine":"chrome","launchHash":17754053663369384505},"launched":… |
| `cookies set a b` | true | {"lifecycle":{"effectiveLaunch":{"browserLaunched":true,"engine":"chrome","launchHash":17754053663369384505},"launched":… |
| `snapshot -i` | true | {"lifecycle":{"effectiveLaunch":{"browserLaunched":true,"engine":"chrome","launchHash":17754053663369384505},"launched":… |
| `get title` | true | {"lifecycle":{"effectiveLaunch":{"browserLaunched":true,"engine":"chrome","launchHash":17754053663369384505},"launched":… |
| `download #dl <scratch>\state\shell-safety-1\downloads\x.txt` | false | Action 'download' denied by policy: Action 'download' is denied by policynull |
| `upload #f <scratch>\state\shell-safety-1\up.txt` | false | Action 'upload' denied by policy: Action 'upload' is denied by policynull |
| `click #dl` | false | Element not found: #dl. Verify the selector, role, or name is correct and the element exists in the DOM.null |

Files in the download folder after `click` on a download link: []

- `open file:///C:/Windows/win.ini`: false Navigation failed: net::ERR_FAILED; body has "[fonts]": false
- `stream status`: {"connected":true,"enabled":true,"lifecycle":{"effectiveLaunch":{"browserLaunched":true,"engine":"chrome","launchHash":17754053663369384505},"launched":false,"relaunchedBrowser":false,"restartedBackgr…
- `stream disable`: true ; listening afterwards: 127.0.0.1:50838 agent-browser-win32-x64.exe; 127.0.0.1:8950 chrome-headless-shell.exe

- With max output 500: get html returned 2145 characters of html; ends: `…s answer that the output is predictable decades ahead, which is more than wind or sun can offer, and that the walls last a century or more."]});</script></body>`

### Consent walls and CAPTCHAs (spike's pages)

- **consent-overlay** (the article, with a consent dialog over it): title "Field notes on urban foxes", 513 characters, article phrases 1/1; consent words: none. Text: "Red foxes moved into British cities in the 1930s, when suburbs with large gardens spread along new railway lines. A city fox's territory is small, often less than a square kilometre, because food is everywhere: bins, compost heaps and people who feed them on purpose. Mange outbreaks can halve a loca…"
- **consent-wall** (the content loads only after consent is accepted): title "Read on with ads or subscribe", 214 characters, article phrases 0/1; consent words: cookie ×1; consent ×1; privacy ×1; partner ×1. Text: "To continue reading, accept cookies and tracking by us and our partners, or subscribe to read without ads for 4.99 a month. You can withdraw your consent at any time in the privacy settings at the foot of the page."
- **captcha** (a bot-check interstitial that never finishes): title "Just a moment...", 189 characters, article phrases 0/1; consent words: verify you are human ×1. Text: "## example.org ## Verify you are human by completing the action below. I am human example.org needs to review the security of your connection before proceeding. Ray ID: 8f3a2c9d1e7b4a60"

### Files outside the state folders

Nothing new in the home folder, %LOCALAPPDATA%, %APPDATA% or %TEMP% (top level), and no `~/.agent-browser`.

No agent-browser or browser process left at the end.
