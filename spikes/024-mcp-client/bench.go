package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// The tool-style bench (ticket point 4): about 10 scene tasks on Viso,
// the same prompt for every model, under both tool styles. The harness
// seeds the spike's own test project before each task and checks the
// scene after it.

const benchProject = "jenab-spike-024 test"

type seedObj struct {
	Name  string
	Args  string // add_object's arguments without the name
	Light bool   // add_light instead
}

type benchTask struct {
	ID     string
	Prompt string
	Seed   []seedObj
	Check  func(sc scene, answer string) (bool, string)
}

var benchTasks = []benchTask{
	{ID: "T01", Prompt: "In the scene open in VISO, add a red box named Crate standing on the floor.",
		Check: func(sc scene, _ string) (bool, string) {
			o := sc.find("Crate")
			if o == nil {
				return false, "no Crate"
			}
			if !strings.Contains(strings.ToLower(o.Kind), "box") {
				return false, "Crate is " + o.Kind
			}
			r, g, b := rgb(o.Color)
			if r < 0.6 || g > 0.35 || b > 0.35 {
				return false, "Crate colour " + o.Color
			}
			if o.HasSpan && math.Abs(o.Bottom) > 0.05 {
				return false, fmt.Sprintf("Crate bottom at %.2f", o.Bottom)
			}
			return sc.count(1)
		}},
	{ID: "T02", Prompt: "In the scene open in VISO, move the ball 2 m to the right, along +x. Keep its height and depth.",
		Seed: []seedObj{{Name: "Ball", Args: `"shapeType":"sphere","position":[0,0.5,0]`}},
		Check: func(sc scene, _ string) (bool, string) {
			o := sc.find("Ball")
			if o == nil {
				return false, "no Ball"
			}
			if !near(o.Pos, [3]float64{2, 0.5, 0}, 0.1) {
				return false, fmt.Sprintf("Ball at %v", o.Pos)
			}
			return sc.count(1)
		}},
	{ID: "T03", Prompt: "In the scene open in VISO, make the vase blue.",
		Seed: []seedObj{{Name: "Vase", Args: `"shapeType":"cylinder","color":"#888888"`}},
		Check: func(sc scene, _ string) (bool, string) {
			o := sc.find("Vase")
			if o == nil {
				return false, "no Vase"
			}
			r, g, b := rgb(o.Color)
			if b < 0.5 || r > 0.4 || b < g {
				return false, "Vase colour " + o.Color
			}
			return sc.count(1)
		}},
	{ID: "T04", Prompt: "In the scene open in VISO, hide the box named Spare, but don't delete it.",
		Seed: []seedObj{{Name: "Crate", Args: `"shapeType":"box","position":[-1.5,0.5,0]`},
			{Name: "Spare", Args: `"shapeType":"box","position":[0,0.5,0]`},
			{Name: "Lamp", Args: `"shapeType":"cylinder","position":[1.5,0.5,0]`}},
		Check: func(sc scene, _ string) (bool, string) {
			o := sc.find("Spare")
			if o == nil {
				return false, "no Spare"
			}
			if !o.Hidden {
				return false, "Spare is visible"
			}
			for _, n := range []string{"Crate", "Lamp"} {
				if p := sc.find(n); p == nil || p.Hidden {
					return false, n + " is gone or hidden"
				}
			}
			return sc.count(3)
		}},
	{ID: "T05", Prompt: "In the scene open in VISO, group the two boxes into a group called Pair.",
		Seed: []seedObj{{Name: "Left", Args: `"shapeType":"box","position":[-1,0.5,0]`},
			{Name: "Right", Args: `"shapeType":"box","position":[1,0.5,0]`},
			{Name: "Ball", Args: `"shapeType":"sphere","position":[0,0.5,2]`}},
		Check: func(sc scene, _ string) (bool, string) {
			g := sc.find("Pair")
			if g == nil {
				return false, "no Pair"
			}
			if l, r, b := sc.find("Left"), sc.find("Right"), sc.find("Ball"); l == nil || r == nil || b == nil {
				return false, "an object is gone"
			} else if sc.Parents && (l.Parent != g.ID || r.Parent != g.ID || b.Parent == g.ID) {
				return false, fmt.Sprintf("parents: Left %q Right %q Ball %q, Pair %q", l.Parent, r.Parent, b.Parent, g.ID)
			}
			return true, ""
		}},
	{ID: "T06", Prompt: "In the scene open in VISO, line up the boxes A, B and C so they all share the same x position.",
		Seed: []seedObj{{Name: "A", Args: `"shapeType":"box","position":[-2,0.5,-2]`},
			{Name: "B", Args: `"shapeType":"box","position":[0.5,0.5,0]`},
			{Name: "C", Args: `"shapeType":"box","position":[3,0.5,2]`}},
		Check: func(sc scene, _ string) (bool, string) {
			a, b, c := sc.find("A"), sc.find("B"), sc.find("C")
			if a == nil || b == nil || c == nil {
				return false, "a box is gone"
			}
			if math.Abs(a.Pos[0]-b.Pos[0]) > 0.05 || math.Abs(a.Pos[0]-c.Pos[0]) > 0.05 {
				return false, fmt.Sprintf("x: %.2f %.2f %.2f", a.Pos[0], b.Pos[0], c.Pos[0])
			}
			return sc.count(3)
		}},
	{ID: "T07", Prompt: "In the scene open in VISO, put a sphere named Orb on top of the box named Stand, resting on it.",
		Seed: []seedObj{{Name: "Stand", Args: `"shapeType":"box","scale":[1,0.8,1],"restOn":"floor"`}},
		Check: func(sc scene, _ string) (bool, string) {
			o, s := sc.find("Orb"), sc.find("Stand")
			if o == nil || s == nil {
				return false, "Orb or Stand missing"
			}
			if math.Abs(o.Pos[0]-s.Pos[0]) > 0.5 || math.Abs(o.Pos[2]-s.Pos[2]) > 0.5 {
				return false, fmt.Sprintf("Orb at %v, Stand at %v", o.Pos, s.Pos)
			}
			if o.HasSpan && s.HasSpan && math.Abs(o.Bottom-s.Top) > 0.05 {
				return false, fmt.Sprintf("Orb bottom %.2f, Stand top %.2f", o.Bottom, s.Top)
			}
			return sc.count(2)
		}},
	{ID: "T08", Prompt: "In the scene open in VISO, add a point light 3 m above the centre of the floor.",
		Seed: []seedObj{{Name: "Table", Args: `"shapeType":"box","scale":[1.5,0.1,1],"position":[0,0.75,0]`}},
		Check: func(sc scene, _ string) (bool, string) {
			for _, o := range sc.Objects {
				if o.Light && strings.Contains(strings.ToLower(o.Kind), "point") {
					if near(o.Pos, [3]float64{0, 3, 0}, 0.15) {
						return true, ""
					}
					return false, fmt.Sprintf("point light at %v", o.Pos)
				}
			}
			return false, "no point light"
		}},
	{ID: "T09", Prompt: "In the scene open in VISO, delete the box named Old.",
		Seed: []seedObj{{Name: "Old", Args: `"shapeType":"box","position":[-1,0.5,0]`},
			{Name: "Keep", Args: `"shapeType":"sphere","position":[1,0.5,0]`}},
		Check: func(sc scene, _ string) (bool, string) {
			if sc.find("Old") != nil {
				return false, "Old is still there"
			}
			if sc.find("Keep") == nil {
				return false, "Keep is gone"
			}
			return sc.count(1)
		}},
	{ID: "T10", Prompt: "Which objects are in the scene open in VISO? Answer with their names only.",
		Seed: []seedObj{{Name: "Teapot", Args: `"shapeType":"sphere","position":[-1,0.5,0]`},
			{Name: "Mug", Args: `"shapeType":"cylinder","position":[0,0.5,0]`},
			{Name: "Tray", Args: `"shapeType":"box","scale":[1,0.05,1],"position":[1,0.025,0]`}},
		Check: func(sc scene, answer string) (bool, string) {
			for _, n := range []string{"Teapot", "Mug", "Tray"} {
				if !strings.Contains(answer, n) {
					return false, "answer lacks " + n
				}
			}
			return sc.count(3)
		}},
}

// scene is what the check reads from get_scene.
type scene struct {
	Text    string
	Objects []sceneObj
	Parents bool // the text says which group an object is in
}

type sceneObj struct {
	ID, Name, Kind, Color, Parent string
	Pos                           [3]float64
	Bottom, Top                   float64
	HasSpan, Hidden, Light        bool
}

func (sc scene) find(name string) *sceneObj {
	for i := range sc.Objects {
		if strings.EqualFold(sc.Objects[i].Name, name) {
			return &sc.Objects[i]
		}
	}
	return nil
}

// count says whether the scene holds n objects (lights and groups not
// counted), so a task didn't add or delete more than it should.
func (sc scene) count(n int) (bool, string) {
	k := 0
	for _, o := range sc.Objects {
		if !o.Light && !strings.Contains(strings.ToLower(o.Kind), "group") {
			k++
		}
	}
	if k != n {
		return false, fmt.Sprintf("%d objects, want %d", k, n)
	}
	return true, ""
}

func near(a, b [3]float64, tol float64) bool {
	for i := range a {
		if math.Abs(a[i]-b[i]) > tol {
			return false
		}
	}
	return true
}

func rgb(hex string) (r, g, b float64) {
	var x, y, z int
	if _, err := fmt.Sscanf(strings.TrimPrefix(strings.TrimSpace(hex), "#"), "%02x%02x%02x", &x, &y, &z); err != nil {
		return -1, -1, -1
	}
	return float64(x) / 255, float64(y) / 255, float64(z) / 255
}

var nameRe = regexp.MustCompile(`"name"\s*:\s*"([^"]*)"`)

type benchResult struct {
	Model string `json:"model"`
	Style string `json:"style"`
	Task  string `json:"task"`
	Rep   int    `json:"rep"`
	OK    bool   `json:"ok"`
	Why   string `json:"why,omitempty"`
	runOut
}

func (r benchResult) name() string {
	return fmt.Sprintf("bench_%s_%s_%s_r%d", safe(r.Model), r.Style, r.Task, r.Rep)
}

// call runs a Viso tool for the harness itself (seeding and checks).
func (v *viso) call(ctx context.Context, name, args string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	r, err := v.cl.Call(ctx, name, json.RawMessage(args), nil)
	if err != nil {
		return "", err
	}
	if r.IsError {
		return r.Text, fmt.Errorf("%s: %s", name, clip(r.Text, 300))
	}
	return r.Text, nil
}

// prepare makes sure the spike's project is open and holds only the seed.
func (v *viso) prepare(ctx context.Context, project string, seed []seedObj) error {
	if _, err := v.call(ctx, "open_project", fmt.Sprintf(`{"id":%q}`, project)); err != nil {
		return err
	}
	if _, err := v.call(ctx, "clear_scene", `{}`); err != nil {
		return err
	}
	for _, s := range seed {
		tool := "add_object"
		if s.Light {
			tool = "add_light"
		}
		if _, err := v.call(ctx, tool, fmt.Sprintf(`{"name":%q,%s}`, s.Name, s.Args)); err != nil {
			return err
		}
	}
	return nil
}

func (v *viso) scene(ctx context.Context) (scene, error) {
	txt, err := v.call(ctx, "get_scene", `{}`)
	if err != nil {
		return scene{Text: txt}, err
	}
	return parseScene(txt), nil
}

// bench runs the tasks. project is the spike's test project id.
func (e *env) bench(ctx context.Context, v *viso, project string, models, styles []string, tasks []string, reps int) {
	path := filepath.Join(e.out, "bench.jsonl")
	local := filepath.Join(e.out, "local")
	done := map[string]bool{}
	if f, err := os.Open(path); err == nil {
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 64<<20)
		for sc.Scan() {
			var r benchResult
			if json.Unmarshal(sc.Bytes(), &r) == nil && r.Err == "" {
				done[r.name()] = true
			}
		}
		f.Close()
	}
	v.created[project] = true
	for rep := 1; rep <= reps; rep++ {
		for _, t := range benchTasks {
			if len(tasks) > 0 && !contains(tasks, t.ID) {
				continue
			}
			for _, style := range styles {
				for _, m := range models {
					r := benchResult{Model: m, Style: style, Task: t.ID, Rep: rep}
					if done[r.name()] || ctx.Err() != nil {
						continue
					}
					if !e.budget.ok(m) {
						log.Printf("skip %s: Gemini budget", r.name())
						continue
					}
					if err := v.prepare(ctx, project, t.Seed); err != nil {
						log.Printf("%s: seed failed: %v", r.name(), err)
						return
					}
					st := &runState{guard: v.guard}
					s, err := newServer("viso", visoTitle, v.cl, v.tools, offerViso, st)
					if err != nil {
						log.Print(err)
						return
					}
					r.runOut = e.h.run(ctx, agentRun{Model: m, Style: style, Prompt: t.Prompt, Servers: []*server{s},
						Instr: map[string]string{"viso": v.instr}, Policy: visoPolicy})
					e.budget.add(m, r.runOut)
					v.noteCreated(r.Calls)
					sc, err := v.scene(ctx)
					if err != nil {
						r.Why = "get_scene: " + err.Error()
					} else {
						r.OK, r.Why = t.Check(sc, r.Answer)
					}
					if r.Err != "" {
						r.OK = false
					}
					// The full row and transcript go to the git-ignored local
					// folder; the committed ones leave out Viso's own texts.
					extra := "Check: ok %v %s\n\nScene after:\n\n```\n%s\n```"
					appendJSON(filepath.Join(local, "bench.jsonl"), r)
					e.transcriptTo(filepath.Join(local, "transcripts"), r.name(), t.Prompt, "", r.runOut,
						fmt.Sprintf(extra, r.OK, r.Why, clip(sc.Text, 3000)))
					tr := trimViso(r)
					appendJSON(path, tr)
					e.transcriptTo(filepath.Join(e.out, "transcripts"), r.name(), t.Prompt, "", tr.runOut,
						fmt.Sprintf(extra, r.OK, r.Why, sceneIndex(sc.Text)))
					log.Printf("%s: ok %v %s · req %d, prompt %d (cached %d), calls %d, mcp errors %v %s", r.name(), r.OK, r.Why,
						r.Requests, r.Prompt, r.Cached, len(r.Calls), countErr(r.Calls), r.Err)
				}
			}
		}
	}
}

// trimViso returns r with Viso's own texts left out: mcp_describe results
// (tool descriptions and schemas) become a placeholder, get_scene results
// their scene index. Errors and other results stay as they are.
func trimViso(r benchResult) benchResult {
	calls := make([]call, len(r.Calls))
	for i, c := range r.Calls {
		switch {
		case c.Error:
		case c.Tool == "mcp_describe":
			c.Result = fmt.Sprintf("[Viso tool description and schema, %d bytes, not committed]", len(c.Result))
		case visoTarget(c) == "get_scene":
			c.Result = sceneIndex(c.Result)
		}
		calls[i] = c
	}
	r.Calls = calls
	return r
}

// visoTarget is the Viso tool a call reached, in either style.
func visoTarget(c call) string {
	if c.Tool == "mcp_call" {
		var a struct {
			Tool string `json:"tool"`
		}
		_ = json.Unmarshal([]byte(c.Args), &a)
		return a.Tool
	}
	return strings.TrimPrefix(c.Tool, "viso_")
}

func countErr(cs []call) int {
	n := 0
	for _, c := range cs {
		if c.Error {
			n++
		}
	}
	return n
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// noteCreated records projects a model made with create_project, so the
// report can list them for the user to remove.
func (v *viso) noteCreated(cs []call) {
	for _, c := range cs {
		t, a := c.Tool, c.Args
		if t == "mcp_call" {
			var ca struct {
				Tool      string          `json:"tool"`
				Arguments json.RawMessage `json:"arguments"`
			}
			_ = json.Unmarshal([]byte(c.Args), &ca)
			t, a = ca.Tool, string(ca.Arguments)
		}
		if strings.HasSuffix(t, "create_project") && !c.Error {
			name := ""
			if m := nameRe.FindStringSubmatch(a); m != nil {
				name = m[1]
			}
			v.modelMade = append(v.modelMade, name+" → "+clip(c.Result, 200))
		}
	}
}
