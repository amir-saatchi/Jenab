package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// Native Ollama API helpers (/api/*), memory sampling and server-log parsing.

var native = &http.Client{Timeout: 7 * time.Minute}

func apiPost(path string, body any, out any) error {
	b, _ := json.Marshal(body)
	r, err := native.Post(*host+path, "application/json", bytes.NewReader(b))
	if err != nil {
		return err
	}
	defer r.Body.Close()
	data, _ := io.ReadAll(r.Body)
	if r.StatusCode != 200 {
		return fmt.Errorf("%s: HTTP %d: %s", path, r.StatusCode, strings.TrimSpace(string(data)))
	}
	if out != nil {
		// /api/create streams status lines; keep the last one.
		lines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))
		return json.Unmarshal(lines[len(lines)-1], out)
	}
	return nil
}

func apiGet(path string, out any) error {
	c := http.Client{Timeout: 10 * time.Second}
	r, err := c.Get(*host + path)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(out)
}

type psModel struct {
	Name          string    `json:"name"`
	Size          int64     `json:"size"`
	ContextLength int       `json:"context_length"`
	ExpiresAt     time.Time `json:"expires_at"`
}

func ps() []psModel {
	var r struct{ Models []psModel }
	_ = apiGet("/api/ps", &r)
	return r.Models
}

func loaded(model string) *psModel {
	for _, m := range ps() {
		if m.Name == model || m.Name == model+":latest" {
			return &m
		}
	}
	return nil
}

// unload asks Ollama to drop a model from memory now (keep_alive 0).
func unload(model string) {
	_ = apiPost("/api/generate", map[string]any{"model": model, "keep_alive": 0}, nil)
	for i := 0; i < 40 && loaded(model) != nil; i++ {
		time.Sleep(250 * time.Millisecond)
	}
}

// only makes sure no other model is loaded, so two runners never share the 16 GB.
func only(model string) {
	for _, m := range ps() {
		if m.Name != model && m.Name != model+":latest" {
			unload(m.Name)
		}
	}
}

// createDerived makes a model that shares the weights of base but has its own parameters.
func createDerived(name string, params map[string]any) error {
	var st struct {
		Status string `json:"status"`
		Error  string `json:"error"`
	}
	err := apiPost("/api/create", map[string]any{"model": name, "from": *model, "parameters": params}, &st)
	if err == nil && st.Error != "" {
		err = fmt.Errorf("%s", st.Error)
	}
	return err
}

func deleteModel(name string) error {
	req, _ := http.NewRequest("DELETE", *host+"/api/delete", strings.NewReader(fmt.Sprintf(`{"model":%q}`, name)))
	r, err := native.Do(req)
	if err != nil {
		return err
	}
	r.Body.Close()
	if r.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", r.StatusCode)
	}
	return nil
}

// nativeChat is /api/chat without streaming. It returns Ollama's own timings.
type nativeRes struct {
	Message struct {
		Content  string `json:"content"`
		Thinking string `json:"thinking"`
	} `json:"message"`
	DoneReason         string `json:"done_reason"`
	TotalDuration      int64  `json:"total_duration"`
	LoadDuration       int64  `json:"load_duration"`
	PromptEvalCount    int    `json:"prompt_eval_count"`
	PromptEvalDuration int64  `json:"prompt_eval_duration"`
	EvalCount          int    `json:"eval_count"`
	EvalDuration       int64  `json:"eval_duration"`
}

func nativeChat(body map[string]any) (*nativeRes, error) {
	body["stream"] = false
	var r nativeRes
	err := apiPost("/api/chat", body, &r)
	return &r, err
}

// ---------------- memory ----------------

type memSample struct {
	RunnerWS  int64 // largest ollama.exe working set (the model runner)
	AvailPhys uint64
	TotalPhys uint64
}

type memStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

var procMemStatus = syscall.NewLazyDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx")

func memNow() memSample {
	var m memStatusEx
	m.Length = uint32(unsafe.Sizeof(m))
	procMemStatus.Call(uintptr(unsafe.Pointer(&m)))
	s := memSample{AvailPhys: m.AvailPhys, TotalPhys: m.TotalPhys}
	out, err := exec.Command("tasklist", "/FI", "IMAGENAME eq ollama.exe", "/FO", "CSV", "/NH").Output()
	if err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			f := strings.Split(strings.TrimSpace(line), "\",\"")
			if len(f) < 5 {
				continue
			}
			kb := strings.NewReplacer("\"", "", ",", "", ".", "", " K", "", " ", "", " ", "").Replace(f[4])
			if v, err := strconv.ParseInt(kb, 10, 64); err == nil && v*1024 > s.RunnerWS {
				s.RunnerWS = v * 1024
			}
		}
	}
	return s
}

func gib(b int64) string  { return fmt.Sprintf("%.1f GiB", float64(b)/(1<<30)) }
func gibu(b uint64) string { return gib(int64(b)) }

// ---------------- server log ----------------

// srvLog reads the log of the `ollama serve` process given by -serverlog (OLLAMA_DEBUG=1 adds the
// "loading cache slot" lines that show how much of the prompt was reused).
type srvLog struct {
	path string
	off  int64
}

func (l *srvLog) mark() {
	if l == nil || l.path == "" {
		return
	}
	if fi, err := os.Stat(l.path); err == nil {
		l.off = fi.Size()
	}
}

func (l *srvLog) lines() []string {
	if l == nil || l.path == "" {
		return nil
	}
	f, err := os.Open(l.path)
	if err != nil {
		return nil
	}
	defer f.Close()
	f.Seek(l.off, 0)
	var out []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<22)
	for sc.Scan() {
		out = append(out, sc.Text())
	}
	return out
}

var (
	reSlot     = regexp.MustCompile(`loading cache slot.*cache=(\d+) prompt=(\d+) used=(\d+) remaining=(\d+)`)
	reTrunc    = regexp.MustCompile(`(?i)truncat[^"]*"?[^\n]{0,200}`)
	reKv       = regexp.MustCompile(`KvSize:(\d+).*NumThreads:(\d+)`)
	reKvCache  = regexp.MustCompile(`msg="kv cache" device=\w+ size="([^"]+)"`)
	reTotalMem = regexp.MustCompile(`msg="total memory" size="([^"]+)"`)
)

type logInfo struct {
	Slots    [][4]int // cache, prompt, used, remaining
	Trunc    []string
	KvSize   int
	Threads  int
	KvCache  string
	TotalMem string
	Have     bool
}

func (l *srvLog) info() logInfo {
	var li logInfo
	ls := l.lines()
	li.Have = l != nil && l.path != ""
	for _, s := range ls {
		if m := reSlot.FindStringSubmatch(s); m != nil {
			var v [4]int
			for i := range v {
				v[i], _ = strconv.Atoi(m[i+1])
			}
			li.Slots = append(li.Slots, v)
		}
		if strings.Contains(strings.ToLower(s), "truncat") {
			li.Trunc = append(li.Trunc, reTrunc.FindString(s))
		}
		if m := reKv.FindStringSubmatch(s); m != nil {
			li.KvSize, _ = strconv.Atoi(m[1])
			li.Threads, _ = strconv.Atoi(m[2])
		}
		if m := reKvCache.FindStringSubmatch(s); m != nil {
			li.KvCache = m[1]
		}
		if m := reTotalMem.FindStringSubmatch(s); m != nil {
			li.TotalMem = m[1]
		}
	}
	return li
}

func (li logInfo) slot() string {
	if !li.Have {
		return "(no -serverlog)"
	}
	if len(li.Slots) == 0 {
		return "(no slot line)"
	}
	var p []string
	for _, s := range li.Slots {
		p = append(p, fmt.Sprintf("used %d of %d", s[2], s[1]))
	}
	return strings.Join(p, "; ")
}

// appServerConfig reads the context length that the Ollama tray app passes to the server it starts
// (from %LOCALAPPDATA%\Ollama\server*.log), newest file first.
func appServerConfig() (ctxLen, kv string) {
	dir := filepath.Join(os.Getenv("LOCALAPPDATA"), "Ollama")
	files, _ := filepath.Glob(filepath.Join(dir, "server*.log"))
	sort.Slice(files, func(i, j int) bool {
		a, _ := os.Stat(files[i])
		b, _ := os.Stat(files[j])
		return a.ModTime().After(b.ModTime())
	})
	re := regexp.MustCompile(`OLLAMA_CONTEXT_LENGTH:(\d+)`)
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		if m := re.FindAllSubmatch(data, -1); m != nil {
			ctxLen = string(m[len(m)-1][1])
			if k := reKv.FindAllSubmatch(data, -1); k != nil {
				kv = string(k[len(k)-1][1])
			}
			return ctxLen + " (" + filepath.Base(f) + ")", kv
		}
	}
	return "", ""
}
