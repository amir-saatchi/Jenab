// Package spike holds what the test app, the feed server and the driver share:
// the per-scenario app config and the JSONL event log.
package spike

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

// EnvConfig names the env var that points the app at its JSON config. The
// updater's helper inherits the parent's environment and passes it on to the
// relaunched app (only WAILS_UPDATER_HELPER_* are cleared), so the new version
// reads the same config. Command-line flags are NOT passed on: the helper
// relaunches the target with no arguments.
const EnvConfig = "SPIKE019_CONFIG"

// AppConfig is written by the driver for each scenario.
type AppConfig struct {
	Scenario string `json:"scenario"`
	Log      string `json:"log"`     // JSONL file, shared by every process of the scenario
	DataDir  string `json:"dataDir"` // app.db, jobs.json, backups, webview2 data

	Provider string `json:"provider"` // "endpoint" | "github" | "endpoint-tls"
	FeedURL  string `json:"feedURL"`  // endpoint: manifest URL; github: BaseURL
	CAFile   string `json:"caFile"`   // PEM trusted only via the provider's HTTPClient

	// Mode: "automatic" (check, download, restart when idle), "notify"
	// (check only), "off" (never check), "stage" (check + download + verify,
	// no restart: used for the rejection cases), "poll" (Config.CheckInterval).
	Mode string `json:"mode"`

	RequireSignature bool  `json:"requireSignature"` // wrap providers: refuse releases without a signature
	BusyMs           int   `json:"busyMs"`           // fake pipeline run from start
	Child            string `json:"child"`           // "", "plain", "job"
	ChildSeconds     int   `json:"childSeconds"`
	JobOffsetMs      int   `json:"jobOffsetMs"`      // >0: schedule a job at restart+offset (falls into the downtime)
	LingerMs         int   `json:"lingerMs"`         // how long a process stays up after it has nothing left to do
	CheckIntervalMs  int   `json:"checkIntervalMs"`  // mode "poll"
	ShutdownDelayMs  int   `json:"shutdownDelayMs"`  // sleep in OnShutdown (widen the helper's wait-for-parent phase)
	HelperTimeoutMs  int   `json:"helperTimeoutMs"`  // Config.HelperReadyTimeout
	FailMigration    bool  `json:"failMigration"`    // abort the v1->v2 migration before commit
	NoWindow         bool  `json:"noWindow"`         // no main window (not used by default)
	GHToken          string `json:"ghToken"`          // fake token for the local GitHub fake (shows header stripping)
	ChildVersion     string `json:"childVersion"`     // only this app version starts the child
}

func Load(path string) (*AppConfig, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c AppConfig
	return &c, json.Unmarshal(b, &c)
}

func (c *AppConfig) Save(path string) error {
	b, _ := json.MarshalIndent(c, "", "  ")
	return os.WriteFile(path, b, 0o644)
}

// Logger appends one JSON object per line. Several processes write the same
// file; each line is a single O_APPEND write.
type Logger struct {
	mu   sync.Mutex
	path string
	base map[string]any
}

func NewLogger(path string, base map[string]any) *Logger {
	return &Logger{path: path, base: base}
}

func (l *Logger) Log(ev string, kv map[string]any) {
	if l == nil || l.path == "" {
		return
	}
	m := map[string]any{"t": time.Now().Format(time.RFC3339Nano), "ev": ev}
	for k, v := range l.base {
		m[k] = v
	}
	for k, v := range kv {
		m[k] = v
	}
	b, err := json.Marshal(m)
	if err != nil {
		b, _ = json.Marshal(map[string]any{"t": m["t"], "ev": ev, "marshalErr": err.Error()})
	}
	b = append(b, '\n')
	l.mu.Lock()
	defer l.mu.Unlock()
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	_, _ = f.Write(b)
	_ = f.Close()
}

// Entry is one parsed log line.
type Entry map[string]any

func (e Entry) S(k string) string {
	if v, ok := e[k].(string); ok {
		return v
	}
	return ""
}

func (e Entry) F(k string) float64 {
	if v, ok := e[k].(float64); ok {
		return v
	}
	return 0
}

func (e Entry) T() time.Time {
	t, _ := time.Parse(time.RFC3339Nano, e.S("t"))
	return t
}

func ReadLog(path string) []Entry {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []Entry
	start := 0
	for i := 0; i <= len(b); i++ {
		if i == len(b) || b[i] == '\n' {
			line := b[start:i]
			start = i + 1
			if len(line) == 0 {
				continue
			}
			var e Entry
			if json.Unmarshal(line, &e) == nil {
				out = append(out, e)
			}
		}
	}
	return out
}

// ReadLogBytes parses JSONL bytes.
func ReadLogBytes(b []byte) []Entry {
	var out []Entry
	for _, line := range splitLines(b) {
		var e Entry
		if json.Unmarshal(line, &e) == nil {
			out = append(out, e)
		}
	}
	return out
}

func splitLines(b []byte) [][]byte {
	var out [][]byte
	start := 0
	for i := 0; i <= len(b); i++ {
		if i == len(b) || b[i] == '\n' {
			if i > start {
				out = append(out, b[start:i])
			}
			start = i + 1
		}
	}
	return out
}
