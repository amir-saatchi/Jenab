package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// replay runs a saved macro without a model and grades the result: does a macro the model
// saved replay at all, and with other params? It uses the same Session, guards and replayer.

type replayFlags struct {
	agent agentFlags
	macro string
}

func (r *replayFlags) register(fs *flag.FlagSet) {
	r.agent.register(fs)
	fs.StringVar(&r.macro, "macro", "", "macro name (default: the app's only macro)")
}

func runReplayCmd(cfg *Config, rf replayFlags) error {
	f := rf.agent
	if f.store == "" {
		f.store = filepath.Join(cfg.Out, "store-"+f.model+".json")
	}
	store, err := openStore(f.store)
	if err != nil {
		return err
	}
	sets, err := parseSets(f.params)
	if err != nil {
		return err
	}
	dir := filepath.Join(cfg.Out, "replay", time.Now().Format("20060102-150405")+"-"+f.model)
	os.MkdirAll(dir, 0o755)
	var all []map[string]any
	for _, name := range strings.Split(f.tasks, ",") {
		t := tasks[name]
		for _, set := range sets {
			p := t.Params[set]
			app, err := t.Launch(cfg, f)
			if err != nil {
				return err
			}
			time.Sleep(1500 * time.Millisecond)
			s := &Session{cli: cfg.CLI, Win: app.Win, ids: map[string]string{}, nodes: map[string]SnapNode{}, counts: map[string]int{},
				owner: windowPID(hwndOf(app.Win.ID)), store: store, mode: "macros", model: f.model}
			if t.Browser {
				s.br = &BrowserNotes{}
				s.refPath = filepath.Join(dir, fmt.Sprintf("%s-%d-reference.json", name, set))
			}
			var m *Macro
			for _, mm := range store.Macros {
				if mm.App == app.Win.AppName && (rf.macro == "" || mm.Name == rf.macro) {
					m = mm
				}
			}
			if m == nil {
				app.Close()
				fmt.Printf("%-10s set %d: no macro for %s\n", name, set, app.Win.AppName)
				continue
			}
			// The model named its params itself: give the task's values in the task's order.
			mp := map[string]string{}
			for i, k := range m.Params {
				if v, ok := p[k]; ok {
					mp[k] = v
				} else if i < len(t.ParamOrder) {
					mp[k] = p[t.ParamOrder[i]]
				}
			}
			out, rec := s.replay(m, mp, false)
			// The replay's log stands in for the model's answer.
			ok, check := t.Check(s, p, strings.Join(rec.Log, "\n"))
			if name == "calculator" {
				a, _ := strconv.Atoi(p["a"])
				b, _ := strconv.Atoi(p["b"])
				ok = strings.Contains(strings.ReplaceAll(check, ",", ""), "display: Display is "+strconv.Itoa(a+b))
			}
			if s.br != nil && s.br.Captcha {
				noteCaptcha(cfg.Out, "replay "+m.Name+" "+s.br.CaptchaWhy)
				fmt.Println("   CAPTCHA:", s.br.CaptchaWhy)
			}
			app.Close()
			store.save()
			fmt.Printf("%-10s set %d macro %s v%d: replay ok=%v failed_at=%d skipped=%d %.0fs; result ok=%v (%s)\n", name, set, m.Name, m.Version, rec.OK, rec.FailedAt, rec.Skipped, rec.Seconds, ok, check)
			os.WriteFile(filepath.Join(dir, fmt.Sprintf("%s-%d.txt", name, set)), []byte(out), 0o644)
			all = append(all, map[string]any{"task": name, "set": set, "replay": rec, "result_ok": ok, "check": check})
			writeJSON(filepath.Join(dir, "replays.json"), all)
		}
	}
	return nil
}

// store list | show <app> <name> | undo <kind> <app> <name>
func runStoreCmd(cfg *Config, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: store <file> list | show <app> <name> | undo macro|recipe <app> <name>")
	}
	st, err := openStore(args[0])
	if err != nil {
		return err
	}
	switch args[1] {
	case "list":
		for _, m := range st.Macros {
			fmt.Printf("%s/%s v%d (%s): %s\n", m.App, m.Name, m.Version, m.MadeBy, describeMacro(m, time.Now()))
			for i, s := range m.Steps {
				fmt.Printf("   %d %s\n", i+1, s)
			}
		}
		for _, r := range st.Recipes {
			fmt.Printf("%s/recipe %s (%s): %s\n", r.App, r.Name, r.MadeBy, r.Text)
		}
		return nil
	case "undo":
		if len(args) < 5 {
			return fmt.Errorf("usage: store <file> undo macro|recipe <app> <name>")
		}
		if err := st.undo(args[2], args[3], args[4]); err != nil {
			return err
		}
		return st.save()
	}
	return fmt.Errorf("unknown store command %q", args[1])
}

// setCalcMode switches Calculator through its navigation menu (no model): "Scientific",
// "Standard", "Date Calculation" ...
func setCalcMode(cli *CLI, winID, mode string) error {
	s := &Session{cli: cli, Win: Window{ID: winID}}
	nav, err := s.findOne("button", "Open Navigation", 5*time.Second)
	if nav == nil {
		return fmt.Errorf("no navigation button (%v)", err)
	}
	if _, _, _, err := cli.Run(15*time.Second, "click", nav.Ref); err != nil {
		return err
	}
	// The modes are options named "Scientific Calculator", "Date calculation Calculator" ...;
	// a click doesn't switch, select does.
	opt, err := s.findOne("option", mode+" Calculator", 5*time.Second)
	if opt == nil {
		return fmt.Errorf("no option %q (%v)", mode+" Calculator", err)
	}
	if _, _, _, err := cli.Run(15*time.Second, "select", opt.Ref, mode+" Calculator"); err != nil {
		return err
	}
	time.Sleep(1500 * time.Millisecond)
	if b, _ := s.findOne("button", "Open Navigation", 5*time.Second); b == nil {
		return fmt.Errorf("the navigation is still open after selecting %q", mode)
	}
	return nil
}
