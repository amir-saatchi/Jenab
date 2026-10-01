# SPIKE-006 results (Windows)

zalando/go-keyring v0.2.8 (danieljoos/wincred v1.2.3), Go go1.26.0, 2026-09-27.
Harness: PID 10796, session 1, screen unlocked (WTS flag 0).

## 1. Store (foreground process)

| Name | Value size | Result |
|---|---|---|
| `API_KEY` | 48 bytes | ok |
| `MAX_SIZE` | 2560 bytes | ok |
| `UNICODE` | 24 bytes | ok |
| `EMPTY` | 0 bytes | ok |
| `LONG_NAME_NNNNNNNNNN… (410 bytes)` | 1 bytes | ok |
| `TOO_BIG` | 2561 bytes | error: data passed to Set was too big |
| `TOO_BIG_UNICODE` | 2562 bytes | error: data passed to Set was too big |

Reading a name that does not exist: error: secret not found in keyring (`errors.Is(err, keyring.ErrNotFound)` = true).

## 2. Read speed

1000 reads of `API_KEY` in the same process: 143.3 µs per read.

## 3. Reads from windowless background processes

`kc-bg.exe` is built with `-H windowsgui` (no console, no window), like Burrow in tray mode.

| How it was started | Console | Parent PID | Session | Screen | Reads ok | Time for all reads | Prompt seen |
|---|---|---|---|---|---|---|---|
| detached from the harness (`DETACHED_PROCESS`) | false | 10796 | 1 | unlocked (WTS flag 0) | 5 of 5 | 3.5274ms | no |
| by Explorer (`explorer.exe kc-bg.exe`), as from the Start menu or at login | false | 21192 | 1 | unlocked (WTS flag 0) | 5 of 5 | 3.3017ms | no |
| hidden, by PowerShell `Start-Process -WindowStyle Hidden` | false | 18788 | 1 | unlocked (WTS flag 0) | 5 of 5 | 1.6304ms | no |

## 4. Other programs of the same user

`other.exe`, a separate program that calls `CredReadW` directly (not go-keyring): read the value (48 bytes, starts with "sk-test-aaaa…"), persist mode 2, no prompt.

`cmdkey /list:burrow-spike006*` (the built-in tool; shows names, not values):

```
Currently stored credentials for burrow-spike006*:

    Target: burrow-spike006:API_KEY
    Type: Generic 
    User: API_KEY
    Local machine persistence
    
    Target: burrow-spike006:LONG_NAME_NNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNN
    Type: Generic 
    User: LONG_NAME_NNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNN
    Local machine persistence
    
    Target: burrow-spike006:MAX_SIZE
    Type: Generic 
    User: MAX_SIZE
    Local machine persistence
    
    Target: burrow-spike006:UNICODE
    Type: Generic 
    User: UNICODE
    Local machine persistence
    
    Target: burrow-spike006:EMPTY
    Type: Generic 
    User: EMPTY
    Local machine persistence
```

## 5. Delete

`keyring.DeleteAll("burrow-spike006")`: ok. Entries left: 0.
