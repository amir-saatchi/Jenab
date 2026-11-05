package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The test binary stands in for agent-desktop when fakeVar is set: its
// first argument says what to answer.
const fakeVar = "JENAB_TEST_FAKE_AGENT_DESKTOP"

func TestMain(m *testing.M) {
	if os.Getenv(fakeVar) != "" {
		fakeMain(os.Args[1:])
		return
	}
	os.Exit(m.Run())
}

func fakeMain(args []string) {
	out := func(ok bool, data, err string) {
		fmt.Printf(`{"version":"2.5","ok":%v,"command":"x","data":%s,"error":%s}`, ok, data, err)
	}
	switch args[0] {
	case "version":
		out(true, `{"version":"1.0.2"}`, "null")
	case "env":
		b, _ := json.Marshal(map[string]string{"home": os.Getenv("AGENT_DESKTOP_HOME"), "session": os.Getenv("AGENT_DESKTOP_SESSION")})
		out(true, string(b), "null")
	case "fail":
		out(false, "null", `{"code":"STALE_REF","message":"gone","suggestion":"snapshot again","disposition":{"delivery":"not_delivered","retry":"safe"}}`)
		os.Exit(1)
	case "old":
		fmt.Print(`{"version":"2.4","ok":true,"data":{}}`)
	case "sleep":
		time.Sleep(time.Minute)
	case "crash":
		fmt.Fprint(os.Stderr, "boom")
		os.Exit(3)
	}
	os.Exit(0)
}

// fakeCLI is the test binary as the pinned program, with its own hash.
func fakeCLI(t *testing.T) *CLI {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(exe)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	err = checkHash(f, "")
	_, got, _ := strings.Cut(err.Error(), "its SHA-256 is ")
	got, _, _ = strings.Cut(got, ",")
	t.Setenv(fakeVar, "1")
	return &CLI{Path: exe, Home: filepath.Join(t.TempDir(), "agent-desktop"), Hash: got}
}

func TestCLIEnvelope(t *testing.T) {
	c := fakeCLI(t)
	data, err := c.Run(t.Context(), callTimeout, "version")
	if err != nil || !strings.Contains(string(data), `"1.0.2"`) {
		t.Fatalf("version: %s, %v", data, err)
	}
	_, err = c.Run(t.Context(), callTimeout, "fail")
	var de *Error
	if !errors.As(err, &de) || de.Code != CodeStaleRef || de.Delivery != "not_delivered" || de.Retry != "safe" {
		t.Fatalf("fail: %#v", err)
	}
	if _, err = c.Run(t.Context(), callTimeout, "old"); err == nil || !strings.Contains(err.Error(), `version "2.4"`) {
		t.Fatalf("an envelope of another version: %v", err)
	}
	if _, err = c.Run(t.Context(), callTimeout, "crash"); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("crash: %v", err)
	}
}

func TestCLIEnvironment(t *testing.T) {
	c := fakeCLI(t)
	t.Setenv("AGENT_DESKTOP_SESSION", "from-outside")
	data, err := c.Run(t.Context(), callTimeout, "env")
	if err != nil {
		t.Fatal(err)
	}
	var env struct{ Home, Session string }
	_ = json.Unmarshal(data, &env)
	if env.Home != c.Home || env.Session != "" {
		t.Fatalf("env: %+v", env)
	}
}

func TestCLIHash(t *testing.T) {
	c := fakeCLI(t)
	c.Hash = SHA256 // the test binary isn't agent-desktop
	if _, err := c.Run(t.Context(), callTimeout, "version"); !errors.Is(err, ErrProgram) {
		t.Fatalf("a wrong hash ran: %v", err)
	}
	c.Path = filepath.Join(t.TempDir(), Program)
	if _, err := c.Run(t.Context(), callTimeout, "version"); !errors.Is(err, ErrProgram) {
		t.Fatalf("a missing program: %v", err)
	}
}

func TestCLIKill(t *testing.T) {
	c := fakeCLI(t)
	start := time.Now()
	if _, err := c.Run(t.Context(), 300*time.Millisecond, "sleep"); !errors.Is(err, ErrKilled) {
		t.Fatalf("sleep: %v", err)
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Fatalf("killed after %s", d)
	}
	ctx, cancel := context.WithCancelCause(t.Context())
	time.AfterFunc(200*time.Millisecond, func() { cancel(errors.New("stopped by the user")) })
	if _, err := c.Run(ctx, callTimeout, "sleep"); err == nil || err.Error() != "stopped by the user" {
		t.Fatalf("a cancelled call: %v", err)
	}
}

func TestStateFolderMustBePrivate(t *testing.T) {
	c := fakeCLI(t)
	if _, err := c.Run(t.Context(), callTimeout, "version"); err != nil {
		t.Fatal(err)
	}
	if err := prepareHome(c.Home); err != nil {
		t.Fatalf("the folder it made: %v", err)
	}
	open := filepath.Join(t.TempDir(), "open")
	if err := os.Mkdir(open, 0o700); err != nil { // inherits its parent's ACL
		t.Fatal(err)
	}
	c.Home = open
	if _, err := c.Run(t.Context(), callTimeout, "version"); !errors.Is(err, ErrHome) {
		t.Fatalf("an inherited ACL: %v", err)
	}
}
