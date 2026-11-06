# TASK-041 report — one card for a new private host

Branch `task-041-one-card-private-host`, one commit on `main` at 8d9d286: `43036f5 TASK-041: one card for a new private host`. Not pushed. Docs/ untouched; no frontend change.

## What changed

**The rule.** When `fetch_page` goes to a host that isn't approved yet, reaches only private addresses, and isn't a private-network exception, and the level would ask for the host card, the host card is dropped. The private card asks alone, and its answer covers the host:

| Answer | This request | Private-network exceptions | Host approval |
|---|---|---|---|
| Allow once | allowed | unchanged | none recorded; the next request asks the one card again |
| Allow in this project | allowed | host added | `host` / `always` / `user` recorded |
| Deny | blocked, with the note | unchanged | `host` / `deny` / `user` recorded; the next request asks the one card again |

Unchanged: a public host (host card only), an exception host that isn't an approved host yet (host card only), and Auto where the level approves the host by itself (auto chip, then the private card). Redirect cards still name both hosts.

**Files**
- `internal/web/fetch.go`
  - `Client.Private(ctx, host)`: true when the host is a held-back address, or a name whose addresses all are. The lookup is capped at 5 s. A failed or empty lookup counts as false, so the host card asks as before.
  - `Rules.PrivateOnly`: the host whose card was folded may reach only private addresses in this fetch. A public address for it is refused ("it resolved to a public address (…) after it was checked for a private one; fetch it again") and nothing is asked. So DNS that changes between the check and the dial can't skip the host card. The model's next call checks again and gets the host card.
- `internal/tool/tool.go`: `Needs.Private` (Preflight's finding) and `Env.FoldHost` (the agent's decision).
- `internal/tool/page.go`
  - Preflight sets `Needs.Private` for an unapproved host that `Client.Private` says is private. It skips the lookup for approved hosts.
  - `run` passes `Env.FoldHost` as `Rules.PrivateOnly`.
  - For the folded host, the private card's details add "New host for this project: Allow in this project also approves it as a host".
- `internal/agent/approve.go`
  - `foldHost` decides with the level, the untrusted mark and the exceptions. It drops the host approval from the needs, sets `FoldHost`, and wraps `Env.Approve` to record the host answer from the private card (Always or Deny by the user; nothing for Once, Stop or an error).
  - `withReason` gives host and private cards the agent's text before the call, or "The agent gave no reason".
- `internal/agent/turn.go`
  - The project's exceptions are now read before the approvals rather than after, so the fold and the fetch use the same list.
  - `foldHost` runs before `approve`.
  - Asked cards get `withReason`.
- `internal/agent/subagent.go`: subagent tool calls run `foldHost` too and ask through `env.Approve`.

## Checks

All run in the worktree with `GOPROXY=off GOMEMLIMIT=1500MiB`.

- `go build ./...` passed, after `bun install --frozen-lockfile` and `bun run build` in `frontend/` to make `frontend/dist`, which the worktree lacked. Both outputs are ignored by git, and no frontend file changed.
- `go vet ./...` passed.
- `go test -p 2 -short ./...`: every package passed except `internal/scenario` `TestRunTimeout/waiting_for_a_message` ("0 calls"). That failure came while I was building the server binary at the same time. Re-run alone, `go test -p 2 -short -count=1 ./internal/scenario/` passed. It's a timing test that doesn't use fetch_page.
- gofmt: clean on every changed file, checked with CR stripped. The worktree checks out CRLF (`core.autocrlf=true`), so plain `gofmt -l` lists every file.
- `go run ./cmd/jenab-scenarios check scenarios`: all ok, exit 0.
- Frontend checks: not run, since nothing in `frontend/` changed.

**New tests**
- `internal/web/fetch_test.go`
  - `TestClientPrivate`: addresses, all-private names, mixed names, public names and failed lookups.
  - `TestPrivateOnly`: a public address after the check is refused without asking; a private one asks.
- `internal/agent/private_host_test.go`, all with a local httptest server and a fake resolver. `public.test` resolves to a documentation address, and no test connects to it.
  - `TestNewPrivateHostOneCard` (Strict and Standard): one card. Once fetches and records no host; the next request asks one card. Allow in this project records `host always` and the exception; the next request asks nothing.
  - `TestNewPrivateHostDeny`: blocked with the note, `host deny` recorded, and the next request asks one card again. Stop records nothing.
  - `TestNewPublicHostUnchanged`: host card only, with the agent's reason; Deny gives "not run".
  - `TestExceptionHostAsksHostCard`: an exception host that isn't approved gets the host card only.
  - `TestNewPrivateHostAuto`: in one turn at Auto, the first host gets the auto chip plus the private card. After the page is read, the next new private host gets one card.
  - `TestCardWithoutReason`: private and host cards show "The agent gave no reason".
- `internal/agent/private_test.go`: the harness now takes the hosts to approve as an argument. The existing tests are unchanged.

## Server-build check

Scratch only: `LOCALAPPDATA` and `USERPROFILE` pointed into the session scratchpad, no API keys, and not the user's data or running Jenab.

- `go build -tags server` of `cmd/desktop`, plus builds of `jenab-fakesite` and a scratch fake Ollama model. The model is 60 lines, stdlib only, in the scratchpad and not committed. Its first reply in a turn calls `fetch_page`, with no text before it.
- Ports: 8099 is in a Windows-reserved range (8054–8153) on this machine, so the fake site ran with `-addr 127.0.0.1:18099`. The fake model used 19411 and the app 19310.
- In the browser: Create project → New chat at **Approvals: Standard** → asked to read `http://127.0.0.1:18099/crypto/2026-10-05`.
  - **One card** appeared: "Allow requests to 127.0.0.1 on this computer?", reason "The agent gave no reason". The details listed Host, "127.0.0.1 is an address on this computer", Tool: fetch_page, and "New host for this project: Allow in this project also approves it as a host". No host card came before it.
  - After *Allow in this project*, the page was stored ("Daily crypto close: Monday 5 October 2026") and the card shows "Approved".
  - A second message in the same chat fetched again with **no card**. The fake site logged both GETs, at 11:23:26 and 11:23:38; one earlier GET at 11:22:24 was my curl check.
  - The server and app logs show no errors.
- The three processes were stopped by PID after checking their paths.

## Open points

1. **Auto after untrusted content.** I read "Auto: unchanged" as: where Auto approves the host by itself, its chip and the private card stay as they are. After the turn has read a page, Auto would ask the host card, and that case now folds into one card too, by the same rule as Strict and Standard (`TestNewPrivateHostAuto` covers it). If Auto should still ask twice there, `foldHost` should skip `project.Auto`.
2. **A host with both public and private addresses** isn't folded. It gets the host card; the private card follows only if every public address fails to connect.
3. **Redirect to a new private host.** `fetch_page` still refuses a redirect to a host that isn't approved, and the model fetches that URL itself, which now asks one card. The card that names both hosts appears only when the redirect target is already an approved host, as before. Folding the redirect too would mean letting `approvedRedirect` pass unapproved private targets; I left it alone.
4. **Preflight now does a DNS lookup** for unapproved hosts (cap 5 s) before the card. Approved hosts skip it.
5. **Record rows.** A folded card writes the `private` row as before, plus a `host` row for Always and Deny. Once writes no `host` row, so the record doesn't show that the host was used once. That seems right, because the host card has no Once.
6. **Subagent path.** `foldHost` runs there too, but no test covers it; a subagent with `fetch_page` would need a test harness of its own.
7. **Exceptions read earlier.** The list is now read before the approvals rather than after. An exception added while a card waits applies from the next request, as SPEC 6.7 already says ("Changes apply to the next request").
8. Unchanged from TASK-036: an exception covers every port.

## Proposed SPEC text

### 6.7, *Private network*: replace the last sub-bullet ("On Strict and Standard, a new private host asks twice for now…") with

> - **One card for a new host:** where the host card would ask and the host reaches only private addresses and isn't an exception, only this card is shown, and its answer covers both. *Allow once* allows this request. *Allow in this project* adds the exception and approves the host for the project. *Deny* denies both. That fetch then reaches the host only at private addresses: a name that resolves to a public one after the check is refused, and the next request asks as for a public host. A public host, a host with public and private addresses, an exception not yet approved as a host, and a host Auto approves by itself ask as before.

and in *The card says*, add at the end:

> The card gives the agent's text before the call as its reason, or "The agent gave no reason".

### 8.8, approval card table

| Asked for | Options | Details shown |
|---|---|---|
| A new host (6.7) | Allow for this project, Deny | The host, the pipeline or tool that needs it, and the agent's reason |
| A private network address (6.7) | Allow once, Allow in this project, Deny | The host, the resolved address and what it is (this computer or your local network), the redirecting host, the tool, and the agent's reason; for a new host, that *Allow in this project* also approves it as a host |

### 8.8, *Content*: append

> For host and private-network cards, the reason is the agent's text before the call, or "The agent gave no reason" when it wrote none.

### 8.8, *The same at every level*: change the private-network bullet to

> - Private network addresses ask every time, unless the host is an exception in project settings (6.7). A new host that reaches only private addresses asks once, in this card, where its host card would ask.
