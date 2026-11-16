package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// The run measures cmd/desktop built with the bench tag (SPIKE-048 Part
// B). The app has no measuring code: MyGo logs each frame with
// MYGO_FRAME_STATS=all, the bench build stamps the log with microseconds
// and adds marks (open, opened, key), and a loopback port drives it.

type nativeApp struct {
	cmd    *exec.Cmd
	exited chan struct{}
	logf   *os.File
	path   string
	port   int
	start  time.Time
}

func startNative(exe string, h home, logPath string) (*nativeApp, error) {
	f, err := os.Create(logPath)
	if err != nil {
		return nil, err
	}
	port, err := freePort()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(), h.env()...)
	cmd.Env = append(cmd.Env, "MYGO_FRAME_STATS=all", fmt.Sprintf("JENAB_BENCH_PORT=%d", port))
	if h.gemini != "" {
		cmd.Env = append(cmd.Env, "JENAB_BENCH_KEY_GEMINI="+os.Getenv("GEMINI_API_KEY"))
	}
	cmd.Stdout, cmd.Stderr = f, f
	if err := cmd.Start(); err != nil {
		f.Close()
		return nil, err
	}
	a := &nativeApp{cmd: cmd, exited: make(chan struct{}), logf: f, path: logPath, port: port}
	go func() { cmd.Wait(); close(a.exited) }()
	if a.start, err = started(cmd.Process.Pid); err != nil {
		a.kill()
		return nil, err
	}
	return a, nil
}

func (a *nativeApp) kill() {
	a.cmd.Process.Kill()
	<-a.exited
	a.logf.Close()
}

// call asks the app's bench port; it waits for the port to open.
func (a *nativeApp) call(ctx context.Context, method, path string, q url.Values, out any) error {
	u := fmt.Sprintf("http://127.0.0.1:%d%s", a.port, path)
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	for {
		req, _ := http.NewRequestWithContext(ctx, method, u, nil)
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode >= 300 {
				var b strings.Builder
				bufio.NewReader(resp.Body).WriteTo(&b)
				return fmt.Errorf("%s %s: %s %s", method, path, resp.Status, strings.TrimSpace(b.String()))
			}
			if out != nil {
				return json.NewDecoder(resp.Body).Decode(out)
			}
			return nil
		}
		if ctx.Err() != nil {
			return err
		}
		select {
		case <-a.exited:
			return errors.New("the app ended")
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func (a *nativeApp) state(ctx context.Context) (benchState, error) {
	var s benchState
	return s, a.call(ctx, "GET", "/state", nil, &s)
}

type benchState struct {
	Loaded    bool     `json:"loaded"`
	Chats     []string `json:"chats"`
	Open      string   `json:"open"`
	Messages  int      `json:"messages"`
	Running   bool     `json:"running"`
	Streaming bool     `json:"streaming"`
	Working   int      `json:"working"`
}

// until polls the app's state until cond holds.
func (a *nativeApp) until(ctx context.Context, what string, wait time.Duration, cond func(benchState) bool) error {
	ctx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	for {
		s, err := a.state(ctx)
		if err == nil && cond(s) {
			return nil
		}
		if ctx.Err() != nil {
			return fmt.Errorf("timed out waiting for %s (last state %+v, %v)", what, s, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// open opens a chat by title; the first call opens the project.
func (a *nativeApp) open(ctx context.Context, title string) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	for {
		err := a.call(ctx, "POST", "/open", url.Values{"title": {title}}, nil)
		if err == nil || ctx.Err() != nil {
			return err
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func (a *nativeApp) send(ctx context.Context, text string) error {
	return a.call(ctx, "POST", "/send", url.Values{"text": {text}}, nil)
}

func (a *nativeApp) quit() error {
	select {
	case <-a.exited:
		return nil
	default:
	}
	defer a.logf.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.call(ctx, "POST", "/quit", nil, nil); err != nil {
		a.kill()
		return err
	}
	select {
	case <-a.exited:
		return nil
	case <-time.After(30 * time.Second):
		a.kill()
		return errors.New("the app didn't quit within 30 s, so it was killed")
	}
}

func (a *nativeApp) memorySamples(n int, every time.Duration) (memory, error) {
	var ms []memory
	for i := range n {
		if i > 0 {
			time.Sleep(every)
		}
		m, err := treeMemory(a.cmd.Process.Pid)
		if err != nil {
			return m, err
		}
		ms = append(ms, m)
	}
	slices.SortFunc(ms, func(x, y memory) int { return int(x.PrivateMB - y.PrivateMB) })
	return ms[len(ms)/2], nil
}

// The app's log.

type logLine struct {
	at   time.Time
	text string
}

type frame struct {
	end  time.Time
	took float64 // ms of work: build, layout, paint, present
}

func (f frame) start() time.Time {
	return f.end.Add(-time.Duration(f.took * float64(time.Millisecond)))
}

var frameRe = regexp.MustCompile(`^mygo: frame \d+ took ([0-9.]+) ms`)

// readLog reads the log's stamped lines: frames and the bench's marks.
func readLog(path string) (frames []frame, marks []logLine, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if len(line) < 27 {
			continue
		}
		at, err := time.ParseInLocation("2006/01/02 15:04:05.000000", line[:26], time.Local)
		if err != nil {
			continue
		}
		text := line[27:]
		if m := frameRe.FindStringSubmatch(text); m != nil {
			took, _ := strconv.ParseFloat(m[1], 64)
			frames = append(frames, frame{end: at, took: took})
		} else if strings.HasPrefix(text, "bench: ") {
			marks = append(marks, logLine{at, strings.TrimPrefix(text, "bench: ")})
		}
	}
	return frames, marks, sc.Err()
}

// waitMark waits until the log has a mark starting with prefix after
// from, and returns it.
func waitMark(path, prefix string, from time.Time, wait time.Duration) (logLine, []frame, error) {
	deadline := time.Now().Add(wait)
	for {
		frames, marks, err := readLog(path)
		if err != nil {
			return logLine{}, nil, err
		}
		for _, m := range marks {
			if !m.at.Before(from) && strings.HasPrefix(m.text, prefix) {
				return m, frames, nil
			}
		}
		if time.Now().After(deadline) {
			return logLine{}, nil, fmt.Errorf("no %q mark in the log", prefix)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// firstFrameAfter is the first frame that began at or after t.
func firstFrameAfter(frames []frame, t time.Time) (frame, bool) {
	for _, f := range frames {
		if !f.start().Before(t) {
			return f, true
		}
	}
	return frame{}, false
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

func pct(xs []float64, p float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := slices.Clone(xs)
	slices.Sort(s)
	i := int(p/100*float64(len(s))+0.999999) - 1
	return s[min(len(s)-1, max(0, i))]
}

func round(x float64) float64 { return float64(int(x*10+0.5)) / 10 }

// The native run.

type nativeStart struct {
	FirstFrameMs int64 `json:"first_frame_ms"` // any frame, as the window first shows
	ShellMs      int64 `json:"shell_ms"`       // the first frame after the projects are read
}

type nativeOpen struct {
	ReadMs      float64 `json:"read_ms"`       // from the click to the thread read
	FirstRowsMs float64 `json:"first_rows_ms"` // to the end of the first frame after it
	AllRowsMs   float64 `json:"all_rows_ms"`   // to the end of the last frame before 300 ms without one
	Frames      int     `json:"frames"`
	LongestMs   float64 `json:"longest_frame_ms"`
}

type nativeStream struct {
	Frames        int     `json:"frames"`
	Seconds       float64 `json:"seconds"`
	FPS           float64 `json:"fps"`
	GapP95Ms      float64 `json:"gap_p95_ms"` // between frame ends, as rAF deltas
	GapMaxMs      float64 `json:"gap_max_ms"`
	WorkP95Ms     float64 `json:"work_p95_ms"` // a frame's own work
	WorkMaxMs     float64 `json:"work_max_ms"`
	Over33MsPct   float64 `json:"work_over_33_ms_pct"`
	Keys          int     `json:"keys"`
	KeyPaintP50Ms float64 `json:"key_paint_p50_ms"`
	KeyPaintP95Ms float64 `json:"key_paint_p95_ms"`
	KeyPaintMaxMs float64 `json:"key_paint_max_ms"`
	// Upper is the same to the end of the second frame after the key, in
	// case the first began before the key was read.
	KeyUpperP95Ms float64 `json:"key_upper_p95_ms"`
}

type nativeReport struct {
	Started     time.Time     `json:"started"`
	Took        float64       `json:"took_s"`
	App         string        `json:"app"`
	Rate        float64       `json:"rate"`
	Reply       int           `json:"reply_tokens"`
	History     string        `json:"history"`
	Starts      []nativeStart `json:"starts"`
	Idle        memory        `json:"idle"`
	Opens       []nativeOpen  `json:"opens"`
	AfterOpens  memory        `json:"after_opens"`
	Stream      nativeStream  `json:"stream"`
	AfterStream memory        `json:"after_stream"`
	Load        loadResult    `json:"load"`
	Left        memory        `json:"left"`
	Cycles      []float64     `json:"cycles_mb"`
}

func runNative(o options, exe string, h home, model *fakeModel) (err error) {
	ctx := context.Background()
	rep := &nativeReport{Started: time.Now().UTC(), App: exe, Rate: o.rate, Reply: len(model.reply), History: o.history}
	log := func(f string, a ...any) { fmt.Printf(f+"\n", a...) }

	for i := range o.starts {
		a, err := startNative(exe, h, filepath.Join(o.out, fmt.Sprintf("native-start-%d.log", i+1)))
		if err != nil {
			return err
		}
		m, frames, err := waitMark(a.path, "projects loaded", a.start, 30*time.Second)
		var s nativeStart
		if err == nil {
			deadline := time.Now().Add(5 * time.Second)
			for {
				if f, ok := firstFrameAfter(frames, m.at); ok {
					s.ShellMs = f.end.Sub(a.start).Milliseconds()
					break
				}
				if time.Now().After(deadline) {
					err = errors.New("no frame after the projects were read")
					break
				}
				time.Sleep(20 * time.Millisecond)
				frames, _, _ = readLog(a.path)
			}
			if len(frames) > 0 {
				s.FirstFrameMs = frames[0].end.Sub(a.start).Milliseconds()
			}
		}
		if qerr := a.quit(); err == nil {
			err = qerr
		}
		if err != nil {
			return fmt.Errorf("start %d: %w", i+1, err)
		}
		rep.Starts = append(rep.Starts, s)
		log("start %d: first frame %d ms, the shell %d ms after the process started", i+1, s.FirstFrameMs, s.ShellMs)
	}

	a, err := startNative(exe, h, filepath.Join(o.out, "native.log"))
	if err != nil {
		return err
	}
	defer a.quit()
	if err := a.until(ctx, "the projects", 30*time.Second, func(s benchState) bool { return s.Loaded }); err != nil {
		return err
	}
	a.call(ctx, "POST", "/front", nil, nil)
	// The first open opens the project at its Mother chat.
	a.call(ctx, "POST", "/open", url.Values{"title": {""}}, nil)
	if err := a.until(ctx, "the Mother chat", 20*time.Second, func(s benchState) bool { return s.Open != "" }); err != nil {
		return err
	}

	time.Sleep(10 * time.Second)
	if rep.Idle, err = a.memorySamples(5, time.Second); err != nil {
		return err
	}
	log("idle: %.0f MB private", rep.Idle.PrivateMB)

	for i := range o.opens {
		if err := a.open(ctx, busyChat); err != nil {
			return err
		}
		time.Sleep(500 * time.Millisecond)
		t0 := time.Now()
		if err := a.open(ctx, historyChat); err != nil {
			return fmt.Errorf("open %d: %w", i+1, err)
		}
		time.Sleep(time.Second)
		r, err := measureOpen(a.path, t0)
		if err != nil {
			return fmt.Errorf("open %d: %w", i+1, err)
		}
		rep.Opens = append(rep.Opens, r)
		log("open %d: read %.0f ms, first rows %.0f ms, all %.0f ms, longest frame %.0f ms", i+1, r.ReadMs, r.FirstRowsMs, r.AllRowsMs, r.LongestMs)
		time.Sleep(500 * time.Millisecond)
	}
	if rep.AfterOpens, err = a.memorySamples(3, time.Second); err != nil {
		return err
	}
	log("the chat open: %.0f MB private", rep.AfterOpens.PrivateMB)

	if rep.Stream, err = nativeStreamRun(ctx, a); err != nil {
		return fmt.Errorf("stream: %w", err)
	}
	s := rep.Stream
	log("stream: %.1f fps, gaps p95 %.1f ms, work p95 %.1f max %.1f ms; key → paint p50 %.1f ms, p95 %.1f, max %.1f ms", s.FPS, s.GapP95Ms, s.WorkP95Ms, s.WorkMaxMs, s.KeyPaintP50Ms, s.KeyPaintP95Ms, s.KeyPaintMaxMs)
	if rep.AfterStream, err = a.memorySamples(3, time.Second); err != nil {
		return err
	}

	// Ten answers: two busy chats and eight more, as the default mode.
	chats := []string{busyChat}
	for i := range otherChats {
		chats = append(chats, fmt.Sprint("Call ", i+1))
	}
	chats = append(chats, historyChat)
	model.peak.Store(0)
	for _, c := range chats {
		if err := a.open(ctx, c); err != nil {
			return err
		}
		if err := a.send(ctx, "Write a plan for the price pipeline."); err != nil {
			return fmt.Errorf("%s: %w", c, err)
		}
	}
	deadline := time.Now().Add(3 * time.Minute)
	for {
		m, err := treeMemory(a.cmd.Process.Pid)
		if err != nil {
			return err
		}
		rep.Load.Samples++
		if m.PrivateMB > rep.Load.Peak.PrivateMB {
			rep.Load.Peak = m
		}
		st, _ := a.state(ctx)
		if model.streams.Load() == 0 && st.Working == 0 {
			break
		}
		if time.Now().After(deadline) {
			return errors.New("the answers didn't end within 3 minutes")
		}
		time.Sleep(500 * time.Millisecond)
	}
	rep.Load.Streams = int(model.peak.Load())
	if rep.Load.After, err = a.memorySamples(3, time.Second); err != nil {
		return err
	}
	log("load: %d answers at once, peak %.0f MB, after %.0f MB private", rep.Load.Streams, rep.Load.Peak.PrivateMB, rep.Load.After.PrivateMB)

	if err := a.open(ctx, busyChat); err != nil {
		return err
	}
	time.Sleep(10 * time.Second)
	if rep.Left, err = a.memorySamples(3, time.Second); err != nil {
		return err
	}
	for range 10 {
		if err := a.open(ctx, historyChat); err != nil {
			return err
		}
		time.Sleep(300 * time.Millisecond)
		if err := a.open(ctx, busyChat); err != nil {
			return err
		}
		time.Sleep(time.Second)
		m, err := treeMemory(a.cmd.Process.Pid)
		if err != nil {
			return err
		}
		rep.Cycles = append(rep.Cycles, m.PrivateMB)
	}
	log("settle: %.0f MB after leaving; cycles %v", rep.Left.PrivateMB, rep.Cycles)
	if err := a.quit(); err != nil {
		return err
	}
	rep.Took = time.Since(rep.Started).Seconds()
	name := rep.Started.Local().Format("20060102-150405")
	b, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	p := filepath.Join(o.out, "native-"+name+".json")
	if err := os.WriteFile(p, b, 0o644); err != nil {
		return err
	}
	fmt.Println("report:", p)
	return nil
}

// measureOpen reads one open of the history chat from the log: the
// click's mark after t0, the read's, and the frames after them.
func measureOpen(path string, t0 time.Time) (nativeOpen, error) {
	var r nativeOpen
	// The app's clock and this one may differ by a millisecond or so; the
	// open before this one was half a second earlier.
	click, _, err := waitMark(path, "open "+historyChat, t0.Add(-100*time.Millisecond), 5*time.Second)
	if err != nil {
		return r, err
	}
	read, frames, err := waitMark(path, "opened "+historyChat, click.at, 5*time.Second)
	if err != nil {
		return r, err
	}
	r.ReadMs = round(ms(read.at.Sub(click.at)))
	first, ok := firstFrameAfter(frames, read.at)
	if !ok {
		return r, errors.New("no frame after the chat was read")
	}
	r.FirstRowsMs = round(ms(first.end.Sub(click.at)))
	last := first
	for _, f := range frames {
		if f.end.Before(click.at) {
			continue
		}
		if f.start().Sub(last.end) > 300*time.Millisecond {
			break
		}
		if !f.end.Before(first.end) {
			last = f
		}
		r.Frames++
		r.LongestMs = max(r.LongestMs, f.took)
	}
	r.AllRowsMs = round(ms(last.end.Sub(click.at)))
	return r, nil
}

// nativeStreamRun sends a message in the open chat and, while the answer
// streams, presses a key every 200 ms, as the default mode does.
func nativeStreamRun(ctx context.Context, a *nativeApp) (nativeStream, error) {
	var r nativeStream
	if err := a.send(ctx, "Write a plan for the price pipeline."); err != nil {
		return r, err
	}
	if err := a.until(ctx, "the answer to stream", 30*time.Second, func(s benchState) bool { return s.Streaming }); err != nil {
		return r, err
	}
	t0 := time.Now()
	for range 60 {
		time.Sleep(200 * time.Millisecond)
		if err := a.call(ctx, "POST", "/key", url.Values{"ch": {"a"}}, nil); err != nil {
			return r, err
		}
	}
	time.Sleep(100 * time.Millisecond)
	t1 := time.Now()
	frames, marks, err := readLog(a.path)
	if err != nil {
		return r, err
	}
	var gaps, work []float64
	var prev time.Time
	for _, f := range frames {
		if f.end.Before(t0) || f.end.After(t1) {
			continue
		}
		r.Frames++
		work = append(work, f.took)
		if !prev.IsZero() {
			gaps = append(gaps, ms(f.end.Sub(prev)))
		}
		prev = f.end
	}
	r.Seconds = round(t1.Sub(t0).Seconds())
	r.FPS = round(float64(r.Frames) / t1.Sub(t0).Seconds())
	r.GapP95Ms, r.WorkP95Ms = round(pct(gaps, 95)), round(pct(work, 95))
	r.GapMaxMs, r.WorkMaxMs = round(slices.Max(append(gaps, 0))), round(slices.Max(append(work, 0)))
	over := 0
	for _, w := range work {
		if w > 33.4 {
			over++
		}
	}
	r.Over33MsPct = round(100 * float64(over) / float64(max(1, len(work))))
	var keys, upper []float64
	for _, m := range marks {
		if m.at.Before(t0) || !strings.HasPrefix(m.text, "key ") {
			continue
		}
		for i, f := range frames {
			if !f.start().Before(m.at) {
				keys = append(keys, ms(f.end.Sub(m.at)))
				if i+1 < len(frames) {
					upper = append(upper, ms(frames[i+1].end.Sub(m.at)))
				}
				break
			}
		}
	}
	r.Keys = len(keys)
	r.KeyPaintP50Ms, r.KeyPaintP95Ms, r.KeyPaintMaxMs = round(pct(keys, 50)), round(pct(keys, 95)), round(slices.Max(append(keys, 0)))
	r.KeyUpperP95Ms = round(pct(upper, 95))
	// Clear the composer and let the answer end.
	if err := a.call(ctx, "POST", "/key", url.Values{"vk": {"8"}, "n": {"70"}}, nil); err != nil {
		return r, err
	}
	return r, a.until(ctx, "the answer to finish", 3*time.Minute, func(s benchState) bool { return !s.Running })
}

// holdNative starts the app with chat open and keeps it up for wait, with
// the fake model behind it, so a person or a UI Automation script can look
// at it; it prints the process and the bench port. The app quits early if
// it is closed from its tray or the port's /quit.
func holdNative(o options, exe string, h home, chat string, wait time.Duration) error {
	ctx := context.Background()
	a, err := startNative(exe, h, filepath.Join(o.out, "native-hold.log"))
	if err != nil {
		return err
	}
	defer a.kill()
	if err := a.until(ctx, "the projects", 30*time.Second, func(s benchState) bool { return s.Loaded }); err != nil {
		return err
	}
	a.call(ctx, "POST", "/open", url.Values{"title": {""}}, nil)
	if err := a.until(ctx, "the Mother chat", 20*time.Second, func(s benchState) bool { return s.Open != "" }); err != nil {
		return err
	}
	if err := a.open(ctx, chat); err != nil {
		return err
	}
	fmt.Printf("holding: pid %d, port %d, home %s, %s open, for %s\n", a.cmd.Process.Pid, a.port, h.dir, chat, wait)
	select {
	case <-a.exited:
		fmt.Println("the app quit")
	case <-time.After(wait):
	}
	return nil
}
