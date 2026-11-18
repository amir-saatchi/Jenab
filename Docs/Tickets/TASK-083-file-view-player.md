# TASK-083 — Play audio and video in the file view
**Type:** Task
**Status:** Open
**Gate:** 3
**Needs:** TASK-056, TASK-082

## Goal
SPEC 5.12 says a file opened in the file view plays when it is audio or video. Today it doesn't: the file view offers *Open with the system app* for both (`internal/native/files/view.go`), as for the types it doesn't show. TASK-082 found that MyGo plays both. Its `audio` package decodes WAV, MP3, Vorbis, FLAC and AAC/ALAC; `ui/video` plays video with Media Foundation on Windows, AVFoundation on macOS and GStreamer on Linux. What's missing is the player around them: neither has play, pause or a seek bar. Build that in the kit and use it in the file view.

## Scope
1. **A player in the kit** (`internal/native/kit`), for audio and video alike:
   - play and pause, the time played and the length, and a seek bar you can drag and step with the arrow keys;
   - volume and mute;
   - for video, the picture at the file view's width, keeping its aspect ratio, with the controls below it.
   
   It follows the kit's rules: tokens and themes (light and dark), keyboard use (Space plays and pauses), focus rings, assistive-technology labels, and right to left. The controls keep their left-to-right order in an RTL layout, as media players do; say in the report what you did.
2. **The file view** plays an audio or video object with it, in the place of today's *Open with the system app* card. *Open with the system app* stays as an action next to it. A type MyGo can't decode, or a file that fails, shows the error and keeps that action.
3. **Reading the bytes.** Today the file view reads at most the first 1 MB of a text file through `NativeBucket.Read`. A player needs the whole object and seeks in it. Read it through the services (`BucketService`), which lease the project, never through a `file://` path (SPEC 4.7, 5.12). Stream it if you can rather than holding a large video in memory; say what you chose and what it costs for a 1 GB file.
4. **Leaving.** Playback stops when the file view closes, the project or chat changes, or the app goes to the tray. It doesn't play on in the background. Two file views never play at once.
5. **Platforms.** Windows is the reference. Say what macOS and Linux need (GStreamer on Linux) for TASK-060, without building there.

## Rules
- No downloads without asking. If you need test media, generate it in the test (a tone, a few frames) rather than committing files; keep anything larger out of the repo.
- No system dialogs in live checks; use a scratch profile.
- Don't edit Docs/. Put SPEC wording (5.12, 4.7) in the report.

## Done when
- Tests in the kit: play and pause, seeking by drag and by keys, and the time shown, on generated audio; a video test if `ui/video` can run headless, else say why not.
- A test in `native/files`: an audio object opens in the player, and an undecodable one shows the error with *Open with the system app*.
- A live check on the bench build plays a generated WAV and a short video in the file view.
- `go build ./...`, `go vet ./...`, `go vet -tags bench ./cmd/desktop` and `go test ./...` pass.
