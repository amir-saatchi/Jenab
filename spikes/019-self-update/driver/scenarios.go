package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"burrow/spikes/selfupdate/internal/feed"
	"burrow/spikes/selfupdate/internal/spike"
)

var binCache = map[string][]byte{}

func bin(name string) []byte {
	if b, ok := binCache[name]; ok {
		return b
	}
	b, err := os.ReadFile(filepath.Join(binDir, name))
	must(err)
	binCache[name] = b
	return b
}

const manifestPath = "/m/manifest.json"

func manifestURL() string { return fd.HTTPURL + manifestPath }

// art describes one artifact entry. body is what gets signed / digested;
// serve (if set) is what the server actually sends.
type art struct {
	name       string
	body       []byte
	serve      *feed.Route
	key        ed25519.PrivateKey // nil = no signature
	noDigest   bool
	digestOf   []byte // digest (and signature) of these bytes instead of body
	platform   string
	arch       string
	sizeLie    int64
	dropAlgo   bool
}

func (a art) entry() map[string]any {
	src := a.body
	if a.digestOf != nil {
		src = a.digestOf
	}
	d := sha(src)
	m := map[string]any{
		"url": "/files/" + a.name, "filename": exeName,
		"platform": "windows", "arch": "amd64", "size": len(a.body),
	}
	if a.platform != "" {
		m["platform"], m["arch"] = a.platform, a.arch
	}
	if a.sizeLie != 0 {
		m["size"] = a.sizeLie
	}
	if !a.noDigest {
		m["digestAlgo"] = "sha256"
		m["digest"] = base64.StdEncoding.EncodeToString(d)
	}
	if a.key != nil {
		if !a.dropAlgo {
			m["signatureAlgo"] = "ed25519"
		}
		m["signature"] = base64.StdEncoding.EncodeToString(ed25519.Sign(a.key, d))
	}
	return m
}

func publish(ver string, schema int, arts ...art) {
	var entries []map[string]any
	for _, a := range arts {
		entries = append(entries, a.entry())
		rt := feed.Route{Body: a.body, ContentType: "application/octet-stream"}
		if a.serve != nil {
			rt = *a.serve
		}
		fd.Set("/files/"+a.name, rt)
	}
	m := map[string]any{
		"schemaVersion": schema, "version": ver, "name": "Burrow " + ver,
		"notes": "SPIKE-019 test release", "publishedAt": time.Now().UTC().Format(time.RFC3339),
		"artifacts": entries,
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	fd.Set(manifestPath, feed.Route{Body: b, ContentType: "application/json"})
}

func good(file string) art { return art{name: file, body: bin(file), key: relKey} }

// ghPublish fakes GET /repos/burrow/burrow/releases/latest plus asset downloads.
// sums: "" = no SHA256SUMS asset, "good", "bad". Downloads go through a 302 to
// another host name (localhost vs 127.0.0.1), like GitHub's CDN redirect.
func ghPublish(tag, file, sums string) {
	asset := "BurrowSpike019-windows-amd64.exe"
	body := bin(file)
	port := fd.HTTPURL[strings.LastIndex(fd.HTTPURL, ":")+1:]
	cdn := "http://localhost:" + port + "/cdn/" + tag + "/" + asset
	fd.Set("/gh-dl/"+tag+"/"+asset, feed.Route{Status: 302, Redirect: cdn})
	fd.Set("/cdn/"+tag+"/"+asset, feed.Route{Body: body, ContentType: "application/octet-stream"})
	assets := []map[string]any{{
		"id": 1, "name": asset, "content_type": "application/octet-stream", "size": len(body),
		"browser_download_url": fd.HTTPURL + "/gh-dl/" + tag + "/" + asset,
	}}
	if sums != "" {
		d := sha(body)
		if sums == "bad" {
			d = sha(append([]byte("x"), body...))
		}
		txt := []byte(hex.EncodeToString(d) + "  " + asset + "\n")
		fd.Set("/gh-dl/"+tag+"/SHA256SUMS", feed.Route{Body: txt, ContentType: "text/plain"})
		assets = append(assets, map[string]any{"id": 2, "name": "SHA256SUMS", "content_type": "text/plain", "size": len(txt),
			"browser_download_url": fd.HTTPURL + "/gh-dl/" + tag + "/SHA256SUMS"})
	}
	rel := map[string]any{"tag_name": tag, "name": "Burrow " + tag, "body": "notes", "prerelease": false, "draft": false,
		"published_at": time.Now().UTC().Format(time.RFC3339), "html_url": "http://127.0.0.1/fake", "assets": assets}
	b, _ := json.Marshal(rel)
	fd.Set("/gh/repos/burrow/burrow/releases/latest", feed.Route{Body: b, ContentType: "application/json"})
}

func cfg(mode string) spike.AppConfig {
	return spike.AppConfig{Provider: "endpoint", FeedURL: manifestURL(), Mode: mode, LingerMs: 1500}
}

func runAll() {
	var scns []*Scn
	add := func(s *Scn) {
		if selected(s.Name) {
			scns = append(scns, s)
		}
	}

	// ---- 1. Feed ----
	add(&Scn{Name: "a1-endpoint-http", Group: "feed", Install: "app-1.0.0.exe", Cfg: cfg("automatic"),
		Feed: func() { publish("1.0.1", 1, good("app-1.0.1.exe")) }})
	add(&Scn{Name: "a8-endpoint-slow-download-5MBps", Group: "feed", Install: "app-1.0.0.exe", Cfg: cfg("automatic"),
		Feed: func() {
			publish("1.0.1", 1, art{name: "app-1.0.1.exe", body: bin("app-1.0.1.exe"), key: relKey,
				serve: &feed.Route{Body: bin("app-1.0.1.exe"), RateBps: 5 << 20}})
		}})
	add(&Scn{Name: "a2-github-http", Group: "feed", Install: "app-1.0.0.exe",
		Cfg:  spike.AppConfig{Provider: "github", FeedURL: fd.HTTPURL + "/gh", Mode: "automatic", LingerMs: 1500, GHToken: "fake-token-for-local-test"},
		Feed: func() { ghPublish("v1.0.1", "app-1.0.1.exe", "good") }})
	add(&Scn{Name: "a3-endpoint-https-own-ca", Group: "feed", Install: "app-1.0.0.exe",
		Cfg:  spike.AppConfig{Provider: "endpoint", FeedURL: fd.HTTPSURL + manifestPath, CAFile: filepath.Join(outDir, "feed-ca.pem"), Mode: "stage"},
		Feed: func() { publish("1.0.1", 1, good("app-1.0.1.exe")) }})
	add(&Scn{Name: "a4-endpoint-https-untrusted", Group: "feed", Install: "app-1.0.0.exe",
		Cfg:  spike.AppConfig{Provider: "endpoint", FeedURL: fd.HTTPSURL + manifestPath, Mode: "stage"},
		Feed: func() { publish("1.0.1", 1, good("app-1.0.1.exe")) }})
	add(&Scn{Name: "a5-github-no-sums-asset", Group: "feed", Install: "app-1.0.0.exe",
		Cfg:  spike.AppConfig{Provider: "github", FeedURL: fd.HTTPURL + "/gh", Mode: "stage"},
		Feed: func() { ghPublish("v1.0.1", "app-1.0.1.exe", "") }})
	add(&Scn{Name: "a6-github-bad-sums", Group: "feed", Install: "app-1.0.0.exe",
		Cfg:  spike.AppConfig{Provider: "github", FeedURL: fd.HTTPURL + "/gh", Mode: "stage"},
		Feed: func() { ghPublish("v1.0.1", "app-1.0.1.exe", "bad") }})
	add(&Scn{Name: "a7-github-require-signature", Group: "feed", Install: "app-1.0.0.exe",
		Cfg:  spike.AppConfig{Provider: "github", FeedURL: fd.HTTPURL + "/gh", Mode: "stage", RequireSignature: true},
		Feed: func() { ghPublish("v1.0.1", "app-1.0.1.exe", "good") }})

	// ---- 2. Verification (stage mode: check + download + verify, no restart) ----
	v101 := func() []byte { return bin("app-1.0.1.exe") }
	flip := func() []byte { b := append([]byte{}, v101()...); b[len(b)/2] ^= 0x01; return b }
	vs := []struct {
		name  string
		inst  string
		reqSig bool
		feed  func()
	}{
		{"b01-good-signed", "", false, func() { publish("1.0.1", 1, good("app-1.0.1.exe")) }},
		{"b02-bit-flip", "", false, func() {
			publish("1.0.1", 1, art{name: "app-1.0.1.exe", body: v101(), key: relKey, serve: &feed.Route{Body: flip()}})
		}},
		{"b03-wrong-key", "", false, func() { publish("1.0.1", 1, art{name: "app-1.0.1.exe", body: v101(), key: otherKey}) }},
		{"b04-digest-only-key-configured", "", false, func() { publish("1.0.1", 1, art{name: "app-1.0.1.exe", body: v101()}) }},
		{"b05-digest-only-require-sig", "", true, func() { publish("1.0.1", 1, art{name: "app-1.0.1.exe", body: v101()}) }},
		{"b06-no-digest-no-sig", "", false, func() { publish("1.0.1", 1, art{name: "app-1.0.1.exe", body: v101(), noDigest: true}) }},
		{"b07-sig-but-no-key-built-in", "app-1.0.0-nokey.exe", false, func() { publish("1.0.1", 1, good("app-1.0.1.exe")) }},
		{"b08-digest-mismatch", "", false, func() {
			publish("1.0.1", 1, art{name: "app-1.0.1.exe", body: v101(), key: relKey, digestOf: bin("app-1.0.2.exe")})
		}},
		{"b09-same-version", "", false, func() { publish("1.0.0", 1, good("app-1.0.0.exe")) }},
		{"b10-older-version", "", false, func() { publish("0.9.0", 1, good("app-1.0.0.exe")) }},
		{"b11-truncated", "", false, func() {
			publish("1.0.1", 1, art{name: "app-1.0.1.exe", body: v101(), key: relKey, serve: &feed.Route{Body: v101()[:len(v101())/2]}})
		}},
		{"b12-connection-cut", "", false, func() {
			publish("1.0.1", 1, art{name: "app-1.0.1.exe", body: v101(), key: relKey, serve: &feed.Route{Body: v101(), Cut: true}})
		}},
		{"b13-other-platform-only", "", false, func() {
			publish("1.0.1", 1, art{name: "app-1.0.1.exe", body: v101(), key: relKey, platform: "darwin", arch: "arm64"})
		}},
		{"b14-sig-without-algo", "", false, func() { publish("1.0.1", 1, art{name: "app-1.0.1.exe", body: v101(), key: relKey, dropAlgo: true}) }},
		{"b15-manifest-schema-2", "", false, func() { publish("1.0.1", 2, good("app-1.0.1.exe")) }},
		{"b16-prerelease-version", "", false, func() { publish("1.0.1-rc.1", 1, good("app-1.0.1.exe")) }},
		{"b17-sig-only-no-digest", "", false, func() { publish("1.0.1", 1, art{name: "app-1.0.1.exe", body: v101(), key: relKey, noDigest: true}) }},
		{"b18-wrong-size-field", "", false, func() { publish("1.0.1", 1, art{name: "app-1.0.1.exe", body: v101(), key: relKey, sizeLie: 1234}) }},
		{"b19-truncated-no-verification", "", false, func() {
			publish("1.0.1", 1, art{name: "app-1.0.1.exe", body: v101(), noDigest: true, serve: &feed.Route{Body: v101()[:len(v101())/2]}})
		}},
	}
	for _, v := range vs {
		v := v
		inst := v.inst
		if inst == "" {
			inst = "app-1.0.0.exe"
		}
		c := cfg("stage")
		c.LingerMs = 0
		c.RequireSignature = v.reqSig
		add(&Scn{Name: v.name, Group: "verify", Install: inst, Cfg: c, Feed: v.feed})
	}

	// ---- 3. Swap ----
	for i := 1; i <= reps; i++ {
		add(&Scn{Name: fmt.Sprintf("c1-downtime-same-r%d", i), Group: "downtime-same", Install: "app-1.0.0.exe", Cfg: cfg("automatic"),
			Feed: func() { publish("1.0.1", 1, good("app-1.0.1.exe")) }})
	}
	for i := 1; i <= reps; i++ {
		f := fmt.Sprintf("app-1.0.1-u%d.exe", i)
		add(&Scn{Name: fmt.Sprintf("c1-downtime-fresh-r%d", i), Group: "downtime-fresh", Install: "app-1.0.0.exe", Cfg: cfg("automatic"),
			Feed: func() { publish("1.0.1", 1, good(f)) }})
	}
	lockScn := func(name, mode string, hold int) *Scn {
		return &Scn{Name: name, Group: "lock", Install: "app-1.0.0.exe", Cfg: cfg("automatic"),
			Feed: func() { publish("1.0.1", 1, good("app-1.0.1.exe")) },
			OnLog: func(r *Run, e spike.Entry) {
				if e.S("ev") == "install-result" && e.S("result") == "staged" {
					startLocker(r, mode, hold)
				}
			}, MaxSettle: 45 * time.Second}
	}
	add(lockScn("c2-locked-20s-no-share-delete", "noshare", 20000))
	add(nextStart("c2b-next-start-after-lock"))
	add(lockScn("c3-locked-3s-no-share-delete", "noshare", 3000))
	add(lockScn("c4-locked-20s-share-delete", "sharedelete", 20000))
	add(nextStart("c4b-next-start"))

	kc := cfg("automatic")
	kc.ShutdownDelayMs = 3000
	add(&Scn{Name: "c5-kill-helper-while-waiting", Group: "kill", Install: "app-1.0.0.exe", Cfg: kc,
		Feed: func() { publish("1.0.1", 1, good("app-1.0.1.exe")) },
		OnLog: func(r *Run, e spike.Entry) {
			if e.S("ev") == "on-shutdown" && int(e.F("pid")) == r.OldPID {
				go func() { time.Sleep(300 * time.Millisecond); killHelper(r, "while parent in OnShutdown") }()
			}
		}})
	add(nextStart("c5b-next-start"))
	for _, d := range []int{0, 100, 250, 500, 1000, 2000} {
		d := d
		add(&Scn{Name: fmt.Sprintf("c6-kill-helper-pad-%04dms", d), Group: "kill", Install: "app-1.0.0-pad.exe", Cfg: cfg("automatic"),
			Feed: func() { publish("1.0.1", 1, good("app-1.0.1.exe")) },
			AfterExit: func(r *Run) {
				time.Sleep(time.Duration(d) * time.Millisecond)
				killHelper(r, fmt.Sprintf("%d ms after parent exit", d))
			}})
		add(nextStart(fmt.Sprintf("c6b-next-start-%04dms", d)))
	}
	hc := cfg("automatic")
	hc.HelperTimeoutMs = 1
	add(&Scn{Name: "c7-helper-not-ready", Group: "swap", Install: "app-1.0.0.exe", Cfg: hc,
		Feed: func() { publish("1.0.1", 1, good("app-1.0.1.exe")) }})
	sd := cfg("automatic")
	sd.ShutdownDelayMs = 35000
	add(&Scn{Name: "c9-shutdown-slower-than-30s", Group: "swap", Install: "app-1.0.0.exe", Cfg: sd,
		Feed: func() { publish("1.0.1", 1, good("app-1.0.1.exe")) }, MaxSettle: 20 * time.Second})
	add(nextStart("c9b-next-start"))
	add(&Scn{Name: "c8-chain-100-101-102", Group: "swap", Install: "app-1.0.0.exe", Cfg: cfg("automatic"),
		Feed: func() { publish("1.0.1", 1, good("app-1.0.1.exe")) },
		OnLog: func(r *Run, e spike.Entry) {
			if e.S("ev") == "main-entered" && e.S("ver") == "1.0.1" && e.S("helperEnv") == "" {
				publish("1.0.2", 1, good("app-1.0.2.exe"))
			}
		}})

	// ---- 4. Safe moment + catch-up ----
	sc := cfg("automatic")
	sc.BusyMs, sc.JobOffsetMs = 4000, 300
	add(&Scn{Name: "d1-busy-then-catch-up", Group: "safe", Install: "app-1.0.0.exe", Cfg: sc,
		Feed: func() { publish("1.0.1", 1, good("app-1.0.1.exe")) }})
	sc2 := cfg("automatic")
	sc2.JobOffsetMs, sc2.LingerMs = 6000, 7000
	add(&Scn{Name: "d2-job-after-restart", Group: "safe", Install: "app-1.0.0.exe", Cfg: sc2,
		Feed: func() { publish("1.0.1", 1, good("app-1.0.1.exe")) }})

	// ---- 5. Data ----
	add(&Scn{Name: "e1-old-app-on-new-db", Group: "data", Install: "app-1.0.0.exe", Cfg: cfg("off"), KeepData: "a1-endpoint-http"})
	add(&Scn{Name: "e2-create-v1-db", Group: "data", Install: "app-1.0.0.exe", Cfg: cfg("off")})
	fm := cfg("off")
	fm.FailMigration = true
	add(&Scn{Name: "e3-failed-migration", Group: "data", Install: "app-1.0.1.exe", Cfg: fm, KeepData: "e2-create-v1-db"})
	add(&Scn{Name: "e4-retry-migration", Group: "data", Install: "app-1.0.1.exe", Cfg: cfg("off"), KeepData: "e2-create-v1-db"})

	// ---- 6. Child processes ----
	for _, kind := range []string{"plain", "job"} {
		c := cfg("automatic")
		c.Child, c.ChildSeconds, c.ChildVersion = kind, 12, "1.0.0"
		add(&Scn{Name: "f-child-" + kind, Group: "child", Install: "app-1.0.0.exe", Cfg: c,
			Feed: func() { publish("1.0.1", 1, good("app-1.0.1.exe")) }, MaxSettle: 30 * time.Second})
	}

	// ---- 7. UI and privacy ----
	g1 := cfg("off")
	g1.LingerMs = 8000
	add(&Scn{Name: "g1-mode-off", Group: "privacy", Install: "app-1.0.0.exe", Cfg: g1, Feed: func() { publish("1.0.1", 1, good("app-1.0.1.exe")) }})
	g2 := cfg("notify")
	g2.LingerMs = 8000
	add(&Scn{Name: "g2-notify-only-interval-0", Group: "privacy", Install: "app-1.0.0.exe", Cfg: g2, Feed: func() { publish("1.0.1", 1, good("app-1.0.1.exe")) }})
	g3 := cfg("poll")
	g3.CheckIntervalMs, g3.LingerMs = 2000, 7000
	add(&Scn{Name: "g3-poll-every-2s", Group: "privacy", Install: "app-1.0.0.exe", Cfg: g3, Feed: func() { publish("1.0.1", 1, good("app-1.0.1.exe")) }})

	for _, s := range scns {
		if iter > 0 {
			c := *s
			c.Name = fmt.Sprintf("%s-i%d", s.Name, iter+1)
			s = &c
		}
		r := runScn(s)
		post(r)
	}
}

// nextStart starts whatever is in the install dir now (no reinstall), updates off.
func nextStart(name string) *Scn {
	c := cfg("off")
	c.LingerMs = 500
	return &Scn{Name: name, Group: "next", Cfg: c, MaxSettle: 20 * time.Second}
}

func startLocker(r *Run, mode string, hold int) {
	ready := filepath.Join(outDir, r.S.Name+".lock-ready")
	self, _ := os.Executable()
	cmd := exec.Command(self, "lock", mode, exePath, fmt.Sprint(hold), ready)
	t0 := time.Now()
	if err := cmd.Start(); err != nil {
		r.fact("lockErr", err.Error())
		return
	}
	r.track(cmd.Process.Pid, "locker", "")
	for i := 0; i < 200; i++ {
		if b, err := os.ReadFile(ready); err == nil {
			r.fact("lock", fmt.Sprintf("%s held %d ms, took after %s: %s", mode, hold, time.Since(t0).Round(time.Millisecond), string(b)))
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	go func() { _ = cmd.Wait(); r.fact("lockReleasedAt", time.Now().Format(time.RFC3339Nano)) }()
}

func killHelper(r *Run, when string) {
	var h *proc
	for i := 0; i < 100 && h == nil; i++ {
		h = r.helper()
		if h == nil {
			time.Sleep(10 * time.Millisecond)
		}
	}
	if h == nil {
		r.fact("killHelper", "no helper seen")
		return
	}
	ok := kill(h)
	snap := snapshotInstall()
	r.fact("killHelper", fmt.Sprintf("%s: terminated=%v at %s; install dir right after: %s", when, ok, time.Now().Format("15:04:05.000"), snap))
}

func snapshotInstall() string {
	es, _ := os.ReadDir(installDir)
	var parts []string
	for _, e := range es {
		fi, _ := e.Info()
		if fi != nil {
			parts = append(parts, fmt.Sprintf("%s %s", e.Name(), human(fi.Size())))
		}
	}
	return strings.Join(parts, "; ")
}
