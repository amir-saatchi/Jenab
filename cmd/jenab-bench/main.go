// Command jenab-bench measures the built app (P1-18): start-up, idle
// memory, opening a chat of 200 messages, streaming at 100 tokens a
// second while the user types, and memory with ten answers streaming at
// once. It starts the app with a new user folder and a local fake model,
// drives it through WebView2's DevTools port, and writes a report. The
// app has no measuring code: a build with the bench tag only opens the
// port. Windows only for now.
//
// Keep the app's window visible and leave the machine alone during a run:
// a hidden window gets fewer frames.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const mb = 1024 * 1024

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "jenab-bench:", err)
		os.Exit(1)
	}
}

type options struct {
	app    string
	out    string
	starts int
	rate   float64
	opens  int
}

func run() (err error) {
	var o options
	flag.StringVar(&o.app, "app", filepath.Join("bin", "jenab-bench-app.exe"), "the app, built with -tags production,bench (task bench)")
	flag.StringVar(&o.out, "out", filepath.Join("bin", "bench"), "where the report goes")
	flag.IntVar(&o.starts, "starts", 3, "app starts for the start-up time")
	flag.Float64Var(&o.rate, "rate", 100, "tokens a second the fake model streams")
	flag.IntVar(&o.opens, "opens", 5, "times the 200-message chat is opened")
	keep := flag.Bool("keep", false, "keep the app's user folder")
	temp := flag.String("temp", "", "where the app's user folder goes; default: the system's temp folder")
	flag.Parse()
	if _, err := os.Stat(o.app); err != nil {
		return fmt.Errorf("the app: %w (task bench builds it)", err)
	}
	ctx := context.Background()
	dir, err := os.MkdirTemp(*temp, "jenab-bench-")
	if err != nil {
		return err
	}
	if dir, err = filepath.Abs(dir); err != nil {
		return err
	}
	if !*keep {
		defer os.RemoveAll(dir)
	}
	h := home{dir: dir}
	model, err := startModel(reply(1, 2000), o.rate)
	if err != nil {
		return err
	}
	defer model.close()
	if err := h.writeSettings(model.url); err != nil {
		return err
	}
	if err := h.seed(ctx); err != nil {
		return fmt.Errorf("seed: %w", err)
	}
	if err := os.MkdirAll(o.out, 0o755); err != nil {
		return err
	}
	rep := &report{Started: time.Now().UTC(), App: o.app, Rate: o.rate, Reply: len(model.reply)}
	log := func(f string, a ...any) { fmt.Printf(f+"\n", a...) }

	// Start-up: each start ends with a normal quit.
	for i := range o.starts {
		a, err := launch(ctx, o.app, h, filepath.Join(o.out, fmt.Sprintf("app-start-%d.log", i+1)))
		if err != nil {
			return err
		}
		s, err := a.startup(ctx)
		if qerr := a.quit(); err == nil {
			err = qerr
		}
		if err != nil {
			return fmt.Errorf("start %d: %w", i+1, err)
		}
		rep.Starts = append(rep.Starts, s)
		log("start %d: first paint %d ms after the process started", i+1, s.FirstPaintMs)
	}

	a, err := launch(ctx, o.app, h, filepath.Join(o.out, "app.log"))
	if err != nil {
		return err
	}
	defer a.quit()
	defer func() {
		if err != nil {
			a.dump(o.out)
		}
	}()
	if _, err := a.startup(ctx); err != nil {
		return err
	}
	if err := a.page.eval(ctx, setupJS, nil); err != nil {
		return err
	}

	// N-52: idle, with the project's Mother chat open.
	time.Sleep(10 * time.Second)
	rep.Idle, err = a.memorySamples(5, time.Second)
	if err != nil {
		return err
	}
	log("idle: %.0f MB private", rep.Idle.PrivateMB)

	// N-02: open the 200-message chat, from an empty one each time.
	for i := range o.opens {
		if err := a.page.eval(ctx, switchJS(busyChat), nil); err != nil {
			return err
		}
		time.Sleep(500 * time.Millisecond)
		var r openResult
		if err := a.page.eval(ctx, openJS(historyChat), &r); err != nil {
			return fmt.Errorf("open %d: %w", i+1, err)
		}
		rep.Opens = append(rep.Opens, r)
		log("open %d: %d rows, first rows %.0f ms, all %.0f ms, longest task %.0f ms, LoAF %.0f ms", i+1, r.Rows, r.FirstRowsMs, r.AllRowsMs, r.LongTaskMaxMs, r.LoafMaxMs)
		time.Sleep(500 * time.Millisecond)
	}
	if rep.AfterOpens, err = a.memorySamples(3, time.Second); err != nil {
		return err
	}

	// N-01: stream an answer into the open chat and type meanwhile.
	if rep.Stream, err = a.stream(ctx); err != nil {
		return fmt.Errorf("stream: %w", err)
	}
	log("stream: %.1f fps, key → paint p50 %.0f ms, max %.0f ms", rep.Stream.FPS, rep.Stream.KeyPaintP50Ms, rep.Stream.KeyPaintMaxMs)
	if rep.AfterStream, err = a.memorySamples(3, time.Second); err != nil {
		return err
	}

	// N-54: two busy chats and eight more answers, all streaming.
	if rep.Load, err = a.load(ctx, model); err != nil {
		return fmt.Errorf("load: %w", err)
	}
	log("load: %d answers at once, peak %.0f MB private", rep.Load.Streams, rep.Load.Peak.PrivateMB)

	// Does memory come back after use? Leave for an empty chat, then
	// collect garbage, then tell WebView2 memory is short, which drops
	// its caches. What stays after that is held.
	if rep.Settle, err = a.settle(ctx); err != nil {
		return fmt.Errorf("settle: %w", err)
	}
	log("settle: %.0f MB after leaving, %.0f MB after memory pressure", rep.Settle.Left.PrivateMB, rep.Settle.Pressure.PrivateMB)

	if err := a.quit(); err != nil {
		log("quit: %v", err)
	}
	rep.Took = time.Since(rep.Started).Seconds()
	md, err := rep.write(o.out)
	if err != nil {
		return err
	}
	log("report: %s", md)
	return nil
}

// app is a running copy of the built app.
type app struct {
	cmd    *exec.Cmd
	page   *cdp
	exited chan struct{}
	log    *os.File
}

// start starts the app with its own user folder. port > 0 asks for a
// DevTools port; only a build with the bench tag opens it
// (cmd/desktop/bench.go).
func start(exe string, h home, logPath string, port int) (*app, error) {
	f, err := os.Create(logPath)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(), h.env()...)
	if port > 0 {
		cmd.Env = append(cmd.Env, fmt.Sprintf("JENAB_DEVTOOLS_PORT=%d", port))
	}
	cmd.Stdout, cmd.Stderr = f, f
	if err := cmd.Start(); err != nil {
		f.Close()
		return nil, err
	}
	a := &app{cmd: cmd, exited: make(chan struct{}), log: f}
	go func() { cmd.Wait(); close(a.exited) }()
	return a, nil
}

// launch starts the app with a DevTools port and connects to its page.
// WebView2 sometimes opens no port, always on the first start with a new
// profile; then the app is quit and started again, up to three times.
func launch(ctx context.Context, exe string, h home, logPath string) (*app, error) {
	var err error
	for try := 1; try <= 3; try++ {
		var a *app
		if a, err = launchOnce(ctx, exe, h, logPath); err == nil {
			return a, nil
		}
		fmt.Printf("%s: no DevTools page in 15 s, so starting again; normal on the first start with a new profile\n", filepath.Base(logPath))
	}
	return nil, err
}

func launchOnce(ctx context.Context, exe string, h home, logPath string) (*app, error) {
	port, err := freePort()
	if err != nil {
		return nil, err
	}
	a, err := start(exe, h, logPath, port)
	if err != nil {
		return nil, err
	}
	dctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if a.page, err = dialPage(dctx, port); err != nil {
		if qerr := a.quit(); qerr != nil {
			err = errors.Join(err, qerr)
		}
		return nil, err
	}
	return a, nil
}

func freePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

// quit closes the window and waits for the app to end; after 30 s it is
// killed. A second quit does nothing.
func (a *app) quit() error {
	select {
	case <-a.exited:
		return nil
	default:
	}
	if a.page != nil {
		a.page.close()
	}
	defer a.log.Close()
	wait := treeWaiter(a.cmd.Process.Pid)
	if err := closeApp(a.cmd.Process.Pid); err != nil {
		a.kill()
		return err
	}
	select {
	case <-a.exited:
		err := wait(30 * time.Second)
		time.Sleep(2 * time.Second) // let WebView2 finish with its profile
		return err
	case <-time.After(30 * time.Second):
		a.kill()
		return errors.New("the app didn't quit within 30 s, so it was killed")
	}
}

// dump saves a screenshot and the page's text, for a step that failed.
func (a *app) dump(dir string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var shot struct {
		Data []byte `json:"data"`
	}
	if a.page.call(ctx, "Page.captureScreenshot", map[string]any{"format": "png"}, &shot) == nil {
		os.WriteFile(filepath.Join(dir, "failed.png"), shot.Data, 0o644)
	}
	var text string
	if a.page.eval(ctx, "document.body.innerText", &text) == nil {
		os.WriteFile(filepath.Join(dir, "failed.txt"), []byte(text), 0o644)
	}
}

func (a *app) kill() {
	a.cmd.Process.Kill()
	<-a.exited
}

// startup measures from the process start to the shell's first paint.
func (a *app) startup(ctx context.Context) (startResult, error) {
	var r struct {
		Painted, Navigation int64
		Already             bool
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	// The first page is blank; the probe starts again in the app's page.
	for {
		err := a.page.eval(ctx, startupJS, &r)
		if err == nil {
			break
		}
		if ctx.Err() != nil || !strings.Contains(err.Error(), "context") {
			return startResult{}, err
		}
		time.Sleep(5 * time.Millisecond)
	}
	t, err := started(a.cmd.Process.Pid)
	if err != nil {
		return startResult{}, err
	}
	return startResult{FirstPaintMs: r.Painted - t.UnixMilli(), NavigationMs: r.Navigation - t.UnixMilli(), Late: r.Already}, nil
}

func (a *app) memory() (memory, error) {
	m, err := treeMemory(a.cmd.Process.Pid)
	if err != nil {
		return m, err
	}
	var heap float64
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	a.page.eval(ctx, heapJS, &heap)
	m.JSHeapMB = heap
	return m, nil
}

// memorySamples is the median of n samples by private bytes.
func (a *app) memorySamples(n int, every time.Duration) (memory, error) {
	var ms []memory
	for i := range n {
		if i > 0 {
			time.Sleep(every)
		}
		m, err := a.memory()
		if err != nil {
			return m, err
		}
		ms = append(ms, m)
	}
	slices.SortFunc(ms, func(x, y memory) int { return int(x.PrivateMB - y.PrivateMB) })
	return ms[len(ms)/2], nil
}

// send types a message into the open chat and presses Enter.
func (a *app) send(ctx context.Context, text string) error {
	if err := a.page.eval(ctx, `(window.__bench.composer().focus(), true)`, nil); err != nil {
		return err
	}
	if err := a.page.insertText(ctx, text); err != nil {
		return err
	}
	if err := a.page.key(ctx, "Enter", "Enter", 13, "\r"); err != nil {
		return err
	}
	return a.page.eval(ctx, sentJS, nil)
}

// stream sends a message in the 200-message chat and, while the answer
// streams, types a key every 200 ms into the composer.
func (a *app) stream(ctx context.Context) (streamResult, error) {
	var r streamResult
	if err := a.send(ctx, "Write a plan for the price pipeline."); err != nil {
		return r, err
	}
	if err := a.page.eval(ctx, streamingJS, nil); err != nil {
		return r, err
	}
	if err := a.page.eval(ctx, startMeasureJS, nil); err != nil {
		return r, err
	}
	for range 60 { // 12 s of a 20 s answer
		time.Sleep(200 * time.Millisecond)
		if err := a.page.key(ctx, "a", "KeyA", 65, "a"); err != nil {
			return r, err
		}
	}
	if err := a.page.eval(ctx, stopMeasureJS, &r); err != nil {
		return r, err
	}
	if err := a.page.eval(ctx, clearJS, nil); err != nil {
		return r, err
	}
	return r, a.page.eval(ctx, finishedJS, nil)
}

// load sends a message in ten chats, then samples memory every 500 ms
// until every answer has ended. The 200-message chat stays open, so one
// answer streams on screen.
func (a *app) load(ctx context.Context, model *fakeModel) (loadResult, error) {
	var r loadResult
	model.peak.Store(0)
	chats := []string{busyChat}
	for i := range otherChats {
		chats = append(chats, fmt.Sprint("Call ", i+1))
	}
	chats = append(chats, historyChat)
	for _, c := range chats {
		if err := a.page.eval(ctx, switchJS(c), nil); err != nil {
			return r, err
		}
		if err := a.send(ctx, "Write a plan for the price pipeline."); err != nil {
			return r, fmt.Errorf("%s: %w", c, err)
		}
	}
	deadline := time.Now().Add(2 * time.Minute)
	for {
		m, err := a.memory()
		if err != nil {
			return r, err
		}
		r.Samples++
		if m.PrivateMB > r.Peak.PrivateMB {
			r.Peak = m
		}
		if model.streams.Load() == 0 {
			break
		}
		if time.Now().After(deadline) {
			return r, errors.New("the answers didn't end within 2 minutes")
		}
		time.Sleep(500 * time.Millisecond)
	}
	r.Streams = int(model.peak.Load())
	var err error
	r.After, err = a.memorySamples(3, time.Second)
	return r, err
}

func (a *app) settle(ctx context.Context) (settleResult, error) {
	var r settleResult
	var err error
	if err = a.page.eval(ctx, switchJS(busyChat), nil); err != nil {
		return r, err
	}
	time.Sleep(10 * time.Second)
	if r.Left, err = a.memorySamples(3, time.Second); err != nil {
		return r, err
	}
	if err = a.page.call(ctx, "HeapProfiler.collectGarbage", map[string]any{}, nil); err != nil {
		return r, err
	}
	time.Sleep(3 * time.Second)
	if r.Collected, err = a.memorySamples(3, time.Second); err != nil {
		return r, err
	}
	if err = a.page.call(ctx, "Memory.simulatePressureNotification", map[string]any{"level": "critical"}, nil); err != nil {
		return r, err
	}
	time.Sleep(5 * time.Second)
	r.Pressure, err = a.memorySamples(3, time.Second)
	return r, err
}

// Results.

type settleResult struct {
	Left      memory `json:"left"`      // 10 s after switching to an empty chat
	Collected memory `json:"collected"` // after a garbage collection
	Pressure  memory `json:"pressure"`  // after a critical memory-pressure signal
}

type memory struct {
	PrivateMB        float64 `json:"private_mb"`
	WorkingSetMB     float64 `json:"working_set_mb"`
	AppPrivateMB     float64 `json:"app_private_mb"`
	WebviewPrivateMB float64 `json:"webview_private_mb"`
	JSHeapMB         float64 `json:"js_heap_mb"`
	Processes        int     `json:"processes"`
	// ByTypeMB splits WebView2's private bytes by process type.
	ByTypeMB map[string]float64 `json:"by_type_mb"`
}

type startResult struct {
	FirstPaintMs int64 `json:"first_paint_ms"`
	NavigationMs int64 `json:"navigation_ms"`
	Late         bool  `json:"late"` // the shell was up before the probe ran
}

type openResult struct {
	FirstRowsMs       float64 `json:"first_rows_ms"`
	AllRowsMs         float64 `json:"all_rows_ms"`
	Rows              int     `json:"rows"`
	LongTasks         int     `json:"long_tasks"`
	LongTaskMaxMs     float64 `json:"long_task_max_ms"`
	Loafs             int     `json:"loafs"`
	LoafMaxMs         float64 `json:"loaf_max_ms"`
	LoafMaxBlockingMs float64 `json:"loaf_max_blocking_ms"`
}

type streamResult struct {
	Frames        int     `json:"frames"`
	Seconds       float64 `json:"seconds"`
	FPS           float64 `json:"fps"`
	FrameP95Ms    float64 `json:"frame_p95_ms"`
	FrameMaxMs    float64 `json:"frame_max_ms"`
	Over18MsPct   float64 `json:"over_18_ms_pct"`
	Over33MsPct   float64 `json:"over_33_ms_pct"`
	Keys          int     `json:"keys"`
	KeyPaintP50Ms float64 `json:"key_paint_p50_ms"`
	KeyPaintP95Ms float64 `json:"key_paint_p95_ms"`
	KeyPaintMaxMs float64 `json:"key_paint_max_ms"`
	LongTasks     int     `json:"long_tasks"`
	LongTaskMaxMs float64 `json:"long_task_max_ms"`
	Loafs         int     `json:"loafs"`
	LoafMaxMs     float64 `json:"loaf_max_ms"`
}

type loadResult struct {
	Streams int    `json:"streams"` // answers streaming at once, at the peak
	Samples int    `json:"samples"`
	Peak    memory `json:"peak"`
	After   memory `json:"after"`
}

type report struct {
	Started     time.Time     `json:"started"`
	Took        float64       `json:"took_s"`
	App         string        `json:"app"`
	Rate        float64       `json:"rate"`
	Reply       int           `json:"reply_tokens"`
	Starts      []startResult `json:"starts"`
	Idle        memory        `json:"idle"`
	Opens       []openResult  `json:"opens"`
	Stream      streamResult  `json:"stream"`
	AfterOpens  memory        `json:"after_opens"`
	AfterStream memory        `json:"after_stream"`
	Load        loadResult    `json:"load"`
	Settle      settleResult  `json:"settle"`
}

func (rep *report) write(dir string) (string, error) {
	name := rep.Started.Local().Format("20060102-150405")
	b, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, name+".json"), b, 0o644); err != nil {
		return "", err
	}
	p := filepath.Join(dir, name+".md")
	return p, os.WriteFile(p, []byte(rep.markdown()), 0o644)
}
