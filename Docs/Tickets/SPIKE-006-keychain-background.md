# SPIKE-006 — Keychain in background mode
**Type:** Spike
**Status:** In progress (Windows decided; macOS needs a Mac)
**Gate:** 2

## Question
Can scheduled runs read secrets from the OS keychain without prompting the user?
1. Windows Credential Manager from a background process
2. macOS Keychain from an unsigned and a signed build
3. Linux Secret Service when no desktop session is unlocked (for later)

## Done when
Reading a stored secret works without a prompt on Windows and macOS, or the needed setup is documented.

## Result

### 1. Windows
Run on 2026-09-27 with zalando/go-keyring v0.2.8 (which uses danieljoos/wincred v1.2.3) and Go 1.26.0 on Windows 11 Pro 26200. The build is unsigned and has no manifest or setup.
- **Code and full output:** [spikes/006-keychain-background](../../spikes/006-keychain-background/) (`results.md`, `watch.log`).

| Check | Result |
|---|---|
| Read from a windowless process (`-H windowsgui`, like tray mode), started three ways: detached from a parent, by Explorer (as from the Start menu or at login), and hidden by PowerShell | **Works without a prompt:** 5 of 5 reads each time, in 2–4 ms for all five |
| Read while the screen is locked | **Works without a prompt** (SPIKE-004, 2026-09-28). Reads at lock, 5 s later, and once a minute until unlock all succeeded, including right after waking from sleep. The input desktop name stayed `Default` while locked, so lock detection uses session notifications. |
| Value size | 0 to 2,560 **bytes** work. 2,561 bytes fail with `ErrSetDataTooBig` (for Unicode the limit is also in bytes, not characters). A 410-byte name works. |
| Unicode value (`schlüssel-کلید-🔑`) | read back exactly |
| Missing name | `keyring.ErrNotFound` |
| Read cost | 0.1–0.15 ms per read |
| Where it is stored | Credential Manager → Windows Credentials → Generic, as `service:NAME` with "Local machine" persistence (it doesn't roam to other PCs) |
| Other programs of the same user | **Can read the value.** A separate program calling `CredReadW` read it without a prompt. `cmdkey /list` shows the names. |

**Findings**
- **No setup is needed on Windows.** The secret is tied to the Windows user, not to the program, so signing or install location doesn't matter.
- **Only other users are kept out.** The keychain protects secrets from other Windows users, and from anyone who copies the project folder. It doesn't protect them from other programs running as the same user.
- **Detecting the lock screen:** the WTS lock flag (`WTSSessionInfoEx` `SessionFlags`) read "locked" while the session was unlocked. Lock detection for SPIKE-004 should use session-change notifications (`WTSRegisterSessionNotification`), not this flag.

**Not tested:**
- **A Windows service,** or a scheduled task set to "run whether the user is logged on or not". These run without the user's logon, so the user's credentials are not expected to be available there. Jenab doesn't use either.
- **An elevated process** of the same user.

### 2. macOS
_Pending: needs a Mac (unsigned build, signed build, and a build with a changed signature)._

### 3. Linux
_Later._

## Decision
**Decided (2026-09-27, Windows):**
- **Storage:** use `zalando/go-keyring` with service `jenab`, so an entry is `jenab:<NAME>`, visible to the user in Credential Manager.
- **No caching:** Jenab reads a secret when a step needs it and doesn't keep it in memory. A read costs about 0.1 ms, and a changed key takes effect at the next run.
- **Size:** values are limited to 2,560 bytes and may not be empty. Jenab checks this when a secret is saved, with a clear message.
- **Where schedules run:** only in the app's own process in the user's session (window or tray), never as a Windows service or a logged-out scheduled task.
- **SPEC 6.7:** add the protection limit above: the keychain keeps secrets away from other users and out of the project folder, but not away from other programs of the same user.
