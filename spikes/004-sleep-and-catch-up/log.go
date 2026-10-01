package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// Rec is one log line. Every line carries all clocks, read together.
type Rec struct {
	Seq   int       `json:"seq"`
	Kind  string    `json:"kind"` // start, sample, probe, power, powercb, setting, session, sched, keychain, endsession, stop, error
	UTC   time.Time `json:"utc"`
	Local string    `json:"local"`
	MonoS float64   `json:"mono_s"`  // Go monotonic time since start
	QPC   int64     `json:"qpc"`     // QueryPerformanceCounter
	QPCF  int64     `json:"qpc_hz"`  // its frequency
	IT    uint64    `json:"it"`      // QueryInterruptTime, 100 ns (counts sleep)
	UIT   uint64    `json:"uit"`     // QueryUnbiasedInterruptTime, 100 ns (doesn't)
	Tick  uint64    `json:"tick_ms"` // GetTickCount64
	Desk  string    `json:"desk,omitempty"`

	Name    string   `json:"name,omitempty"`    // probe, event, setting, or scheduler name
	Detail  string   `json:"detail,omitempty"`  // human-readable extra
	Target  string   `json:"target,omitempty"`  // RFC 3339 wall time the probe or job aimed at
	LateS   float64  `json:"late_s,omitempty"`  // seconds after the expected wall time
	WallS   float64  `json:"wall_s,omitempty"`  // wall seconds since the previous fire (or since start)
	MonoDS  float64  `json:"mono_ds,omitempty"` // Go monotonic seconds over the same span
	N       int      `json:"n,omitempty"`
	Trigger string   `json:"trigger,omitempty"` // on_time or catch_up
	Reason  string   `json:"reason,omitempty"`  // what made the scheduler check / the keychain read
	Covers  []string `json:"covers,omitempty"`  // scheduled times covered by a run (RFC 3339)
	Result  string   `json:"result,omitempty"`
	DurMS   float64  `json:"dur_ms,omitempty"`
	Info    []string `json:"info,omitempty"`
}

type logger struct {
	mu    sync.Mutex
	f     *os.File
	start time.Time
	seq   int
	echo  bool
}

func openLog(path string) (*logger, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	return &logger{f: f, start: time.Now(), echo: true}, nil
}

// emit fills the clock fields, writes the line and flushes it to disk.
func (l *logger) emit(r Rec) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	c := readClocks()
	l.seq++
	r.Seq = l.seq
	r.UTC = now.UTC()
	r.Local = now.Format("2006-01-02 15:04:05.000")
	r.MonoS = now.Sub(l.start).Seconds()
	r.QPC, r.QPCF, r.IT, r.UIT, r.Tick = c.qpc, qpcFreq, c.it, c.uit, c.tickMilli
	b, _ := json.Marshal(r)
	b = append(b, '\n')
	l.f.Write(b)
	l.f.Sync()
	if l.echo {
		os.Stdout.Write(b)
	}
}

func (l *logger) errorf(format string, a ...any) {
	l.emit(Rec{Kind: "error", Detail: fmt.Sprintf(format, a...)})
}

func (l *logger) close() { l.f.Close() }
