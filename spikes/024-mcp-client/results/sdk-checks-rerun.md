| SDK | Transport | Check | OK | Detail |
|---|---|---|---|---|
| go-sdk | stdio | connect | yes | protocol 2026-07-28 in 36ms |
| go-sdk | stdio | list tools | yes | 10 tools, err=<nil> |
| go-sdk | stdio | call | yes | text="hi" err=<nil> |
| go-sdk | stdio | tool error | yes | isError=true text="failed: x" err=<nil> |
| go-sdk | stdio | unknown tool | yes | Go error: calling "tools/call": unknown tool "nope" |
| go-sdk | stdio | bad arguments (info) | yes | passed through: isError=false text="5" err=<nil> |
| go-sdk | stdio | progress | **no** | messages of 3 (when the result came/after 0.3s), 5 calls: 3/3 3/3 2/2 2/2 3/3 |
| go-sdk | stdio | cancel | yes | returned after 1.5s (context deadline exceeded); server saw the cancel: true; next call ok: true |
| go-sdk | stdio | 6 MB result | yes | 6144000 bytes in 0.3s, err=<nil> |
| go-sdk | stdio | crash | yes | call returned after 0.0s: calling "tools/call": EOF; next call: connection closed: calling "tools/call": client is closing: EOF |
| go-sdk | stdio | restart | yes | new process 28240 connected in 31ms, answers: true |
| go-sdk | stdio | tree: Close only | **no** | before: fake.exe(17528), fake.exe(31132); after Close (0.0s, err=<nil>): fake.exe(31132); at the end: fake.exe(31132) |
| go-sdk | stdio | tree: Close + job | yes | before: fake.exe(35568), conhost.exe(20184), fake.exe(17340); after Close (0.0s, err=<nil>): conhost.exe(20184), fake.exe(17340); at the end: none |
| go-sdk | http | connect | yes | protocol 2025-11-25 in 5ms |
| go-sdk | http | list tools | yes | 10 tools, err=<nil> |
| go-sdk | http | call | yes | text="hi" err=<nil> |
| go-sdk | http | tool error | yes | isError=true text="failed: x" err=<nil> |
| go-sdk | http | unknown tool | yes | Go error: calling "tools/call": unknown tool "nope" |
| go-sdk | http | bad arguments (info) | yes | passed through: isError=false text="5" err=<nil> |
| go-sdk | http | progress | **no** | messages of 3 (when the result came/after 0.3s), 5 calls: 3/3 3/3 2/2 2/2 3/3 |
| go-sdk | http | cancel | yes | returned after 1.5s (context deadline exceeded); server saw the cancel: true; next call ok: true |
| go-sdk | http | 6 MB result | yes | 6144000 bytes in 0.3s, err=<nil> |
| mcp-go | stdio | connect | yes | protocol 2026-07-28 in 35ms |
| mcp-go | stdio | list tools | yes | 10 tools, err=<nil> |
| mcp-go | stdio | call | yes | text="hi" err=<nil> |
| mcp-go | stdio | tool error | yes | isError=true text="failed: x" err=<nil> |
| mcp-go | stdio | unknown tool | yes | Go error: invalid params: unknown tool "nope" |
| mcp-go | stdio | bad arguments (info) | yes | passed through: isError=false text="5" err=<nil> |
| mcp-go | stdio | progress | yes | messages of 3 (when the result came/after 0.3s), 5 calls: 3/3 3/3 3/3 3/3 3/3 |
| mcp-go | stdio | cancel | yes | returned after 1.5s (transport error: context deadline exceeded); server saw the cancel: true; next call ok: true |
| mcp-go | stdio | 6 MB result | yes | 6144000 bytes in 0.3s, err=<nil> |
| mcp-go | stdio | crash | yes | call returned after 0.0s: transport error: transport closed; next call: transport error: transport closed |
| mcp-go | stdio | restart | yes | new process 32220 connected in 27ms, answers: true |
| mcp-go | stdio | tree: Close only | **no** | before: fake.exe(39456), fake.exe(39872); after Close (0.0s, err=<nil>): fake.exe(39872); at the end: fake.exe(39872) |
| mcp-go | stdio | tree: Close + job | yes | before: fake.exe(31808), fake.exe(11076), conhost.exe(22140); after Close (0.0s, err=<nil>): fake.exe(11076), conhost.exe(22140); at the end: none |
| mcp-go | http | connect (default) | **no** | server/discover result lists no modern version this client supports: [2025-11-25 2025-06-18 2025-03-26 2024-11-05] |
| mcp-go | http | connect | yes | protocol 2025-11-25 in 1ms |
| mcp-go | http | list tools | yes | 10 tools, err=<nil> |
| mcp-go | http | call | yes | text="hi" err=<nil> |
| mcp-go | http | tool error | yes | isError=true text="failed: x" err=<nil> |
| mcp-go | http | unknown tool | yes | Go error: invalid params: unknown tool "nope" |
| mcp-go | http | bad arguments (info) | yes | passed through: isError=false text="5" err=<nil> |
| mcp-go | http | progress | yes | messages of 3 (when the result came/after 0.3s), 5 calls: 3/3 3/3 3/3 3/3 3/3 |
| mcp-go | http | cancel | **no** | returned after 1.5s (transport error: context deadline exceeded); server saw the cancel: false; next call ok: true |
| mcp-go | http | 6 MB result | yes | 6144000 bytes in 0.4s, err=<nil> |
