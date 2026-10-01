// SPIKE-019 test app: a Wails v3 beta.26 app that updates itself with
// pkg/updater from a local feed and drives its own scenario unattended.
package main

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/endpoint"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"
	"golang.org/x/sys/windows"

	"burrow/spikes/selfupdate/internal/spike"
)

// Injected with -ldflags -X.
var (
	version   = "0.0.0"
	pubKeyB64 = "" // raw 32-byte ed25519 public key, base64; empty = no key built in
	nonce     = "" // makes otherwise identical builds hash differently (Defender test)
)

//go:embed page.html
var pageHTML string

var (
	lg      *spike.Logger
	cfg     *spike.AppConfig
	busy    atomic.Bool
	startAt = time.Now()
)

func main() {
	cfgPath := os.Getenv(spike.EnvConfig)
	if cfgPath != "" {
		if c, err := spike.Load(cfgPath); err == nil {
			cfg = c
		}
	}
	if cfg == nil {
		cfg = &spike.AppConfig{}
	}
	lg = spike.NewLogger(cfg.Log, map[string]any{"pid": os.Getpid(), "ver": version, "sc": cfg.Scenario})
	exe, _ := os.Executable()
	// First line, before anything else: shows that the updater helper runs
	// OUR main() up to the point where HandleHelperMode is reached.
	lg.Log("main-entered", map[string]any{
		"ppid": os.Getppid(), "args": os.Args[1:], "exe": exe,
		"helperEnv": os.Getenv("WAILS_UPDATER_HELPER"), "inJob": jobInfo(),
	})

	// Our own child mode is a command-line flag. Helper mode is an env var
	// and the helper is started with no arguments, so the two cannot collide.
	if len(os.Args) > 1 && os.Args[1] == "--child" {
		runChild(os.Args[2:])
		return
	}
	if os.Getenv("WAILS_UPDATER_HELPER") == "1" {
		lg.Log("helper-mode", map[string]any{
			"target": os.Getenv("WAILS_UPDATER_HELPER_TARGET"), "new": os.Getenv("WAILS_UPDATER_HELPER_NEW"),
			"parent": os.Getenv("WAILS_UPDATER_HELPER_PID"), "helperLog": os.Getenv("WAILS_UPDATER_HELPER_LOG"),
		})
	}
	// Call it ourselves before any side effect (DB, scheduler). application.New
	// calls it too, but everything main() does before New would also run in
	// the helper process.
	updater.HandleHelperMode()

	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil && cfg.DataDir != "" {
		lg.Log("fatal", map[string]any{"err": err.Error()})
		os.Exit(2)
	}
	exeHash := fileSHA(exe)
	lg.Log("start", map[string]any{"exeSHA": exeHash[:16], "mode": cfg.Mode, "nonce": nonce})

	db, err := openDB(filepath.Join(cfg.DataDir, "app.db"))
	if err != nil {
		lg.Log("db-refused", map[string]any{"err": err.Error()})
		os.Exit(3)
	}
	sched := newScheduler(filepath.Join(cfg.DataDir, "jobs.json"))
	sched.start()

	app := application.New(application.Options{
		Name: "BurrowSpike019",
		Assets: application.AssetOptions{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = io.WriteString(w, strings.ReplaceAll(pageHTML, "{{VERSION}}", version))
		})},
		// One WebView2 profile shared by all scenarios (as a real install would keep it).
		Windows: application.WindowsOptions{WebviewUserDataPath: filepath.Join(filepath.Dir(cfg.DataDir), "webview2")},
		OnShutdown: func() {
			lg.Log("on-shutdown", nil)
			if cfg.ShutdownDelayMs > 0 {
				time.Sleep(time.Duration(cfg.ShutdownDelayMs) * time.Millisecond)
			}
		},
	})

	var once sync.Once
	pageReady := make(chan struct{})
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		lg.Log("app-started", map[string]any{"sinceMainMs": ms(time.Since(startAt))})
	})
	app.Event.On("spike:page-ready", func(e *application.CustomEvent) {
		lg.Log("window-ready", map[string]any{"sinceMainMs": ms(time.Since(startAt))})
		once.Do(func() { close(pageReady) })
	})
	app.Event.On("spike:js-event", func(e *application.CustomEvent) {
		lg.Log("js-saw", map[string]any{"data": e.Data})
	})
	for _, name := range []string{
		updater.EventCheckStarted, updater.EventUpdateAvailable, updater.EventNoUpdate,
		updater.EventDownloadStarted, updater.EventDownloadProgress, updater.EventDownloadComplete,
		updater.EventVerifying, updater.EventInstalling, updater.EventUpdateReady, updater.EventError,
		updater.EventMeta,
	} {
		n := name
		app.Event.On(n, func(e *application.CustomEvent) {
			lg.Log("go-saw", map[string]any{"event": n, "data": summarise(e.Data)})
		})
	}

	if !cfg.NoWindow {
		app.Window.NewWithOptions(application.WebviewWindowOptions{
			Title: "BurrowSpike019 " + version, Width: 360, Height: 220, URL: "/",
			X: 40, Y: 40, InitialPosition: application.WindowXY,
		})
	} else {
		close(pageReady)
	}

	go func() {
		select {
		case <-pageReady:
		case <-time.After(20 * time.Second):
			lg.Log("page-ready-timeout", nil)
		}
		scenario(app, db, sched, exe, exeHash)
	}()

	err = app.Run()
	lg.Log("run-returned", map[string]any{"err": errString(err)})
	sched.stop()
	_ = db.Close()
	lg.Log("exiting", nil)
}

func scenario(app *application.App, db *appDB, sched *scheduler, exe, exeHash string) {
	ctx := context.Background()
	quit := func(reason string) {
		linger()
		lg.Log("quit", map[string]any{"reason": reason})
		app.Quit()
	}
	if cfg.BusyMs > 0 {
		busy.Store(true)
		go db.fakePipeline(time.Duration(cfg.BusyMs) * time.Millisecond)
	}
	if cfg.Child != "" && (cfg.ChildVersion == "" || cfg.ChildVersion == version) {
		startChild(exe)
	}
	if cfg.Mode == "off" || cfg.Mode == "" {
		// Updates off: never Init, never check. Stay up for the linger time
		// so the driver can confirm the feed sees nothing.
		quit("mode off")
		return
	}

	pub, _ := base64.StdEncoding.DecodeString(pubKeyB64)
	provs, err := providers()
	if err != nil {
		lg.Log("provider-error", map[string]any{"err": err.Error()})
		quit("provider error")
		return
	}
	ucfg := updater.Config{
		CurrentVersion: version, Providers: provs, PublicKey: pub,
		Window: updater.WindowNone,
	}
	if cfg.HelperTimeoutMs > 0 {
		ucfg.HelperReadyTimeout = time.Duration(cfg.HelperTimeoutMs) * time.Millisecond
	}
	if cfg.Mode == "poll" {
		ucfg.CheckInterval = time.Duration(cfg.CheckIntervalMs) * time.Millisecond
	}
	if err := app.Updater.Init(ucfg); err != nil {
		lg.Log("init-error", map[string]any{"err": err.Error()})
		quit("init error")
		return
	}
	lg.Log("updater-init", map[string]any{"keyBuiltIn": len(pub) > 0, "checkInterval": ucfg.CheckInterval.String()})
	if cfg.Mode == "poll" {
		quit("poll done")
		return
	}

	rel, err := app.Updater.Check(ctx)
	if err != nil {
		lg.Log("check-result", map[string]any{"result": "error", "err": err.Error()})
		stillAlive(db, exe, exeHash)
		quit("check error")
		return
	}
	if rel == nil {
		lg.Log("check-result", map[string]any{"result": "no-update"})
		quit("no update")
		return
	}
	lg.Log("check-result", map[string]any{"result": "available", "version": rel.Version, "hasDigest": rel.Verification != nil && len(rel.Verification.Digest) > 0,
		"hasSig": rel.Verification != nil && len(rel.Verification.Signature) > 0})
	if cfg.Mode == "notify" {
		quit("notify only")
		return
	}

	t0 := time.Now()
	if err := app.Updater.DownloadAndInstall(ctx); err != nil {
		lg.Log("install-result", map[string]any{"result": "rejected", "err": err.Error(), "ms": ms(time.Since(t0))})
		stillAlive(db, exe, exeHash)
		quit("install rejected")
		return
	}
	staged := app.Updater.DownloadedPath()
	lg.Log("install-result", map[string]any{"result": "staged", "path": staged, "ms": ms(time.Since(t0)), "stagedSHA": fileSHA(staged)[:16], "busy": busy.Load()})
	if cfg.Mode == "stage" {
		stillAlive(db, exe, exeHash)
		quit("staged only")
		return
	}

	// Safe moment: defer the restart until no pipeline run / write is active.
	if busy.Load() {
		lg.Log("restart-deferred", map[string]any{"why": "pipeline run in progress"})
		for busy.Load() {
			time.Sleep(50 * time.Millisecond)
		}
		lg.Log("idle-now", nil)
	}
	if cfg.JobOffsetMs > 0 {
		sched.stop() // the old version will not see the job come due
		due := time.Now().Add(time.Duration(cfg.JobOffsetMs) * time.Millisecond)
		sched.add("nightly", due)
		lg.Log("job-scheduled", map[string]any{"due": due.Format(time.RFC3339Nano)})
	}
	_ = db.Close()
	lg.Log("restart-called", nil)
	err = app.Updater.Restart(ctx)
	lg.Log("restart-returned", map[string]any{"err": errString(err)})
	if err != nil {
		// Old version keeps running.
		if db2, e := openDB(filepath.Join(cfg.DataDir, "app.db")); e == nil {
			stillAlive(db2, exe, exeHash)
			_ = db2.Close()
		}
		quit("restart failed")
	}
}

func providers() ([]updater.Provider, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	if cfg.CAFile != "" {
		pem, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, err
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, errors.New("bad CA PEM")
		}
		client.Transport = &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}
	}
	var p updater.Provider
	var err error
	switch cfg.Provider {
	case "github", "github-nosums":
		gc := github.Config{Repository: "burrow/burrow", BaseURL: cfg.FeedURL, HTTPClient: client, Token: cfg.GHToken}
		if cfg.Provider == "github" {
			gc.ChecksumAsset = "SHA256SUMS"
		}
		p, err = github.New(gc)
	default:
		p, err = endpoint.New(endpoint.Config{URL: cfg.FeedURL, HTTPClient: client})
	}
	if err != nil {
		return nil, err
	}
	if cfg.RequireSignature {
		p = requireSig{p}
	}
	return []updater.Provider{p}, nil
}

// requireSig is the wrapper Burrow would need: pkg/updater accepts a release
// with no signature (digest-only or no verification at all) even when a
// public key is configured.
type requireSig struct{ updater.Provider }

func (r requireSig) Check(ctx context.Context, req updater.CheckRequest) (*updater.Release, error) {
	rel, err := r.Provider.Check(ctx, req)
	if err != nil || rel == nil {
		return rel, err
	}
	if rel.Verification == nil || len(rel.Verification.Signature) == 0 || rel.Verification.SignatureAlgo != "ed25519" {
		return nil, fmt.Errorf("burrow: release %s is not signed (ed25519 signature required)", rel.Version)
	}
	return rel, nil
}

func stillAlive(db *appDB, exe, exeHash string) {
	n, err := db.touch()
	lg.Log("still-alive", map[string]any{"dbRows": n, "dbErr": errString(err), "exeUnchanged": fileSHA(exe) == exeHash})
}

func linger() {
	if cfg.LingerMs > 0 {
		time.Sleep(time.Duration(cfg.LingerMs) * time.Millisecond)
	}
}

func summarise(d any) any {
	b, _ := json.Marshal(d)
	if len(b) > 400 {
		var m map[string]any
		if json.Unmarshal(b, &m) == nil {
			delete(m, "notes")
			delete(m, "metadata")
			return m
		}
		return string(b[:400])
	}
	return d
}

func fileSHA(p string) string {
	f, err := os.Open(p)
	if err != nil {
		return "missing-----------"
	}
	defer f.Close()
	h := sha256.New()
	_, _ = io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil))
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// jobInfo reports whether this process runs in a Job Object and its flags.
func jobInfo() string {
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	if err := windows.QueryInformationJobObject(0, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)), nil); err != nil {
		return "no (" + err.Error() + ")"
	}
	return fmt.Sprintf("yes flags=0x%x", info.BasicLimitInformation.LimitFlags)
}
