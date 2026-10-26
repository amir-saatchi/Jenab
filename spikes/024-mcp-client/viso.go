package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
)

// Viso as the real server: Viso.exe --mcp in a Job Object, with the
// go-sdk client. The models get the scene tools plus create_project and
// open_project; open_project only for projects the spike created.

const visoTitle = "VISO, a 3D scene editor app on this computer"

// never are the tools no model gets: they touch the user's own projects,
// files, credits or the cloud.
var never = []string{"list_projects", "rename_project", "duplicate_project", "load_local_model",
	"get_credit_balance", "generate_image", "preview_generation", "get_generation"}

func offerViso(t toolInfo) bool { return !slices.Contains(never, t.Name) }

type viso struct {
	cl        mcpClient
	job       *job
	stopWatch func()
	tools     []toolInfo
	instr     string
	created   map[string]bool // project ids the spike created
	modelMade []string        // projects a model created during the bench
}

// openViso starts the shim in a job. The job is watched from the first
// moment: once it holds anything besides the shim (Viso launched by a
// call), kill-on-close is turned off, so neither close nor a crash of the
// spike can stop that app.
func openViso(ctx context.Context) (*viso, error) {
	j, err := newJob()
	if err != nil {
		return nil, err
	}
	cl, err := goStdio(ctx, exec.Command(visoExe, "--mcp"), j)
	if err != nil {
		j.release()
		return nil, err
	}
	v := &viso{cl: cl, job: j, stopWatch: j.watch(cl.PID()), created: map[string]bool{}}
	if r := cl.s.InitializeResult(); r != nil {
		v.instr = r.Instructions
	}
	if v.tools, err = cl.Tools(ctx); err != nil {
		v.close()
		return nil, err
	}
	return v, nil
}

// withViso runs f with Viso open and always closes it, also on an error
// or a panic in f.
func withViso(ctx context.Context, f func(v *viso) error) error {
	v, err := openViso(ctx)
	if err != nil {
		return err
	}
	defer v.close()
	return f(v)
}

// close stops the shim and its tree, but never a process the shim didn't
// start for this connection: if the job holds more than the shim (Viso
// launched by a call), only the shim is stopped and the rest keeps running.
func (v *viso) close() {
	v.stopWatch()
	shim := v.cl.PID()
	before := tree(shim)
	jobPIDs, qerr := v.job.pids()
	_ = v.cl.Close()
	others, err := v.job.closeOnly(shim)
	time.Sleep(300 * time.Millisecond)
	log.Printf("Viso shim %d stopped; job held %v (err %v); left running on purpose: %v (err %v); shim tree before: %s; still running: %s",
		shim, jobPIDs, qerr, others, err, procList(before), procList(still(before)))
}

// guard keeps models inside the spike's projects.
func (v *viso) guard(srv, name string, args json.RawMessage) string {
	if slices.Contains(never, name) {
		return name + " is not available in this test"
	}
	if name == "open_project" {
		var a struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(args, &a)
		if !v.created[a.ID] {
			return "open_project is allowed only for projects created in this chat"
		}
	}
	return ""
}

// policy is the scripted user for Viso tasks: Always for tools that only
// read, Once for every change.
func visoPolicy(c card) (chat.Grant, string) {
	if slices.Contains(never, c.Tool) {
		return chat.GrantDeny, "Not in this test."
	}
	if c.ReadOnly {
		return chat.GrantAlways, ""
	}
	return chat.GrantOnce, ""
}

// calib measures what the tools cost in prompt tokens per model: one
// turn that asks for no tool, under each condition.
func (e *env) calib(ctx context.Context, v *viso, models []string) {
	path := filepath.Join(e.out, "calib.jsonl")
	conds := []struct{ name, style string }{{"none", ""}, {"native-all", "native"}, {"native-offered", "native"}, {"describe", "describe"}}
	for _, m := range models {
		for _, c := range conds {
			if !e.budget.ok(m) {
				continue
			}
			st := &runState{guard: func(string, string, json.RawMessage) string { return "not in this test" }}
			var servers []*server
			instr := map[string]string{}
			if c.style != "" {
				offer := offerViso
				if c.name == "native-all" {
					offer = nil
				}
				s, err := newServer("viso", visoTitle, v.cl, v.tools, offer, st)
				if err != nil {
					log.Print(err)
					continue
				}
				servers = []*server{s}
			}
			a := agentRun{Model: m, Style: c.style, Prompt: "Reply with the single word OK. Do not call any tool.",
				Servers: servers, Instr: instr, Policy: func(card) (chat.Grant, string) { return chat.GrantDeny, "not in this test" }}
			if c.style == "" {
				a.Style = "native"
			}
			o := e.h.run(ctx, a)
			e.budget.add(m, o)
			n := 0
			if len(servers) > 0 {
				n = len(servers[0].order)
			}
			rec := map[string]any{"model": m, "cond": c.name, "tools": n, "requests": o.Requests, "first": o.First,
				"prompt": o.Prompt, "cached": o.Cached, "calls": len(o.Calls), "err": o.Err, "errors": o.Errors, "card_bytes": len(a.card())}
			appendJSON(path, rec)
			log.Printf("calib %s %s: %d tools, first request %d prompt tokens, %d calls %s %v", m, c.name, n, o.First, len(o.Calls), o.Err, o.Errors)
		}
	}
}

// schemaBytes is the JSON size of the tools a model would get.
func schemaBytes(tools []toolInfo, offer func(toolInfo) bool) (n, bytes int) {
	for _, t := range tools {
		if offer != nil && !offer(t) {
			continue
		}
		b, _ := json.Marshal(map[string]any{"name": t.Name, "description": t.Description, "parameters": t.InputSchema})
		n++
		bytes += len(b)
	}
	return
}

func describeCost(v *viso) string {
	var b strings.Builder
	n, by := schemaBytes(v.tools, nil)
	fmt.Fprintf(&b, "all tools: %d, %d bytes of JSON\n", n, by)
	n, by = schemaBytes(v.tools, offerViso)
	fmt.Fprintf(&b, "offered tools: %d, %d bytes of JSON\n", n, by)
	s, _ := newServer("viso", visoTitle, v.cl, v.tools, offerViso, &runState{})
	fmt.Fprintf(&b, "describe-style card: %d bytes; server instructions: %d bytes\n", len(s.cardBlock(true, "")), len(v.instr))
	return b.String()
}

// probe saves the projects list to the scratch folder (never to results:
// it holds the user's project names), creates the test project unless
// it exists, and prints get_scene before and after one add_object.
func (v *viso) probe(ctx context.Context, scratch string) error {
	list, err := v.call(ctx, "list_projects", `{}`)
	if err != nil {
		return fmt.Errorf("list_projects: %w", err)
	}
	f := filepath.Join(scratch, fmt.Sprintf("projects-%d.txt", time.Now().Unix()))
	_ = os.WriteFile(f, []byte(list), 0o600)
	fmt.Printf("projects list saved to %s (%d bytes, %d lines); has the test project: %v\n", f, len(list),
		strings.Count(list, "\n")+1, strings.Contains(list, benchProject))
	if strings.Contains(list, benchProject) {
		for _, line := range strings.Split(list, "\n") {
			if strings.Contains(line, benchProject) {
				fmt.Println("test project line:", line)
			}
		}
		return nil
	}
	r, err := v.call(ctx, "create_project", fmt.Sprintf(`{"name":%q,"description":"Made by Jenab's SPIKE-024 test; safe to delete."}`, benchProject))
	fmt.Printf("create_project: %s err=%v\n", r, err)
	sc, err := v.call(ctx, "get_scene", `{}`)
	fmt.Printf("get_scene (new project): err=%v\n%s\n", err, sc)
	r, err = v.call(ctx, "add_object", `{"name":"Probe","shapeType":"box","color":"#ff0000","scale":[1,0.8,1],"restOn":"floor"}`)
	fmt.Printf("add_object: %s err=%v\n", r, err)
	r, err = v.call(ctx, "add_light", `{"lightType":"point","position":[0,3,0]}`)
	fmt.Printf("add_light: %s err=%v\n", r, err)
	sc, err = v.call(ctx, "get_scene", `{}`)
	fmt.Printf("get_scene (after): err=%v\n%s\n", err, sc)
	return nil
}

// try runs harness calls on the test project, then shows get_scene and
// what parseScene made of it.
func (v *viso) try(ctx context.Context, project, calls string) error {
	if _, err := v.call(ctx, "open_project", fmt.Sprintf(`{"id":%q}`, project)); err != nil {
		return fmt.Errorf("open_project: %w", err)
	}
	for _, line := range strings.Split(calls, "\n") {
		name, args, _ := strings.Cut(strings.TrimSpace(line), " ")
		if name == "" {
			continue
		}
		if why := v.guard("viso", name, json.RawMessage(args)); why != "" {
			return errors.New(why)
		}
		if args == "" {
			args = "{}"
		}
		r, err := v.call(ctx, name, args)
		fmt.Printf("%s: %s err=%v\n", name, r, err)
	}
	sc, err := v.scene(ctx)
	fmt.Printf("get_scene err=%v\n%s\n", err, sc.Text)
	for _, o := range sc.Objects {
		fmt.Printf("parsed: %+v\n", o)
	}
	return nil
}
