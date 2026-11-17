# TASK-073 report: crash log and updates, part 2

Branch `claude/task-073-crash-and-updates`, from main at 0e68d3c. Docs/ was not edited.

## 1. Restart now during a download

Before, a click on Restart now while a newer version downloaded did nothing. The request waited in `install` until the download ended.

Now Restart now stops the download and restarts into the installed version (`internal/update/update.go`, `install`). The service waits for the stopped download to return before it restarts, so the swap never runs half done. If the swap had already finished, the newer version counts as installed and the restart goes into it. The Settings line still hides the button while a download runs; this covers the race and any other caller.

Test: `TestRestartNowDuringADownload`. 1.1.0 is installed and 1.2.0 hangs while downloading. Restart now gives one relaunch, the download is stopped, and the status is Ready 1.1.0.

## 2. Off and on again during a download

Before, turning updates off and back on during a download left the next check to the 6-hour timer.

Now, when off stopped a download and on came back before the download returned, `Run` checks at once, as a start does. A fast toggle that the service only reads as on lets the download carry on, which is the same result. If the download returns before on is read, the old path (it was off, now it's on, so check) still covers it.

Test: `TestOffAndOnDuringADownloadChecks`. A download runs, is turned off, is stopped, is turned on again, and only then returns. A second check and install start at once, with the timer at an hour.

## 3. The raw crash file

Before, if the redacted log couldn't be written, the `.raw` file, which may hold keys, stayed on disk for good.

Now `crash.Collect` counts failed tries in a `crash-*.raw.tries` file next to it. At the third failed try (`MaxTries`), or at the first failed try once the file is 7 days old (`MaxAge`), it deletes the raw file and its count. It returns the file's name in `Result.Lost`. A count whose raw file is gone is removed.

The start then logs that the crash log couldn't be kept and shows a sticky warning: "Jenab closed unexpectedly. Its crash log couldn't be written, so what the crash left was deleted: it can hold keys. Nothing was sent anywhere." There is no button. A toast without a button shows no detail line, so the whole sentence is in the text.

`Collect` now returns a `Result` (`Logs`, `Lost`) in place of the list of logs. `cmd/desktop` was changed to match.

Tests:
- `TestAnOutputThatCantBeKeptGoesAfterAFewTries`: the first two failed tries keep the raw file, with counts 1 and 2. The third deletes it and its count and reports it lost.
- `TestAnOldOutputThatCantBeKeptGoesAtOnce`: a raw file 8 days old goes at the first failed try.
- `TestALostCrashLogIsTold`: the notice is sticky, has no button and says the log couldn't be written.

## Checks

- `go build ./...`, `go vet ./...` and `go vet -tags bench ./cmd/desktop` pass.
- `go test ./...` passes, with one note. In the full run, `internal/project` `TestCrashExport` ("no kill landed in an export") and `internal/scenario` `TestRunTimeout` ("0 calls") failed under load. Both are timing tests in packages this branch doesn't touch. Both pass alone, and both packages pass in full when run again.
- The two new update tests passed 30 runs in a row.
- No live check was done: the bench can't start a download without a real update, and the crash cases are covered by the tests.

## For Main: SPEC lines that may want a change

- 2.8 (updates): Restart now during a download stops the download and restarts into the installed version. Turning updates on again checks at once, even during a download.
- 2.1 (crash log): output whose log can't be written is deleted after 3 failed starts or 7 days, and the user is told the log couldn't be kept.
