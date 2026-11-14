## 4b. Action policy as an allow list (shell)

Policy: `{ "allow": [ "navigate", "waitforloadstate", "content", "innerhtml", "snapshot", "url", "title", "scroll", "wait", "stream_disable", "close" ], "default": "deny" }`; max output 300 in the config file.

| Command | Success | Error, or the size of the output |
|---|---|---|
| `open http://site.spike047.test:3735/js/spa-inline` | true | ok, 687 bytes of JSON |
| `wait --load networkidle --timeout 5000` | true | ok, 392 bytes of JSON |
| `wait 300` | true | ok, 398 bytes of JSON |
| `get html html` | true | ok, 2545 bytes of JSON, html 2145 chars |
| `get text body` | false | Action 'gettext' denied by policy: Action 'gettext' is not in the allow list |
| `get url` | true | ok, 460 bytes of JSON |
| `get title` | true | ok, 414 bytes of JSON |
| `snapshot` | true | ok, 1555 bytes of JSON, snapshot 962 chars |
| `snapshot -i` | true | ok, 635 bytes of JSON, snapshot 63 chars |
| `scroll down 500` | true | ok, 386 bytes of JSON |
| `eval document.title` | false | Action 'evaluate' denied by policy: Action 'evaluate' is not in the allow list |
| `wait --fn true` | false | Action 'waitforfunction' denied by policy: Action 'waitforfunction' is not in the allow list |
| `network route **/* --abort` | false | Action 'route' denied by policy: Action 'route' is not in the allow list |
| `network requests` | false | Action 'requests' denied by policy: Action 'requests' is not in the allow list |
| `state save <scratch>\state\shell-policy-1\state.json` | false | Action 'state_save' denied by policy: Action 'state_save' is not in the allow list |
| `cookies get` | false | Action 'cookies_get' denied by policy: Action 'cookies_get' is not in the allow list |
| `cookies set a b` | false | Action 'cookies_set' denied by policy: Action 'cookies_set' is not in the allow list |
| `storage local` | false | Action 'storage_get' denied by policy: Action 'storage_get' is not in the allow list |
| `read http://site.spike047.test:3735/js/spa-inline` | false | Action 'read' denied by policy: Action 'read' is not in the allow list |
| `click h1` | false | Action 'click' denied by policy: Action 'click' is not in the allow list |
| `fill input x` | false | Action 'fill' denied by policy: Action 'fill' is not in the allow list |
| `type h1 x` | false | Action 'type' denied by policy: Action 'type' is not in the allow list |
| `press Enter` | false | Action 'press' denied by policy: Action 'press' is not in the allow list |
| `upload #f <scratch>\state\shell-policy-1\up.txt` | false | Action 'upload' denied by policy: Action 'upload' is not in the allow list |
| `download #dl <scratch>\state\shell-policy-1\downloads\x.txt` | false | Action 'download' denied by policy: Action 'download' is not in the allow list |
| `screenshot <scratch>\state\shell-policy-1\shots\s.png` | false | Action 'screenshot' denied by policy: Action 'screenshot' is not in the allow list |
| `tab new` | false | Action 'tab_new' denied by policy: Action 'tab_new' is not in the allow list |
| `stream disable` | true | ok, 387 bytes of JSON |

### Where --max-output applies

- flag: get html gave 2145 chars of html (true ); snapshot 962 chars
- env: get html gave 2145 chars of html (true ); snapshot 962 chars

### The daemon's port

- session "": default.port=50838
- session "jenab-a1": jenab-a1.port=49264
- session "jenab-b2": jenab-b2.port=49232

### Files outside the state folders

New entries: %TEMP%\497171a5-fbd3-4f97-9eb9-1b460ae8f418.tmp

No agent-browser or browser process left at the end.
