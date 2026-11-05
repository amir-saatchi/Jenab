package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
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
		fmt.Printf(`{"version":"2.6","ok":%v,"command":"x","data":%s,"error":%s}`, ok, data, err)
	}
	switch args[0] {
	case "version":
		out(true, `{"version":"1.2.0"}`, "null")
	case "env":
		b, _ := json.Marshal(map[string]string{"home": os.Getenv("AGENT_DESKTOP_HOME"), "session": os.Getenv("AGENT_DESKTOP_SESSION")})
		out(true, string(b), "null")
	case "stdin":
		// What a --stdin command gets: its arguments and its input.
		in, err := io.ReadAll(os.Stdin)
		b, _ := json.Marshal(map[string]any{"args": args, "input": string(in), "err": fmt.Sprint(err)})
		out(true, string(b), "null")
	case "ambiguous":
		out(false, "null", `{"code":"AMBIGUOUS_TARGET","message":"not proven unique","details":{"kind":"identity_not_unique","reason":"x"},"disposition":{"delivery":"not_delivered","retry":"safe"}}`)
		os.Exit(1)
	case "odd-details":
		out(false, "null", `{"code":"ACTION_FAILED","message":"x","details":["not","an","object"],"disposition":{"delivery":"unknown","retry":"unsafe"}}`)
		os.Exit(1)
	case "fail":
		out(false, "null", `{"code":"STALE_REF","message":"gone","suggestion":"snapshot again","disposition":{"delivery":"not_delivered","retry":"safe"}}`)
		os.Exit(1)
	case "old":
		fmt.Print(`{"version":"2.5","ok":true,"data":{}}`)
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
	if err != nil || !strings.Contains(string(data), `"1.2.0"`) {
		t.Fatalf("version: %s, %v", data, err)
	}
	_, err = c.Run(t.Context(), callTimeout, "fail")
	var de *Error
	if !errors.As(err, &de) || de.Code != CodeStaleRef || de.Delivery != "not_delivered" || de.Retry != "safe" {
		t.Fatalf("fail: %#v", err)
	}
	_, err = c.Run(t.Context(), callTimeout, "ambiguous")
	if !errors.As(err, &de) || de.Code != CodeAmbiguous || de.Kind != "identity_not_unique" || de.Delivery != "not_delivered" {
		t.Fatalf("ambiguous: %#v", err)
	}
	_, err = c.Run(t.Context(), callTimeout, "odd-details")
	if !errors.As(err, &de) || de.Code != "ACTION_FAILED" || de.Kind != "" || de.Retry != "unsafe" {
		t.Fatalf("details that aren't an object: %#v", err)
	}
	if _, err = c.Run(t.Context(), callTimeout, "old"); err == nil || !strings.Contains(err.Error(), `version "2.5"`) {
		t.Fatalf("an envelope of another version: %v", err)
	}
	if _, err = c.Run(t.Context(), callTimeout, "crash"); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("crash: %v", err)
	}
}

// RunInput gives the text on standard input, through a pipe, and nothing
// of it in the arguments (TASK-039); Run gives an empty input.
func TestCLIStdin(t *testing.T) {
	c := fakeCLI(t)
	const text = "Meet me at the north gate\r\nسلام\n"
	data, err := c.RunInput(t.Context(), callTimeout, text, "stdin", "set-value", "@s1:e1", "--stdin")
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Args  []string
		Input string
		Err   string
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Input != text || got.Err != "<nil>" || !slices.Equal(got.Args, []string{"stdin", "set-value", "@s1:e1", "--stdin"}) {
		t.Fatalf("got %+v", got)
	}
	if data, err = c.Run(t.Context(), callTimeout, "stdin"); err != nil {
		t.Fatal(err)
	}
	got.Input = "x"
	if err := json.Unmarshal(data, &got); err != nil || got.Input != "" {
		t.Fatalf("Run's input: %+v %v", got, err)
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
