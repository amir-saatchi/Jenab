// Command jenab-bench measures the app (P1-18, SPIKE-048 Part B):
// start-up, idle memory, opening a chat of 200 messages, streaming at 100
// tokens a second while the user types, and memory with ten answers
// streaming at once. It starts a bench build of the app (cmd/desktop built
// with -tags bench) with a new user folder and a local fake model, drives
// it through the build's loopback port, reads the times from its log, and
// writes a report. Windows only for now.
//
//	go build -tags bench -o bin/jenab-bench-app.exe ./cmd/desktop
//	go run ./cmd/jenab-bench -app bin/jenab-bench-app.exe
//
// Keep the app's window visible and leave the machine alone during a run:
// a hidden window gets fewer frames.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const mb = 1024 * 1024

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "jenab-bench:", err)
		os.Exit(1)
	}
}

type options struct {
	app     string
	out     string
	starts  int
	rate    float64
	opens   int
	history string
}

func run() (err error) {
	var o options
	flag.StringVar(&o.app, "app", filepath.Join("bin", "jenab-bench-app.exe"), "the app: cmd/desktop built with -tags bench")
	flag.StringVar(&o.out, "out", filepath.Join("bin", "bench"), "where the report goes")
	flag.IntVar(&o.starts, "starts", 3, "app starts for the start-up time")
	flag.Float64Var(&o.rate, "rate", 100, "tokens a second the fake model streams")
	flag.IntVar(&o.opens, "opens", 5, "times the 200-message chat is opened")
	keep := flag.Bool("keep", false, "keep the app's user folder")
	temp := flag.String("temp", "", "where the app's user folder goes; default: the system's temp folder")
	tokens := flag.Int("tokens", 2000, "the fake model's answer, in tokens")
	withSamples := flag.Bool("samples", false, "picture the 20 text samples instead of measuring")
	hold := flag.Duration("hold", 0, "open the app on -hold-chat and keep it up this long, for checks by hand or UI Automation")
	holdChat := flag.String("hold-chat", "History 200", "the chat -hold opens")
	devTools := flag.Bool("dev-tools", false, "with -hold: turn the developer tools on; a message with \"use a tool\" in it gets two tool calls first")
	gemini := flag.String("gemini", "", "with -hold: also connect Gemini with this model and make it the default, with the key in GEMINI_API_KEY, kept in the app's memory")
	flag.StringVar(&o.history, "history", "mixed", "what the 200-message chat's answers hold: "+strings.Join(historyKinds, ", "))
	flag.Parse()
	if !slices.Contains(historyKinds, o.history) {
		return fmt.Errorf("-history must be one of %s", strings.Join(historyKinds, ", "))
	}
	if _, err := os.Stat(o.app); err != nil {
		return fmt.Errorf("the app: %w (go build -tags bench -o %s ./cmd/desktop builds it)", err, o.app)
	}
	ctx := context.Background()
	dir, err := os.MkdirTemp(*temp, "jenab-bench-")
	if err != nil {
		return err
	}
	if dir, err = filepath.Abs(dir); err != nil {
		return err
	}
	if !*keep {
		defer os.RemoveAll(dir)
	}
	h := home{dir: dir}
	for _, d := range []string{"roaming", "temp", filepath.Join("user", "AppData", "Local"), filepath.Join("user", "AppData", "Roaming")} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
			return err
		}
	}
	model, err := startModel(reply(1, *tokens), o.rate)
	if err != nil {
		return err
	}
	defer model.close()
	if *gemini != "" && (*hold == 0 || os.Getenv("GEMINI_API_KEY") == "") {
		return errors.New("-gemini needs -hold and GEMINI_API_KEY")
	}
	h.gemini = *gemini
	h.devTools = *devTools && *hold > 0
	if err := h.writeSettings(model.url); err != nil {
		return err
	}
	if err := h.seed(ctx, o.history, *withSamples); err != nil {
		return fmt.Errorf("seed: %w", err)
	}
	if err := os.MkdirAll(o.out, 0o755); err != nil {
		return err
	}
	switch {
	case *hold > 0:
		return holdNative(o, o.app, h, *holdChat, *hold)
	case *withSamples:
		return runSamples(o, o.app, h)
	default:
		return runNative(o, o.app, h, model)
	}
}

func freePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

// memory is the app's process tree: the app and the processes it started,
// such as query workers.
type memory struct {
	PrivateMB    float64 `json:"private_mb"`
	WorkingSetMB float64 `json:"working_set_mb"`
	AppPrivateMB float64 `json:"app_private_mb"`
	Processes    int     `json:"processes"`
}

type loadResult struct {
	Streams int    `json:"streams"` // answers streaming at once, at the peak
	Samples int    `json:"samples"`
	Peak    memory `json:"peak"`
	After   memory `json:"after"`
}
