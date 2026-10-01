package main

// Part 4: does time.LoadLocation work in a built exe without GOROOT?
// Builds a tiny probe in a temp dir, with and without `time/tzdata`, runs it
// with GOROOT and ZONEINFO removed from the environment, then deletes it.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const probeSrc = `package main

import (
	"fmt"
	"time"
	%s
)

func main() {
	for _, z := range []string{"Europe/Berlin", "America/New_York", "Asia/Tehran", "UTC"} {
		loc, err := time.LoadLocation(z)
		if err != nil {
			fmt.Printf("%%s: error: %%v\n", z, err)
			continue
		}
		t := time.Date(2026, 7, 1, 12, 0, 0, 0, loc)
		_, off := t.Zone()
		fmt.Printf("%%s: ok %%+d\n", z, off/60)
	}
}
`

type probeVariant struct {
	name, imp string
	flags     []string
	env       []string // extra env for the run
}

func part4() string {
	var b strings.Builder
	b.WriteString("## 4. Time zones in a built Windows exe\n\n")
	dir, err := os.MkdirTemp("", "spike014-tz-")
	if err != nil {
		return b.String() + "temp dir: " + err.Error() + "\n\n"
	}
	defer os.RemoveAll(dir)

	variants := []probeVariant{
		{"plain build", "", nil, nil},
		{"plain build, GOROOT=C:\\nonexistent", "", nil, []string{`GOROOT=C:\nonexistent`}},
		{"-trimpath", "", []string{"-trimpath"}, nil},
		{"-trimpath + time/tzdata", `_ "time/tzdata"`, []string{"-trimpath"}, nil},
		{"-trimpath -ldflags=-s -w", "", []string{"-trimpath", "-ldflags=-s -w"}, nil},
		{"-trimpath -ldflags=-s -w + time/tzdata", `_ "time/tzdata"`, []string{"-trimpath", "-ldflags=-s -w"}, nil},
	}
	b.WriteString("Probe loads 4 zones. Each exe runs with `GOROOT` and `ZONEINFO` removed from its environment (a user machine without Go). A plain build still has the build machine's GOROOT baked in (`runtime.GOROOT()`), so it finds `lib/time/zoneinfo.zip` here but would not on a user machine; the second row simulates that.\n\n")
	b.WriteString("| Build | exe size | Europe/Berlin | America/New_York | Asia/Tehran | UTC |\n|---|---|---|---|---|---|\n")
	sizes := map[string]int64{}
	for i, v := range variants {
		sub := filepath.Join(dir, fmt.Sprint(i))
		os.MkdirAll(sub, 0o755)
		os.WriteFile(filepath.Join(sub, "go.mod"), []byte("module probe\n\ngo 1.26.0\n"), 0o644)
		os.WriteFile(filepath.Join(sub, "main.go"), []byte(fmt.Sprintf(probeSrc, v.imp)), 0o644)
		exe := filepath.Join(sub, "probe.exe")
		args := append([]string{"build"}, v.flags...)
		args = append(args, "-o", exe, ".")
		cmd := exec.Command("go", args...)
		cmd.Dir = sub
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
		if out, err := cmd.CombinedOutput(); err != nil {
			fmt.Fprintf(&b, "| %s | build failed: %s | | | | |\n", v.name, clip(string(out), 80))
			continue
		}
		st, _ := os.Stat(exe)
		sizes[v.name] = st.Size()
		run := exec.Command(exe)
		run.Env = append(cleanEnv(), v.env...)
		out, _ := run.CombinedOutput()
		res := map[string]string{}
		for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			z, r, _ := strings.Cut(strings.TrimSpace(l), ": ")
			res[z] = clip(r, 70)
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s |\n", v.name, kb(st.Size()), res["Europe/Berlin"], res["America/New_York"], res["Asia/Tehran"], res["UTC"])
	}
	fmt.Fprintf(&b, "\ntime/tzdata adds %s to a -trimpath build and %s to a stripped build.\n\n",
		kb(sizes["-trimpath + time/tzdata"]-sizes["-trimpath"]), kb(sizes["-trimpath -ldflags=-s -w + time/tzdata"]-sizes["-trimpath -ldflags=-s -w"]))
	return b.String()
}

func cleanEnv() []string {
	var env []string
	for _, e := range os.Environ() {
		k, _, _ := strings.Cut(e, "=")
		if strings.EqualFold(k, "GOROOT") || strings.EqualFold(k, "ZONEINFO") {
			continue
		}
		env = append(env, e)
	}
	return env
}

func kb(n int64) string {
	return fmt.Sprintf("%d KB", n/1024)
}
