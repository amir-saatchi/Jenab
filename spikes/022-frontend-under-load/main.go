// SPIKE-022: frontend under load. One Wails v3 window runs one scenario
// (chosen by the runner via -q), the page measures itself, calls Spike.Report,
// and the app writes out/<name>.json and quits.
package main

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

var (
	flagName    = flag.String("name", "manual", "result file base name")
	flagQuery   = flag.String("q", "s=idle", "scenario query string passed to the page")
	flagOut     = flag.String("out", "out", "output directory")
	flagTimeout = flag.Int("timeout", 300, "seconds before the app gives up and writes an error result")
	flagData    = flag.String("wvdata", "", "WebView2 user data folder (default out/wv-data)")
	flagManual  = flag.Bool("manual", false, "stay open, do not quit after the report")
)

var (
	mainStart = float64(time.Now().UnixMicro()) / 1000
	cpu       = &cpuSampler{}
	logMu     sync.Mutex
	logFile   *os.File
)

func logf(format string, a ...any) {
	logMu.Lock()
	defer logMu.Unlock()
	if logFile != nil {
		fmt.Fprintf(logFile, "%s "+format+"\n", append([]any{time.Now().Format("15:04:05.000")}, a...)...)
	}
}

type Spike struct {
	app       *application.App
	win       *application.WebviewWindow
	hwnd      uintptr
	winShown  float64
	reported  atomic.Bool
	keysSent  atomic.Int64
	keysSkip  atomic.Int64
	reply     []string
	replyText string
}

type Info struct {
	Name           string  `json:"name"`
	Query          string  `json:"query"`
	ProcStartEpoch float64 `json:"procStartEpoch"`
	MainStartEpoch float64 `json:"mainStartEpoch"`
	WinShownEpoch  float64 `json:"winShownEpoch"`
	ReplyTokens    int     `json:"replyTokens"`
	ReplyChars     int     `json:"replyChars"`
}

func (s *Spike) Hello() Info {
	s.ensureHwnd()
	return Info{*flagName, *flagQuery, processStartEpochMs(), mainStart, s.winShown, len(s.reply), len(s.replyText)}
}

func (s *Spike) ensureHwnd() {
	if s.hwnd == 0 && s.win != nil {
		s.hwnd = uintptr(s.win.NativeWindow())
	}
}

func (s *Spike) Noop() int { return 1 }

func (s *Spike) Echo(n int) string { return string(make([]byte, n)) }

func (s *Spike) Log(msg string) { logf("js: %s", msg) }

func (s *Spike) Mem() MemSnapshot { return memSnapshot() }

func (s *Spike) GetRows(offset, limit int) PageArr    { return pageArr(offset, limit) }
func (s *Spike) GetRowsObj(offset, limit int) PageObj { return pageObj(offset, limit) }

// RowsViaEvent answers with a "rows" event instead of a return value.
func (s *Spike) RowsViaEvent(offset, limit int) {
	go s.app.Event.Emit("rows", pageArr(offset, limit))
}

func (s *Spike) History(n int) []HistMsg { return history(n) }

func (s *Spike) ReplyText() string { return s.replyText }

func (s *Spike) Chart(n int) ChartData { return chartData(n) }

type Tok struct {
	I  int     `json:"i"`
	T  string  `json:"t"`
	TS float64 `json:"ts"` // Go emit time, epoch ms
}

type EmitStats struct {
	ID          string  `json:"id"`
	N           int     `json:"n"`
	Rate        int     `json:"rate"`
	StartEpoch  float64 `json:"startEpoch"`
	EndEpoch    float64 `json:"endEpoch"`
	AchievedHz  float64 `json:"achievedHz"`
	EmitTotalMs float64 `json:"emitTotalMs"` // time spent inside Emit (backpressure shows up here)
	EmitMaxMs   float64 `json:"emitMaxMs"`
	LateMaxMs   float64 `json:"lateMaxMs"` // worst lag of the pacing loop behind schedule
}

func nowMs() float64 { return float64(time.Now().UnixMicro()) / 1000 }

// emitPaced emits n events at rate/s (rate 0 = as fast as possible).
func (s *Spike) emitPaced(id, name string, rate, n int, text func(i int) string) EmitStats {
	st := EmitStats{ID: id, N: n, Rate: rate}
	start := time.Now()
	st.StartEpoch = nowMs()
	for i := range n {
		if rate > 0 {
			due := start.Add(time.Duration(float64(i) * float64(time.Second) / float64(rate)))
			if d := time.Until(due); d > 0 {
				time.Sleep(d)
			} else {
				st.LateMaxMs = max(st.LateMaxMs, float64(-d.Microseconds())/1000)
			}
		}
		t0 := time.Now()
		s.app.Event.Emit(name, Tok{I: i, T: text(i), TS: nowMs()})
		d := float64(time.Since(t0).Microseconds()) / 1000
		st.EmitTotalMs += d
		st.EmitMaxMs = max(st.EmitMaxMs, d)
	}
	st.EndEpoch = nowMs()
	st.AchievedHz = float64(n) / time.Since(start).Seconds()
	return st
}

// StartStream streams the prepared reply as "tok" events, then sends "tok-end".
func (s *Spike) StartStream(id string, rate int) {
	go func() {
		st := s.emitPaced(id, "tok", rate, len(s.reply), func(i int) string { return s.reply[i] })
		logf("stream %s done: %+v", id, st)
		s.app.Event.Emit("tok-end", st)
	}()
}

// Flood emits n tiny events at rate/s as "fl", then "fl-end".
func (s *Spike) Flood(id string, rate, n int) {
	go func() {
		st := s.emitPaced(id, "fl", rate, n, func(int) string { return "tok " })
		logf("flood %s done: %+v", id, st)
		s.app.Event.Emit("fl-end", st)
	}()
}

// TypeKeys presses 'A' every intervalMs, count times, only while our window is in front.
func (s *Spike) TypeKeys(count, intervalMs int) {
	s.ensureHwnd()
	setForeground(s.hwnd)
	go func() {
		for range count {
			time.Sleep(time.Duration(intervalMs) * time.Millisecond)
			if pressKey(s.hwnd, 'A') {
				s.keysSent.Add(1)
			} else {
				s.keysSkip.Add(1)
			}
		}
	}()
}

type Phase struct {
	Name  string  `json:"name"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

// Report writes the result file. data is the page's JSON; phases are windows (epoch ms) for CPU stats.
func (s *Spike) Report(data string, phases []Phase) error {
	if !s.reported.CompareAndSwap(false, true) {
		return nil
	}
	s.ensureHwnd()
	mem := memSnapshot()
	res := map[string]any{
		"name":       *flagName,
		"query":      *flagQuery,
		"wails":      wailsVersion(),
		"memAtEnd":   mem,
		"foreground": foregroundIs(s.hwnd),
		"keysSent":   s.keysSent.Load(),
		"keysSkip":   s.keysSkip.Load(),
		"info":       s.Hello(),
	}
	var page any
	if err := json.Unmarshal([]byte(data), &page); err != nil {
		res["pageError"] = err.Error()
	}
	res["page"] = page
	cs := map[string]CPUStats{}
	for _, p := range phases {
		cs[p.Name] = cpu.stats(int64(p.Start), int64(p.End))
	}
	cs["whole"] = cpu.stats(0, time.Now().UnixMilli())
	res["cpu"] = cs
	if err := writeResult(res); err != nil {
		return err
	}
	logf("report written")
	if !*flagManual {
		go func() { time.Sleep(200 * time.Millisecond); s.app.Quit() }()
	}
	return nil
}

func writeResult(res map[string]any) error {
	b, err := json.MarshalIndent(res, "", " ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(*flagOut, *flagName+".json"), b, 0o644)
}

func main() {
	flag.Parse()
	os.MkdirAll(*flagOut, 0o755)
	logFile, _ = os.Create(filepath.Join(*flagOut, *flagName+".log"))
	go cpu.run(250 * time.Millisecond)

	wv := *flagData
	if wv == "" {
		wv = filepath.Join(*flagOut, "wv-data")
	}
	wv, _ = filepath.Abs(wv)

	spike := &Spike{}
	spike.replyText = reply(1, 2000)
	spike.reply = tokenize(spike.replyText)

	sub, err := fs.Sub(assets, "frontend/dist")
	if err != nil {
		log.Fatal(err)
	}
	app := application.New(application.Options{
		Name:         "Burrow SPIKE-022",
		Assets:       application.AssetOptions{Handler: application.AssetFileServerFS(sub)},
		Services:     []application.Service{application.NewService(spike)},
		Windows:      application.WindowsOptions{WebviewUserDataPath: wv},
		Logger:       slog.New(slog.NewTextHandler(logWriter{}, nil)),
		ErrorHandler: func(err error) { logf("wails error: %v", err) },
		PanicHandler: func(p *application.PanicDetails) { logf("wails panic: %+v", p) },
	})
	spike.app = app
	registerStreams(app)

	spike.win = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            fmt.Sprintf("SPIKE-022 %s (pid %d)", *flagName, os.Getpid()),
		Width:            1280,
		Height:           800,
		URL:              "/?" + *flagQuery + "&name=" + *flagName,
		BackgroundColour: application.NewRGB(255, 255, 255),
	})
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		spike.winShown = nowMs()
		spike.hwnd = uintptr(spike.win.NativeWindow())
		logf("app started, hwnd %x", spike.hwnd)
		application.InvokeSync(func() { spike.win.Focus() })
		setForeground(spike.hwnd)
	})
	go func() {
		time.Sleep(time.Duration(*flagTimeout) * time.Second)
		if spike.reported.CompareAndSwap(false, true) {
			logf("timeout")
			writeResult(map[string]any{"name": *flagName, "query": *flagQuery, "error": "timeout", "memAtEnd": memSnapshot()})
			app.Quit()
		}
	}()
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

type Cost struct {
	GenMs     float64 `json:"genMs"`
	MarshalMs float64 `json:"marshalMs"`
	Bytes     int     `json:"bytes"`
}

// RowsCost reports the Go-side share of a GetRows call: building the page and JSON-encoding it.
func (s *Spike) RowsCost(offset, limit int) Cost {
	t0 := time.Now()
	p := pageArr(offset, limit)
	t1 := time.Now()
	b, _ := json.Marshal(p)
	t2 := time.Now()
	return Cost{float64(t1.Sub(t0).Microseconds()) / 1000, float64(t2.Sub(t1).Microseconds()) / 1000, len(b)}
}

// logWriter sends Wails system logs to the run's log file.
type logWriter struct{}

func (logWriter) Write(p []byte) (int, error) {
	logMu.Lock()
	defer logMu.Unlock()
	if logFile != nil {
		logFile.Write(p)
	}
	return len(p), nil
}
