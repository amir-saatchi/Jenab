// Command headlessbrowser runs SPIKE-047's Part A: agent-browser as a
// headless renderer for fetch_page, behind a proxy that applies SPEC 6.7.
//
//	go build -o bin/spike047.exe . && bin/spike047.exe -part run|net|text|safety
//
// Paths come from flags; see README.md.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var (
	flagAB       = flag.String("ab", "", "agent-browser-win32-x64.exe")
	flagHash     = flag.String("hash", "ebcef5ebea93c7ccf0fe3f8c18eee908ae9174dfef8e52a3dda6f24a59f630dc", "its pinned SHA-256")
	flagCfT      = flag.String("cft", "", "Chrome for Testing chrome.exe")
	flagShell    = flag.String("shell", "", "chrome-headless-shell.exe")
	flagChrome   = flag.String("chrome", `C:\Program Files\Google\Chrome\Application\chrome.exe`, "the installed chrome.exe (fresh profile only)")
	flagEdge     = flag.String("edge", `C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`, "msedge.exe")
	flagHome     = flag.String("home", "", "base folder for private state folders")
	flagPart     = flag.String("part", "run", "run, net, text, live, extract, safety, policy, edgeflags, fakejenab")
	flagOnly     = flag.String("browsers", "cft,shell,chrome", "browsers for net and text")
	flagLive     = flag.String("livepages", "", "for -part live: id=url,… (SPIKE-015 corpus pages)")
	flagVariants = flag.String("variants", "", "for -part net: only these variants (e.g. V5,V7), and no navigation tests")
)

type browser struct{ ID, Path string }

func browsers() []browser {
	all := map[string]string{"cft": *flagCfT, "shell": *flagShell, "chrome": *flagChrome, "edge": *flagEdge}
	var out []browser
	for _, id := range strings.Split(*flagOnly, ",") {
		if p := all[id]; p != "" {
			out = append(out, browser{id, p})
		}
	}
	return out
}

const root = "agent-browser-win32-x64.exe"

var out io.Writer = os.Stdout

func p(format string, a ...any) { fmt.Fprintf(out, format+"\n", a...) }

// env is the shared test setup: canaries, the site and the proxy.
type env struct {
	C     *Canaries
	S     *Site
	P     *Proxy
	PAddr string
	n     int
}

func newEnv() (*env, error) {
	e := &env{C: &Canaries{}}
	if err := e.C.Start(); err != nil {
		return nil, err
	}
	e.S = &Site{C: e.C, Nonce: fmt.Sprintf("%x", time.Now().UnixNano()&0xffffffff)}
	if err := e.S.Start(); err != nil {
		return nil, err
	}
	loop := netip.MustParseAddr("127.0.0.1")
	e.P = &Proxy{
		Exceptions: map[string]bool{siteName: true, "approved2.spike047.test": true},
		Fake:       map[string]netip.Addr{siteName: loop, "approved2.spike047.test": loop},
	}
	a, err := e.P.Start()
	if err != nil {
		return nil, err
	}
	e.PAddr = a
	return e, nil
}

// ab returns a setup with a fresh private state folder.
func (e *env) ab(b browser, name string) *AB {
	e.n++
	return &AB{
		Path: *flagAB, Hash: *flagHash, Browser: b.Path,
		Home:  filepath.Join(*flagHome, fmt.Sprintf("%s-%s-%d", b.ID, name, e.n)),
		Proxy: "http://" + e.PAddr, Bypass: true, Idle: "60s",
	}
}

func main() {
	flag.Parse()
	if *flagPart == "fakejenab" {
		fakeJenab()
		return
	}
	if *flagAB == "" || *flagHome == "" {
		fmt.Fprintln(os.Stderr, "need -ab and -home")
		os.Exit(2)
	}
	if err := prepareHome(*flagHome); err != nil {
		fmt.Fprintln(os.Stderr, "home:", err)
		os.Exit(1)
	}
	before := homeListing()
	ctx := context.Background()
	switch *flagPart {
	case "run":
		partRun(ctx)
	case "net":
		partNet(ctx)
	case "text":
		partText(ctx)
	case "policy":
		partPolicy(ctx)
	case "extract":
		partExtract(ctx)
	case "live":
		partLive(ctx)
	case "safety":
		partSafety(ctx)
	case "edgeflags":
		partEdgeFlags()
	default:
		fmt.Fprintln(os.Stderr, "unknown part")
		os.Exit(2)
	}
	p("\n### Files outside the state folders\n")
	after := homeListing()
	var added []string
	for k := range after {
		if !before[k] {
			added = append(added, k)
		}
	}
	sort.Strings(added)
	if len(added) == 0 {
		p("Nothing new in the home folder, %%LOCALAPPDATA%%, %%APPDATA%% or %%TEMP%% (top level), and no `~/.agent-browser`.")
	} else {
		p("New entries: %s", strings.Join(added, ", "))
	}
	if left := Left(root); len(left) > 0 {
		p("\n**Processes left at the end:** %s", strings.Join(left, ", "))
	} else {
		p("\nNo agent-browser or browser process left at the end.")
	}
}

// homeListing is the top level of the user's folders, to catch files
// written outside the state folder. User names stay out of the output.
func homeListing() map[string]bool {
	out := map[string]bool{}
	for label, dir := range map[string]string{"~": os.Getenv("USERPROFILE"), "%LOCALAPPDATA%": os.Getenv("LOCALAPPDATA"),
		"%APPDATA%": os.Getenv("APPDATA"), "%TEMP%": os.Getenv("TEMP")} {
		es, _ := os.ReadDir(dir)
		for _, e := range es {
			out[label+`\`+e.Name()] = true
		}
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("USERPROFILE"), ".agent-browser")); err == nil {
		out[`~\.agent-browser (exists)`] = true
	}
	return out
}

func waitGone(d time.Duration) (time.Duration, []string) {
	t0 := time.Now()
	for time.Since(t0) < d {
		if len(Left(root)) == 0 {
			return time.Since(t0), nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return time.Since(t0), Left(root)
}

func mb(b uint64) string { return fmt.Sprintf("%d MB", b>>20) }

func dirSize(dir string) int64 {
	var n int64
	filepath.Walk(dir, func(_ string, fi os.FileInfo, err error) error {
		if err == nil && !fi.IsDir() {
			n += fi.Size()
		}
		return nil
	})
	return n
}
