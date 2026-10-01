# SPIKE-004 sleep watch: results

Log `sleep.jsonl`: 470 lines, 2026-09-28 16:19:53 to 2026-09-28 17:29:43 local, 69.8 min of wall time. End: stopped by stop file, keychain delete: ok.

- pid 23612, go1.26.0, windows/amd64
- args: watch --jobs 16:34,16:44,16:54
- log: D:\DEV\Burrow\spikes\004-sleep-and-catch-up\sleep.jsonl, stop file: D:\DEV\Burrow\spikes\004-sleep-and-catch-up\stop
- timezone: +0330 +03:30
- QueryInterruptTime: ok, QueryUnbiasedInterruptTime: ok
- job 2026-09-28T16:34:00+03:30
- job 2026-09-28T16:44:00+03:30
- job 2026-09-28T16:54:00+03:30
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
| `WM_POWERBROADCAST` (hidden window) | `PBT_APMPOWERSTATUSCHANGE` | 2 | yes: `events.Windows.APMPowerStatusChange` |
| `WM_POWERBROADCAST` `PBT_POWERSETTINGCHANGE` | `GUID_CONSOLE_DISPLAY_STATE` | 5 | only if the app registers the GUID on Wails' window itself; Wails then raises `APMPowerSettingChange` without the GUID or value |
| `WM_POWERBROADCAST` `PBT_POWERSETTINGCHANGE` | `GUID_MONITOR_POWER_ON` | 5 | only if the app registers the GUID on Wails' window itself; Wails then raises `APMPowerSettingChange` without the GUID or value |
| `WM_POWERBROADCAST` `PBT_POWERSETTINGCHANGE` | `GUID_LIDSWITCH_STATE_CHANGE` | 3 | only if the app registers the GUID on Wails' window itself; Wails then raises `APMPowerSettingChange` without the GUID or value |
| `WM_POWERBROADCAST` `PBT_POWERSETTINGCHANGE` | `GUID_SYSTEM_AWAYMODE` | 0 | only if the app registers the GUID on Wails' window itself; Wails then raises `APMPowerSettingChange` without the GUID or value |
| `WM_POWERBROADCAST` `PBT_POWERSETTINGCHANGE` | `GUID_ACDC_POWER_SOURCE` | 3 | only if the app registers the GUID on Wails' window itself; Wails then raises `APMPowerSettingChange` without the GUID or value |
| `WM_POWERBROADCAST` `PBT_POWERSETTINGCHANGE` | `GUID_SESSION_DISPLAY_STATUS` | 5 | only if the app registers the GUID on Wails' window itself; Wails then raises `APMPowerSettingChange` without the GUID or value |
| `WM_POWERBROADCAST` `PBT_POWERSETTINGCHANGE` | `GUID_SESSION_USER_PRESENCE` | 9 | only if the app registers the GUID on Wails' window itself; Wails then raises `APMPowerSettingChange` without the GUID or value |
| `PowerRegisterSuspendResumeNotification` callback | `PBT_APMSUSPEND` | 4 | no |
| `PowerRegisterSuspendResumeNotification` callback | `PBT_APMRESUMEAUTOMATIC` | 4 | no |
| `PowerRegisterSuspendResumeNotification` callback | `PBT_APMRESUMESUSPEND` | 4 | no |
| `WTSRegisterSessionNotification` | lock | 1 | no |
| `WTSRegisterSessionNotification` | unlock | 1 | no |

## 2. Timeline

All OS events, scheduler runs, the AfterFunc and job timers, probe fires more than 5 s late, keychain reads not from the minute loop, and desktop changes. A row in italics marks a silence in the log. Windows sends each registered power setting's current value right after registration, so the first `setting` rows are not changes.

| Local | Since start | Kind | Event | Detail | Desktop |
|---|---|---|---|---|---|
| 16:19:53 | +0:00 | setting | GUID_ACDC_POWER_SOURCE | AC (0) |  |
| 16:19:53 | +0:00 | start |  | 3 jobs | Default |
| 16:19:53 | +0:00 | setting | GUID_CONSOLE_DISPLAY_STATE | on (1) |  |
| 16:19:53 | +0:00 | setting | GUID_MONITOR_POWER_ON | on (1) |  |
| 16:19:53 | +0:00 | sched | spec scheduler | check (start): nothing due |  |
| 16:19:53 | +0:00 | sched | spec scheduler | check (wake: display on): nothing due |  |
| 16:19:53 | +0:00 | sched | tick_only scheduler | check (start): nothing due |  |
| 16:19:53 | +0:00 | setting | GUID_LIDSWITCH_STATE_CHANGE | open (1) |  |
| 16:19:53 | +0:00 | setting | GUID_SESSION_DISPLAY_STATUS | on (1) |  |
| 16:19:53 | +0:00 | setting | GUID_SESSION_USER_PRESENCE | present (0) |  |
| 16:28:32 | +8:39 | setting | GUID_SESSION_USER_PRESENCE | not present (2) |  |
| 16:28:46 | +8:53 | setting | GUID_SESSION_USER_PRESENCE | present (0) |  |
| 16:29:53 | +10:00 | probe | afterfunc_10m | 0.0 s late (wall 10.0 min, Go monotonic 10.0 min) |  |
| 16:32:57 | +13:04 | setting | GUID_SESSION_USER_PRESENCE | not present (2) |  |
| 16:34:00 | +14:06 | probe | timer_job | job 16:34:00, fired 0.0 s after it |  |
| 16:34:01 | +14:07 | sched | tick_only scheduler | **on_time** run for 16:34:00, 1.0 s after the first, check: tick |  |
| 16:34:01 | +14:07 | sched | spec scheduler | **on_time** run for 16:34:00, 1.0 s after the first, check: tick |  |
| 16:38:40 | +18:47 | setting | GUID_SESSION_USER_PRESENCE | present (0) |  |
| 16:42:55 | +23:02 | setting | GUID_SESSION_USER_PRESENCE | not present (2) |  |
| 16:42:55 | +23:02 | setting | GUID_CONSOLE_DISPLAY_STATE | off (0) |  |
| 16:42:55 | +23:02 | setting | GUID_MONITOR_POWER_ON | off (0) |  |
| 16:42:55 | +23:02 | powercb | PBT_APMSUSPEND |  | Default |
| 16:42:58 | +23:05 | powercb | PBT_APMRESUMEAUTOMATIC |  | Default |
| 16:42:58 | +23:05 | powercb | PBT_APMRESUMESUSPEND |  | Default |
| 16:42:58 | +23:05 | keychain | read (resume) | ok, 0.53 ms | Default |
| 16:42:58 | +23:05 | sched | spec scheduler | check (wake: callback PBT_APMRESUMEAUTOMATIC): nothing due |  |
| 16:42:58 | +23:05 | sched | spec scheduler | check (wake: callback PBT_APMRESUMESUSPEND): nothing due |  |
| 16:42:58 | +23:05 | keychain | read (resume) | ok, 0.36 ms | Default |
| 16:42:58 | +23:05 | setting | GUID_SESSION_DISPLAY_STATUS | off (0) |  |
| 16:42:58 | +23:05 | setting | GUID_SESSION_USER_PRESENCE | present (0) |  |
| 16:42:58 | +23:05 | setting | GUID_SESSION_DISPLAY_STATUS | on (1) |  |
| 16:42:58 | +23:05 | setting | GUID_MONITOR_POWER_ON | on (1) |  |
| 16:42:58 | +23:05 | setting | GUID_CONSOLE_DISPLAY_STATE | on (1) |  |
| 16:42:58 | +23:05 | sched | spec scheduler | check (wake: display on): nothing due |  |
| 16:44:00 | +24:06 | probe | timer_job | job 16:44:00, fired 0.0 s after it |  |
| 16:44:01 | +24:07 | sched | tick_only scheduler | **on_time** run for 16:44:00, 1.0 s after the first, check: tick |  |
| 16:44:01 | +24:07 | sched | spec scheduler | **on_time** run for 16:44:00, 1.0 s after the first, check: tick |  |
| 16:47:18 | +27:24 | setting | GUID_SESSION_USER_PRESENCE | not present (2) |  |
| 16:51:43 | +31:50 | setting | GUID_CONSOLE_DISPLAY_STATE | off (0) |  |
| 16:51:43 | +31:50 | setting | GUID_SESSION_DISPLAY_STATUS | off (0) |  |
| 16:51:43 | +31:50 | setting | GUID_MONITOR_POWER_ON | off (0) |  |
| 16:51:43 | +31:50 | powercb | PBT_APMSUSPEND |  | Default |
| 16:51:45 | +31:51 | powercb | PBT_APMRESUMEAUTOMATIC |  | Default |
| 16:51:45 | +31:51 | sched | spec scheduler | check (wake: callback PBT_APMRESUMEAUTOMATIC): nothing due |  |
| 16:51:45 | +31:51 | powercb | PBT_APMRESUMESUSPEND |  | Default |
| 16:51:45 | +31:51 | keychain | read (resume) | ok, 7.37 ms | Default |
| 16:51:45 | +31:51 | sched | spec scheduler | check (wake: callback PBT_APMRESUMESUSPEND): nothing due |  |
| 16:51:45 | +31:51 | keychain | read (resume) | ok, 2.40 ms | Default |
| 16:51:45 | +31:52 | setting | GUID_LIDSWITCH_STATE_CHANGE | closed (0) |  |
| 16:51:45 | +31:52 | setting | GUID_ACDC_POWER_SOURCE | battery (1) |  |
| 16:51:45 | +31:52 | power | PBT_APMPOWERSTATUSCHANGE |  | Default |
| 16:51:50 | +31:56 | powercb | PBT_APMSUSPEND |  | Default |
| *16:51:50–16:54:51* | | | *no log lines for 3.0 min* | *asleep per counters: 3.0 min; Go monotonic advanced 3.0 min* | |
| 16:54:51 | +34:58 | probe | sleep_1m | 3.0 min late (wall 4.0 min, Go monotonic 4.0 min) |  |
| 16:54:51 | +34:58 | probe | timer_job | job 16:54:00, fired 51.8 s after it |  |
| 16:54:51 | +34:58 | sched | spec scheduler | **on_time** run for 16:54:00, 51.8 s after the first, check: tick |  |
| 16:54:51 | +34:58 | sched | tick_only scheduler | **on_time** run for 16:54:00, 51.8 s after the first, check: tick |  |
| 16:54:51 | +34:58 | probe | ticker_1m | 3.0 min late (wall 4.0 min, Go monotonic 4.0 min) |  |
| 16:54:51 | +34:58 | powercb | PBT_APMRESUMEAUTOMATIC |  | Default |
| 16:54:51 | +34:58 | powercb | PBT_APMRESUMESUSPEND |  | Default |
| 16:54:51 | +34:58 | sched | spec scheduler | check (wake: callback PBT_APMRESUMEAUTOMATIC): nothing due |  |
| 16:54:52 | +34:59 | keychain | read (resume) | ok, 13.61 ms | Default |
| 16:54:52 | +34:59 | keychain | read (resume) | ok, 2.30 ms | Default |
| 16:54:52 | +34:59 | sched | spec scheduler | check (wake: callback PBT_APMRESUMESUSPEND): nothing due |  |
| 16:54:53 | +35:00 | probe | ticker_1m | -58.7 s late (wall 1.3 s, Go monotonic 1.3 s) |  |
| 16:54:56 | +35:03 | powercb | PBT_APMSUSPEND |  | Default |
| *16:54:56–17:19:37* | | | *no log lines for 24.7 min* | *asleep per counters: 24.7 min; Go monotonic advanced 24.7 min* | |
| 17:19:37 | +59:44 | powercb | PBT_APMRESUMEAUTOMATIC |  | Default |
| 17:19:38 | +59:44 | powercb | PBT_APMRESUMESUSPEND |  | Default |
| 17:19:38 | +59:45 | sched | spec scheduler | check (wake: callback PBT_APMRESUMEAUTOMATIC): nothing due |  |
| 17:19:38 | +59:45 | sched | spec scheduler | check (wake: callback PBT_APMRESUMESUSPEND): nothing due |  |
| 17:19:39 | +59:45 | probe | sleep_1m | 23.8 min late (wall 24.8 min, Go monotonic 24.8 min) |  |
| 17:19:39 | +59:46 | power | PBT_APMPOWERSTATUSCHANGE |  | Default |
| 17:19:39 | +59:46 | keychain | read (resume) | ok, 12.76 ms | Default |
| 17:19:39 | +59:46 | probe | ticker_1m | 23.8 min late (wall 24.8 min, Go monotonic 24.8 min) |  |
| 17:19:39 | +59:46 | keychain | read (resume) | ok, 82.13 ms | Default |
| 17:19:39 | +59:46 | setting | GUID_ACDC_POWER_SOURCE | AC (0) |  |
| 17:19:39 | +59:46 | session | lock | session 1 | Default |
| 17:19:39 | +59:46 | keychain | read (lock) | ok, 1.15 ms | Default |
| 17:19:42 | +59:49 | setting | GUID_LIDSWITCH_STATE_CHANGE | open (1) |  |
| 17:19:42 | +59:49 | setting | GUID_SESSION_USER_PRESENCE | present (0) |  |
| 17:19:42 | +59:49 | setting | GUID_CONSOLE_DISPLAY_STATE | on (1) |  |
| 17:19:43 | +59:49 | setting | GUID_MONITOR_POWER_ON | on (1) |  |
| 17:19:43 | +59:49 | setting | GUID_SESSION_DISPLAY_STATUS | on (1) |  |
| 17:19:43 | +59:50 | sched | spec scheduler | check (wake: display on): nothing due |  |
| 17:19:44 | +59:51 | keychain | read (lock+5s) | ok, 0.37 ms | Default |
| 17:19:53 | +60:00 | probe | ticker_1m | -45.1 s late (wall 14.9 s, Go monotonic 14.9 s) |  |
| 17:20:02 | +60:09 | session | unlock | session 1 | Default |
| 17:20:02 | +60:09 | keychain | read (unlock) | ok, 0.70 ms | Default |
| 17:20:07 | +60:14 | keychain | read (unlock+5s) | ok, 0.58 ms | Default |
| 17:29:43 | +69:50 | stop |  | stop file; keychain delete: ok | Default |

## 3. Sleep intervals

**From suspend/resume events.** "Asleep" is the growth of (interrupt time − unbiased interrupt time) between the two events. "Lines written" are log lines between the events other than power events: if there are any, the process ran during sleep.

| Source | Suspend | Resume | Wall | Asleep (counters) | Go monotonic | QPC | GetTickCount64 | Lines written in between |
|---|---|---|---|---|---|---|---|---|
| callback | 16:42:55 | 16:42:58 | 2.9 s | 2.9 s | 2.9 s | 2.9 s | 2.9 s | 0  |
| callback | 16:51:43 | 16:51:45 | 1.6 s | 1.6 s | 1.6 s | 1.6 s | 1.6 s | 0  |
| callback | 16:51:50 | 16:54:51 | 3.0 min | 3.0 min | 3.0 min | 3.0 min | 3.0 min | 6 (probe 3, sample 1, sched 2) |
| callback | 16:54:56 | 17:19:37 | 24.7 min | 24.7 min | 24.7 min | 24.7 min | 24.7 min | 0  |

**From the counters and log gaps** (consecutive lines more than 25.0 s apart, or more than 1 s asleep between them; touching spans are merged):

| From (last line before) | To (first line after) | Wall | Asleep (counters) | Interrupt time | Unbiased interrupt time | Go monotonic | QPC | GetTickCount64 | Lines inside |
|---|---|---|---|---|---|---|---|---|---|
| 16:42:55 | 16:42:58 | 2.9 s | 2.9 s | 2.9 s | 0.0 s | 2.9 s | 2.9 s | 2.9 s | 0  |
| 16:51:43 | 16:51:45 | 1.6 s | 1.6 s | 1.6 s | 0.0 s | 1.6 s | 1.6 s | 1.6 s | 0  |
| 16:51:50 | 16:54:51 | 3.0 min | 3.0 min | 3.0 min | 0.0 s | 3.0 min | 3.0 min | 3.0 min | 0  |
| 16:54:56 | 17:19:37 | 24.7 min | 24.7 min | 24.7 min | 0.0 s | 24.7 min | 24.7 min | 24.7 min | 0  |

## 4. Wakes and what fired after them

### Wake 1 at 16:42:58 (+23:05)

Signals: callback PBT_APMRESUMEAUTOMATIC; interrupt-time counters show 2.9 s asleep; callback PBT_APMRESUMESUSPEND.

| Timer or loop | First fire after the wake | Seconds after the wake | Late vs its own schedule |
|---|---|---|---|
| 10 s sample ticker | 16:43:03 | 4.9 s |  |
| `time.Ticker(1 min)` | 16:43:53 | 54.9 s | 0.0 s |
| `time.Sleep(1 min)` loop | 16:43:53 | 55.0 s | 0.0 s |
| `time.AfterFunc(10 min)` | not before the next wake / end | | |
| job `time.Timer` (naive scheduler) | 16:44:00 | 61.8 s | 0.0 s |
| keychain minute read | 16:43:53 | 54.9 s |  |
| spec scheduler run | 16:44:01 | 62.7 s | on_time, 1.0 s after the scheduled time (tick) |
| tick-only scheduler run | 16:44:01 | 62.7 s | on_time, 1.0 s after the scheduled time (tick) |

### Wake 2 at 16:51:45 (+31:51)

Signals: callback PBT_APMRESUMEAUTOMATIC; interrupt-time counters show 1.6 s asleep; callback PBT_APMRESUMESUSPEND.

| Timer or loop | First fire after the wake | Seconds after the wake | Late vs its own schedule |
|---|---|---|---|
| 10 s sample ticker | not before the next wake / end | | |
| `time.Ticker(1 min)` | not before the next wake / end | | |
| `time.Sleep(1 min)` loop | not before the next wake / end | | |
| `time.AfterFunc(10 min)` | not before the next wake / end | | |
| job `time.Timer` (naive scheduler) | not before the next wake / end | | |
| keychain minute read | not before the next wake / end | | |
| spec scheduler run | not before the next wake / end | | |
| tick-only scheduler run | not before the next wake / end | | |

### Wake 3 at 16:54:51 (+34:58)

Signals: first line after a 3.0 min log gap; callback PBT_APMRESUMEAUTOMATIC; callback PBT_APMRESUMESUSPEND.

| Timer or loop | First fire after the wake | Seconds after the wake | Late vs its own schedule |
|---|---|---|---|
| 10 s sample ticker | 16:54:51 | 0.1 s |  |
| `time.Ticker(1 min)` | 16:54:51 | 0.1 s | 3.0 min |
| `time.Sleep(1 min)` loop | 16:54:51 | 0.0 s | 3.0 min |
| `time.AfterFunc(10 min)` | not before the next wake / end | | |
| job `time.Timer` (naive scheduler) | 16:54:51 | 0.1 s | 51.8 s |
| keychain minute read | 16:54:52 | 0.2 s |  |
| spec scheduler run | 16:54:51 | 0.1 s | on_time, 51.8 s after the scheduled time (tick) |
| tick-only scheduler run | 16:54:51 | 0.1 s | on_time, 51.8 s after the scheduled time (tick) |

### Wake 4 at 17:19:37 (+59:44)

Signals: callback PBT_APMRESUMEAUTOMATIC; first line after a 24.7 min log gap; callback PBT_APMRESUMESUSPEND.

| Timer or loop | First fire after the wake | Seconds after the wake | Late vs its own schedule |
|---|---|---|---|
| 10 s sample ticker | 17:19:38 | 1.0 s |  |
| `time.Ticker(1 min)` | 17:19:39 | 1.5 s | 23.8 min |
| `time.Sleep(1 min)` loop | 17:19:39 | 1.3 s | 23.8 min |
| `time.AfterFunc(10 min)` | not before the next wake / end | | |
| job `time.Timer` (naive scheduler) | not before the next wake / end | | |
| keychain minute read | 17:19:39 | 1.5 s |  |
| spec scheduler run | not before the next wake / end | | |
| tick-only scheduler run | not before the next wake / end | | |


**All probe fires** (late = more than 5 s off the wall clock):

| Probe | Fires | On time | Late fires |
|---|---|---|---|
| `ticker_1m` | 44 | 40 | 16:54:51: 3.0 min late (Go monotonic 4.0 min); 16:54:53: -58.7 s late (Go monotonic 1.3 s); 17:19:39: 23.8 min late (Go monotonic 24.8 min); 17:19:53: -45.1 s late (Go monotonic 14.9 s) |
| `sleep_1m` | 43 | 41 | 16:54:51: 3.0 min late (Go monotonic 4.0 min); 17:19:39: 23.8 min late (Go monotonic 24.8 min) |
| `afterfunc_10m` | 1 | 1 |  |
| `timer_job` | 3 | 2 | 16:54:51: 51.8 s late (Go monotonic 35.0 min) |

## 5. Scheduled jobs

*spec* = SPEC 6.2 scheduler: check at start, every minute (1-minute ticker) and on each resume or display-on event. *tick-only* = the same without OS events. *naive* = one `time.Timer` per job, duration computed once at start. "After wake" is filled when the job time fell before the wake that preceded the run.

| Job | spec | tick-only | naive timer |
|---|---|---|---|
| 16:34:00 | **on time** at 16:34:01, 1.0 s after the job time (tick) | **on time** at 16:34:01, 1.0 s after the job time (tick) | **on time**: fired at 16:34:00, 0.0 s after the job time |
| 16:44:00 | **on time** at 16:44:01, 1.0 s after the job time (tick) | **on time** at 16:44:01, 1.0 s after the job time (tick) | **on time**: fired at 16:44:00, 0.0 s after the job time |
| 16:54:00 | **on time** at 16:54:51, 51.9 s after the job time, 0.1 s after wake (tick) | **on time** at 16:54:51, 51.9 s after the job time, 0.1 s after wake (tick) | **on time**: fired at 16:54:51, 51.8 s after the job time, 0.1 s after wake |

## 6. Keychain reads (SPIKE-006 lock-screen row)

One `go-keyring` read of `burrow-spike004:API_KEY` every minute, plus on lock/unlock (at once and 5 s later) and on resume. Desktop `Default` = unlocked; `Winlogon` or no access = lock screen.

| Input desktop | Reads | ok | Errors | Median | Max |
|---|---|---|---|---|---|
| Default | 57 | 57 |  | 0.64 ms | 82.13 ms |

## 7. Clocks over the whole log

| Clock | Elapsed first → last line | Minus wall time |
|---|---|---|
| wall (`time.Now`, UTC) | 69.8 min | 0.0 s |
| Go monotonic (`time.Since`) | 69.8 min | 0.0 s |
| `QueryPerformanceCounter` | 69.8 min | -0.0 s |
| `QueryInterruptTime` | 69.8 min | 0.0 s |
| `QueryUnbiasedInterruptTime` | 42.0 min | -27.8 min |
| `GetTickCount64` | 69.8 min | 0.0 s |

Asleep over the run per the counters: 27.8 min.
