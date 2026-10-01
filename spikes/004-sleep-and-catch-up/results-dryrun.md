# SPIKE-004 sleep watch: results

Log `dryrun.jsonl`: 47 lines, 2026-09-28 12:13:34 to 2026-09-28 12:16:54 local, 3.3 min of wall time. End: stopped by --for 3m20s reached, keychain delete: ok.

- pid 22988, go1.26.0, windows/amd64
- args: watch --jobs 12:11:34,12:14:49,12:15:49,12:16:04 --log dryrun.jsonl --for 3m20s
- log: D:\DEV\Burrow\spikes\004-sleep-and-catch-up\dryrun.jsonl, stop file: D:\DEV\Burrow\spikes\004-sleep-and-catch-up\stop
- timezone: +0330 +03:30
- QueryInterruptTime: ok, QueryUnbiasedInterruptTime: ok
- job 2026-09-28T12:11:34+03:30
- job 2026-09-28T12:14:49+03:30
- job 2026-09-28T12:15:49+03:30
- job 2026-09-28T12:16:04+03:30
- keychain set burrow-spike004:API_KEY: ok
- RegisterPowerSettingNotification GUID_CONSOLE_DISPLAY_STATE: ok
- RegisterPowerSettingNotification GUID_MONITOR_POWER_ON: ok
- RegisterPowerSettingNotification GUID_LIDSWITCH_STATE_CHANGE: ok
- RegisterPowerSettingNotification GUID_SYSTEM_AWAYMODE: ok
- RegisterPowerSettingNotification GUID_ACDC_POWER_SOURCE: ok
- RegisterPowerSettingNotification GUID_SESSION_DISPLAY_STATUS: ok
- RegisterPowerSettingNotification GUID_SESSION_USER_PRESENCE: ok
- WTSRegisterSessionNotification: ok
- PowerRegisterSuspendResumeNotification: ok

## 1. Event sources

"Wails v3" is what v3.0.0-beta.26 `pkg/application/application_windows.go` does: its hidden top-level main-thread window turns `WM_POWERBROADCAST` into application events. It registers no power settings, no suspend/resume callback and no session notifications.

| Source | Event | Seen | Exposed by Wails v3 beta.26 |
|---|---|---|---|
| `WM_POWERBROADCAST` (hidden window) | `PBT_APMSUSPEND` | 0 | yes: `events.Windows.APMSuspend`, also as `events.Common.SystemWillSleep` |
| `WM_POWERBROADCAST` (hidden window) | `PBT_APMRESUMEAUTOMATIC` | 0 | yes: `events.Windows.APMResumeAutomatic`, also as `events.Common.SystemDidWake` |
| `WM_POWERBROADCAST` (hidden window) | `PBT_APMRESUMESUSPEND` | 0 | yes: `events.Windows.APMResumeSuspend` |
| `WM_POWERBROADCAST` (hidden window) | `PBT_APMPOWERSTATUSCHANGE` | 0 | yes: `events.Windows.APMPowerStatusChange` |
| `WM_POWERBROADCAST` `PBT_POWERSETTINGCHANGE` | `GUID_CONSOLE_DISPLAY_STATE` | 1 | only if the app registers the GUID on Wails' window itself; Wails then raises `APMPowerSettingChange` without the GUID or value |
| `WM_POWERBROADCAST` `PBT_POWERSETTINGCHANGE` | `GUID_MONITOR_POWER_ON` | 1 | only if the app registers the GUID on Wails' window itself; Wails then raises `APMPowerSettingChange` without the GUID or value |
| `WM_POWERBROADCAST` `PBT_POWERSETTINGCHANGE` | `GUID_LIDSWITCH_STATE_CHANGE` | 1 | only if the app registers the GUID on Wails' window itself; Wails then raises `APMPowerSettingChange` without the GUID or value |
| `WM_POWERBROADCAST` `PBT_POWERSETTINGCHANGE` | `GUID_SYSTEM_AWAYMODE` | 0 | only if the app registers the GUID on Wails' window itself; Wails then raises `APMPowerSettingChange` without the GUID or value |
| `WM_POWERBROADCAST` `PBT_POWERSETTINGCHANGE` | `GUID_ACDC_POWER_SOURCE` | 1 | only if the app registers the GUID on Wails' window itself; Wails then raises `APMPowerSettingChange` without the GUID or value |
| `WM_POWERBROADCAST` `PBT_POWERSETTINGCHANGE` | `GUID_SESSION_DISPLAY_STATUS` | 1 | only if the app registers the GUID on Wails' window itself; Wails then raises `APMPowerSettingChange` without the GUID or value |
| `WM_POWERBROADCAST` `PBT_POWERSETTINGCHANGE` | `GUID_SESSION_USER_PRESENCE` | 1 | only if the app registers the GUID on Wails' window itself; Wails then raises `APMPowerSettingChange` without the GUID or value |
| `PowerRegisterSuspendResumeNotification` callback | `PBT_APMSUSPEND` | 0 | no |
| `PowerRegisterSuspendResumeNotification` callback | `PBT_APMRESUMEAUTOMATIC` | 0 | no |
| `PowerRegisterSuspendResumeNotification` callback | `PBT_APMRESUMESUSPEND` | 0 | no |
| `WTSRegisterSessionNotification` | lock | 0 | no |
| `WTSRegisterSessionNotification` | unlock | 0 | no |

## 2. Timeline

All OS events, scheduler runs, the AfterFunc and job timers, probe fires more than 5 s late, keychain reads not from the minute loop, and desktop changes. A row in italics marks a silence in the log. Windows sends each registered power setting's current value right after registration, so the first `setting` rows are not changes.

| Local | Since start | Kind | Event | Detail | Desktop |
|---|---|---|---|---|---|
| 12:13:34 | +0:00 | setting | GUID_CONSOLE_DISPLAY_STATE | on (1) |  |
| 12:13:34 | +0:00 | start |  | 4 jobs | Default |
| 12:13:34 | +0:00 | sched | spec scheduler | **catch_up** run for 12:11:34, 2.0 min after the first, check: start |  |
| 12:13:34 | +0:00 | setting | GUID_LIDSWITCH_STATE_CHANGE | open (1) |  |
| 12:13:34 | +0:00 | sched | tick_only scheduler | **catch_up** run for 12:11:34, 2.0 min after the first, check: start |  |
| 12:13:34 | +0:00 | sched | spec scheduler | check (wake: display on): nothing due |  |
| 12:13:34 | +0:00 | setting | GUID_ACDC_POWER_SOURCE | AC (0) |  |
| 12:13:34 | +0:00 | setting | GUID_SESSION_DISPLAY_STATUS | on (1) |  |
| 12:13:34 | +0:00 | setting | GUID_MONITOR_POWER_ON | on (1) |  |
| 12:13:34 | +0:00 | setting | GUID_SESSION_USER_PRESENCE | present (0) |  |
| 12:14:49 | +1:14 | probe | timer_job | job 12:14:49, fired 0.0 s after it |  |
| 12:15:01 | +1:26 | sched | spec scheduler | **on_time** run for 12:14:49, 12.0 s after the first, check: tick |  |
| 12:15:01 | +1:26 | sched | tick_only scheduler | **on_time** run for 12:14:49, 12.0 s after the first, check: tick |  |
| 12:15:49 | +2:14 | probe | timer_job | job 12:15:49, fired 0.0 s after it |  |
| 12:16:01 | +2:26 | sched | spec scheduler | **on_time** run for 12:15:49, 12.0 s after the first, check: tick |  |
| 12:16:01 | +2:26 | sched | tick_only scheduler | **on_time** run for 12:15:49, 12.0 s after the first, check: tick |  |
| 12:16:04 | +2:29 | probe | timer_job | job 12:16:04, fired 0.0 s after it |  |
| 12:16:54 | +3:20 | stop |  | --for 3m20s reached; keychain delete: ok | Default |

## 3. Sleep intervals

**From suspend/resume events.** "Asleep" is the growth of (interrupt time − unbiased interrupt time) between the two events. "Lines written" are log lines between the events other than power events: if there are any, the process ran during sleep.

| Source | Suspend | Resume | Wall | Asleep (counters) | Go monotonic | QPC | GetTickCount64 | Lines written in between |
|---|---|---|---|---|---|---|---|---|
| none | | | | | | | | |

**From the counters and log gaps** (consecutive lines more than 25.0 s apart, or more than 1 s asleep between them; touching spans are merged):

| From (last line before) | To (first line after) | Wall | Asleep (counters) | Interrupt time | Unbiased interrupt time | Go monotonic | QPC | GetTickCount64 | Lines inside |
|---|---|---|---|---|---|---|---|---|---|
| none | | | | | | | | | |

## 4. Wakes and what fired after them

No wake detected.

**All probe fires** (late = more than 5 s off the wall clock):

| Probe | Fires | On time | Late fires |
|---|---|---|---|
| `ticker_1m` | 3 | 3 |  |
| `sleep_1m` | 3 | 3 |  |
| `afterfunc_10m` | 0 | 0 |  |
| `timer_job` | 3 | 3 |  |

## 5. Scheduled jobs

*spec* = SPEC 6.2 scheduler: check at start, every minute (1-minute ticker) and on each resume or display-on event. *tick-only* = the same without OS events. *naive* = one `time.Timer` per job, duration computed once at start. "After wake" is filled when the job time fell before the wake that preceded the run.

| Job | spec | tick-only | naive timer |
|---|---|---|---|
| 12:11:34 | **caught up** at 12:13:34, 2.0 min after the job time (start) | **caught up** at 12:13:34, 2.0 min after the job time (start) | not armed (time before start) |
| 12:14:49 | **on time** at 12:15:01, 12.0 s after the job time (tick) | **on time** at 12:15:01, 12.0 s after the job time (tick) | **on time**: fired at 12:14:49, 0.0 s after the job time |
| 12:15:49 | **on time** at 12:16:01, 12.0 s after the job time (tick) | **on time** at 12:16:01, 12.0 s after the job time (tick) | **on time**: fired at 12:15:49, 0.0 s after the job time |
| 12:16:04 | **missed** (no run by the end of the log) | **missed** (no run by the end of the log) | **on time**: fired at 12:16:04, 0.0 s after the job time |

## 6. Keychain reads (SPIKE-006 lock-screen row)

One `go-keyring` read of `burrow-spike004:API_KEY` every minute, plus on lock/unlock (at once and 5 s later) and on resume. Desktop `Default` = unlocked; `Winlogon` or no access = lock screen.

| Input desktop | Reads | ok | Errors | Median | Max |
|---|---|---|---|---|---|
| Default | 4 | 4 |  | 0.64 ms | 18.14 ms |

## 7. Clocks over the whole log

| Clock | Elapsed first → last line | Minus wall time |
|---|---|---|
| wall (`time.Now`, UTC) | 3.3 min | 0.0 s |
| Go monotonic (`time.Since`) | 3.3 min | 0.0 s |
| `QueryPerformanceCounter` | 3.3 min | 0.0 s |
| `QueryInterruptTime` | 3.3 min | 0.0 s |
| `QueryUnbiasedInterruptTime` | 3.3 min | 0.0 s |
| `GetTickCount64` | 3.3 min | 0.0 s |

Asleep over the run per the counters: 0.0 s.
