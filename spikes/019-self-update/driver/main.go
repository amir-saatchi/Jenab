// SPIKE-019 driver: keys, builds, local feed, per-user install, scenarios,
// JSONL logs and a summary. Run from the spike folder: go run ./driver
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"golang.org/x/sys/windows"

	"burrow/spikes/selfupdate/internal/feed"
)

var (
	root, outDir, binDir, keysDir string
	installDir, exePath         string
	fd                          *feed.Server
	pubB64                      string
	relKey, otherKey            ed25519.PrivateKey
	tempBefore                  map[string]bool
	onlyRe                      *regexp.Regexp
	reps                        int
	iter                        int
)

const exeName = "BurrowSpike019.exe"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "lock" {
		lockMain(os.Args[2:])
		return
	}
	only := flag.String("only", "", "regexp: run only matching scenarios")
	flag.IntVar(&reps, "reps", 6, "downtime repetitions per variant")
	noBuild := flag.Bool("nobuild", false, "reuse bin/")
	outName := flag.String("out", "out", "output folder (wiped at start)")
	repeat := flag.Int("repeat", 1, "run the selected scenarios this many times")
	flag.Parse()
	if *only != "" {
		onlyRe = regexp.MustCompile(*only)
	}
	var err error
	root, err = os.Getwd()
	must(err)
	if _, err := os.Stat(filepath.Join(root, "app", "main.go")); err != nil {
		fmt.Println("run from the spike folder (spikes/019-self-update)")
		os.Exit(2)
	}
	outDir = filepath.Join(root, *outName)
	binDir = filepath.Join(root, "bin")
	keysDir = filepath.Join(root, "keys")
	installDir = filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "BurrowSpike019")
	exePath = filepath.Join(installDir, exeName)
	_ = os.RemoveAll(outDir)
	for _, d := range []string{outDir, binDir, keysDir, filepath.Join(outDir, "logs"), filepath.Join(outDir, "cfg"), filepath.Join(outDir, "data")} {
		must(os.MkdirAll(d, 0o755))
	}
	tempBefore = tempEntries()
	logf("driver start %s, install dir %s, temp %s", time.Now().Format(time.RFC3339), installDir, os.TempDir())

	keys()
	if !*noBuild {
		builds()
	}
	fd, err = feed.Start(filepath.Join(outDir, "feed.jsonl"))
	must(err)
	must(os.WriteFile(filepath.Join(outDir, "feed-ca.pem"), fd.CertPEM, 0o644))
	logf("feed on %s and %s (127.0.0.1 only)", fd.HTTPURL, fd.HTTPSURL)

	defer cleanup()
	for iter = 0; iter < *repeat; iter++ {
		runAll()
	}
	writeSummary()
}

func keys() {
	relKey = loadOrMakeKey("release-ed25519")
	otherKey = loadOrMakeKey("attacker-ed25519")
	pubB64 = base64.StdEncoding.EncodeToString(relKey.Public().(ed25519.PublicKey))
	must(os.WriteFile(filepath.Join(keysDir, "release-ed25519.pub"), []byte(pubB64+"\n"), 0o644))
}

// loadOrMakeKey keeps a test key pair in keys/ (the .key files are gitignored).
func loadOrMakeKey(name string) ed25519.PrivateKey {
	p := filepath.Join(keysDir, name+".key")
	if b, err := os.ReadFile(p); err == nil {
		seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
		if err == nil && len(seed) == ed25519.SeedSize {
			return ed25519.NewKeyFromSeed(seed)
		}
	}
	_, k, err := ed25519.GenerateKey(rand.Reader)
	must(err)
	must(os.WriteFile(p, []byte(base64.StdEncoding.EncodeToString(k.Seed())+"\n"), 0o600))
	return k
}

type buildSpec struct {
	out, ver, key, nonce string
}

func builds() {
	specs := []buildSpec{
		{"app-1.0.0.exe", "1.0.0", pubB64, ""},
		{"app-1.0.1.exe", "1.0.1", pubB64, ""},
		{"app-1.0.2.exe", "1.0.2", pubB64, ""},
		{"app-1.0.0-nokey.exe", "1.0.0", "", ""},
	}
	for i := 1; i <= reps; i++ {
		specs = append(specs, buildSpec{fmt.Sprintf("app-1.0.1-u%d.exe", i), "1.0.1", pubB64, fmt.Sprintf("u%d-%d", i, time.Now().UnixNano())})
	}
	for _, s := range specs {
		t0 := time.Now()
		ld := fmt.Sprintf("-H=windowsgui -s -w -X main.version=%s -X main.pubKeyB64=%s -X main.nonce=%s", s.ver, s.key, s.nonce)
		cmd := exec.Command("go", "build", "-trimpath", "-tags", "production", "-ldflags", ld, "-o", filepath.Join(binDir, s.out), "./app")
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
		b, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Println(string(b))
			must(err)
		}
		logf("built %s (%s) in %s", s.out, human(size(filepath.Join(binDir, s.out))), time.Since(t0).Round(time.Millisecond))
	}
	// Padded old version: 1.0.0 with 256 MiB of random data appended (the PE
	// loader ignores trailing data). Makes the helper's backup copy slow enough
	// to kill the helper in the middle of it.
	src, err := os.ReadFile(filepath.Join(binDir, "app-1.0.0.exe"))
	must(err)
	f, err := os.Create(filepath.Join(binDir, "app-1.0.0-pad.exe"))
	must(err)
	_, _ = f.Write(src)
	_, err = io.CopyN(f, rand.Reader, 256<<20)
	must(err)
	must(f.Close())
	logf("made app-1.0.0-pad.exe (%s)", human(size(filepath.Join(binDir, "app-1.0.0-pad.exe"))))
}

func cleanup() {
	// Processes: only the ones we recorded (by handle), never by image name.
	killAllKnown("final cleanup")
	// Temp: only wails-update-* entries created during this run.
	var removed []string
	for name := range tempEntries() {
		if !tempBefore[name] {
			if os.RemoveAll(filepath.Join(os.TempDir(), name)) == nil {
				removed = append(removed, name)
			}
		}
	}
	sort.Strings(removed)
	logf("removed %d temp entries created by this run", len(removed))
	if fd != nil {
		fd.Close()
	}
	err := os.RemoveAll(installDir)
	_, statErr := os.Stat(installDir)
	logf("install dir removed: err=%v, still exists=%v", err, statErr == nil)
}

func tempEntries() map[string]bool {
	m := map[string]bool{}
	es, _ := os.ReadDir(os.TempDir())
	for _, e := range es {
		if strings.HasPrefix(e.Name(), "wails-update-") {
			m[e.Name()] = true
		}
	}
	return m
}

// lockMain: driver lock <noshare|sharedelete> <path> <holdMs> <readyFile>
func lockMain(a []string) {
	mode, path, readyFile := a[0], a[1], a[3]
	var hold time.Duration
	fmt.Sscanf(a[2], "%d", &hold)
	share := uint32(windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE)
	if mode == "sharedelete" {
		share |= windows.FILE_SHARE_DELETE
	}
	p, _ := windows.UTF16PtrFromString(path)
	h, err := windows.CreateFile(p, windows.GENERIC_READ, share, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		_ = os.WriteFile(readyFile, []byte("err: "+err.Error()), 0o644)
		os.Exit(1)
	}
	_ = os.WriteFile(readyFile, []byte("ok"), 0o644)
	time.Sleep(hold * time.Millisecond)
	windows.CloseHandle(h)
}

func sha(b []byte) []byte { s := sha256.Sum256(b); return s[:] }

func fileHash(p string) string {
	f, err := os.Open(p)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	_, _ = io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil))
}

func size(p string) int64 {
	fi, err := os.Stat(p)
	if err != nil {
		return -1
	}
	return fi.Size()
}

func human(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}

func logf(f string, a ...any) {
	fmt.Printf("%s  %s\n", time.Now().Format("15:04:05.000"), fmt.Sprintf(f, a...))
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
