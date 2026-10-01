# SPIKE-004 — Sleep/wake detection and catch-up
**Type:** Spike
**Status:** Done (Windows decided); macOS needs a Mac
**Gate:** 2

## Question
Can the scheduler reliably detect missed runs after sleep, as SPEC 6.2 requires?
1. How Go timers, tickers and `time.Sleep` behave across sleep on Windows and macOS
2. Whether a one-minute wall-clock check is enough, or OS wake events are needed
3. How to receive wake events: Wails, or OS APIs (`WM_POWERBROADCAST`, `PowerRegisterSuspendResumeNotification`)
4. Modern Standby (S0 low power idle, the only sleep on the test laptop): does the process keep running during sleep, and which events arrive?

On the same run, a keychain read every minute also covers SPIKE-006's open lock-screen row.

## Done when
A test program shows a missed scheduled run being caught up after the machine wakes, with a log of which events and timers fired when.

## Result

### 1. Windows
Run on 2026-09-28, 16:19–17:29 (70 min), with Go 1.26.0 on Windows 11 Pro 26200. The laptop supports Modern Standby only. Code and full output: `spikes/004-sleep-and-catch-up/` (`results.md`, `sleep.jsonl`).

**How the test program works**
- **Event sources:**
  - a hidden top-level window made the same way as Wails v3 beta.26's
  - `PowerRegisterSuspendResumeNotification`
  - 7 power-setting notifications (display, lid, user presence, power source)
  - session (lock/unlock) notifications
- **Three schedulers** had jobs at 16:34, 16:44 and 16:54:
  - *spec*: a one-minute check plus a check on every wake event
  - *tick-only*: the one-minute check alone
  - *naive*: one `time.Timer` per job, set once at start
- **The laptop slept 4 times:** 2.9 s, 1.6 s, 3.0 min and 24.7 min. The sleeps started when the display turned off or the lid closed, on AC power and on battery.

| Question | Answer |
|---|---|
| Does the process run during sleep? | **No.** No log lines were written inside any sleep, and the unbiased interrupt time stood still. |
| Go timers across sleep | Go's monotonic clock **includes** sleep time on Windows. A timer that fell due during sleep fires once, 0.0–1.5 s after waking. A 1-minute `Ticker` fires once, not a burst, then keeps its old phase, so a second tick can follow within seconds. `time.Sleep` returns on wake. Even the naive 35-minute timer fired 0.1 s after waking. |
| Is a one-minute check enough? | **Yes.** The 16:54 job fell inside the 3-minute sleep. All three schedulers ran it 0.1 s after waking, 52 s late. The wake-event checks came later and found nothing due. |
| Wake events through Wails | **None.** The Wails-style window got **0** suspend and 0 resume messages, only 2 power-status changes. Wails' `SystemWillSleep` and `SystemDidWake` would never have fired. |
| Wake events from the OS | `PowerRegisterSuspendResumeNotification` delivered all 4 suspends and 4 resumes, about 0.1 s after the timers had already fired. The display, lid, presence and power-source settings arrived too. |
| Keychain on the lock screen (SPIKE-006) | **Works.** The screen locked at 17:19:39 as the laptop woke and was unlocked at 17:20:02. Reads at lock, lock + 5 s, and the minute read in between all succeeded. Over the whole run, 57 of 57 reads succeeded, with a median of 0.6 ms (82 ms for the first read after a wake). |

**Findings**
- **Detecting the lock screen:** the input desktop's name stayed `Default` while the screen was locked, so it can't show the lock screen. Session lock/unlock notifications are the reliable signal.
- **Catch-up after a long sleep:** the 25-minute sleep had no job inside it, so no run was more than 65 s late (the spike's limit for `on_time`). Every wake still ran the minute check within 1.5 s, which is what catch-up needs.
- **Not tested:**
  - a real Wails app window (the spike used a window made the same way)
  - hibernate
  - sleep started from the Start menu
  - a job due more than a minute before waking

### 2. macOS
_Pending: needs a Mac. Its sleep, App Nap and Go's monotonic clock may behave differently._

## Decision
**Decided (2026-09-28, Windows):**
- **Scheduler (SPEC 6.2):**
  - The one-minute wall-clock check stays the main mechanism.
  - It also runs when `PowerRegisterSuspendResumeNotification` reports a resume. The call goes through `golang.org/x/sys/windows`, so no cgo is needed.
  - Suspend and resume times are logged, so a late run can show "laptop asleep 16:54–17:19".
- **Don't rely on Wails' sleep and wake events** on Windows: they never arrived on Modern Standby.
- **Job times are never long timers.** Long timers happened to work here, but they don't follow clock changes (DST, manual changes), and macOS may differ.
- **Lock screen:** scheduled runs read secrets while the screen is locked with no special handling. Jenab detects locking with session notifications, never the input desktop name.
