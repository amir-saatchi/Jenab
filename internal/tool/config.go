package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/store"
	"github.com/amir-saatchi/jenab/internal/view"
	"go.yaml.in/yaml/v4"
)

// The config tools (SPEC 8.1, P3-03): save_view, save_page, validate,
// read_config, open_page and delete_config. A save validates first (10)
// and saves only a config without errors; the result has every issue, so
// the model can fix them all in one try. They are in block 1 with the
// data tools, as views (Areas).

// RepairAttempts is how many more saves of one config a turn may try
// after the first failed (SPEC 10): the fifth failure in a row tells the
// model to stop and show the user the remaining errors, and a sixth save
// is refused without being checked.
const RepairAttempts = 4

const saveViewDescription = `Save a view: a table, chart, stat or form that pages show. Pass the whole view as YAML in config; its id names it, and saving an id that exists replaces that view. The view is validated first and saved only when it has no errors; the result lists every error with its line, so fix them all and save again. To change a view you read, pass the revision read_config showed. Load the config-guide skill before your first view or page.`

const savePageDescription = `Save a page: rows of views, headings, text, filters and buttons the user opens from the sidebar. Pass the whole page as YAML in config; its id names it, and saving an id that exists replaces that page. Every view a page shows must be saved first. The page is validated first and saved only when it has no errors; the result lists every error with its line. To change a page you read, pass the revision read_config showed.`

// validate has no repair limit of its own: it changes nothing, and the
// request cap bounds it: llm.turn_max_requests a turn (25 by default) and
// subMaxRequests (15) a subagent run, in internal/agent.
const validateDescription = `Check a view or a page without saving it: the same checks as save_view and save_page, with every error and warning and its line.`

const readConfigDescription = `Read a saved view or page by its id: its YAML as stored, with its revision. Read one before you change it, then save the whole changed config.`

const openPageDescription = `Ask the app to open a saved page for the user, with filter values if you give them (filters: the page's filter params and their values). The user sees a chip with Open in the chat.`

const deleteConfigDescription = `Delete a saved view or page by its id. Only when the user asked to remove it: taking a view off a page doesn't need a delete. A view still on a page, or opened by a page's button or a table's row action, is not deleted; the result names what uses it. Undo turn brings it back.`

type configArgs struct {
	Config   string `json:"config"`
	Revision int    `json:"revision"`
}

// validateArgs are validate's: a config, nothing to compare.
type validateArgs struct {
	Config string `json:"config"`
}

type idArgs struct {
	ID string `json:"id"`
}

type openPageArgs struct {
	ID      string         `json:"id"`
	Filters map[string]any `json:"filters"`
}

var configSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"config": {"type": "string", "description": "the whole config as YAML"},
		"revision": {"type": "integer", "description": "the revision read_config showed; leave out for a new config"}
	},
	"required": ["config"],
	"additionalProperties": false
}`)

var validateSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"config": {"type": "string", "description": "the whole config as YAML"}
	},
	"required": ["config"],
	"additionalProperties": false
}`)

var idSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"id": {"type": "string", "description": "the view's or page's id, as the project card lists it"}
	},
	"required": ["id"],
	"additionalProperties": false
}`)

// ConfigTools are the config tools. The two saves share the turn's
// repair counts.
func ConfigTools() []Tool {
	r := &repairs{turns: map[repairKey]*turnRepairs{}}
	spec := func(name, desc string, schema json.RawMessage, e Effects) Spec {
		return Spec{Name: name, Description: desc, Schema: schema, Effects: e, Areas: Views, When: WhenTables}
	}
	return []Tool{
		Func(spec("save_view", saveViewDescription, configSchema, WritesDB), func(ctx context.Context, env *Env, a configArgs) (Result, error) {
			return r.save(ctx, env, "save_view", store.KindView, a)
		}),
		Func(spec("save_page", savePageDescription, configSchema, WritesDB), func(ctx context.Context, env *Env, a configArgs) (Result, error) {
			return r.save(ctx, env, "save_page", store.KindPage, a)
		}),
		Func(spec("validate", validateDescription, validateSchema, ReadsDB), runValidate),
		Func(spec("read_config", readConfigDescription, idSchema, ReadsDB), runReadConfig),
		Func(spec("open_page", openPageDescription, json.RawMessage(`{
			"type": "object",
			"properties": {
				"id": {"type": "string", "description": "the page's id"},
				"filters": {"type": "object", "description": "filter values by param, such as {\"from_date\": \"2026-01-01\"}; leave out for the page's defaults"}
			},
			"required": ["id"],
			"additionalProperties": false
		}`), ReadsDB), runOpenPage),
		Func(spec("delete_config", deleteConfigDescription, idSchema, WritesDB), runDeleteConfig),
	}
}

func validator(env *Env) *view.Validator {
	return &view.Validator{Env: view.ProjectEnv{DB: env.Project.DB}}
}

// repairs counts each turn's failed saves of each config (SPEC 10).
type repairs struct {
	mu    sync.Mutex
	turns map[repairKey]*turnRepairs
}

type repairKey struct {
	chat id.Chat
	turn int
	sub  string // a subagent run's Env.Run; "" for the chat's turn
}

// turnRepairs are one turn's saves that failed: by tool and config,
// the failures in a row and the last one's error count. A save that
// succeeds starts the config's count again.
type turnRepairs struct {
	failed  map[string]*failure // by "save_view daily_btc"
	order   []string
	checked bool // the early-stop check is set up
	told    bool // the early-stop notice was given
}

type failure struct {
	n, errors int
}

// turn is the state of env's turn, set up on its first save: the chat's
// turn, or a subagent run's, each counted on its own. Nil where there is
// no turn to end it, which then has no limit.
func (r *repairs) turn(env *Env) *turnRepairs {
	if env.AtTurnEnd == nil {
		return nil
	}
	k := repairKey{env.Chat, env.Turn, env.Run}
	r.mu.Lock()
	defer r.mu.Unlock()
	t := r.turns[k]
	if t == nil {
		t = &turnRepairs{failed: map[string]*failure{}}
		r.turns[k] = t
		env.AtTurnEnd(func() {
			r.mu.Lock()
			delete(r.turns, k)
			r.mu.Unlock()
		})
	}
	return t
}

// save validates src as a view or a page and saves it when it has no
// errors.
func (r *repairs) save(ctx context.Context, env *Env, tool string, kind store.ConfigKind, a configArgs) (Result, error) {
	src := a.Config
	if a.Revision < 0 {
		return Result{}, Errorf("revision is the number read_config showed, 1 or more; leave it out for a new %s", kind)
	}
	t := r.turn(env)
	key := tool + " " + yamlID(src)
	r.mu.Lock()
	var f failure
	if t != nil && t.failed[key] != nil {
		f = *t.failed[key]
	}
	r.mu.Unlock()
	if f.n > RepairAttempts {
		return Result{}, Errorf("%s: %d saves of this config failed this turn, which is the limit; don't try again. Show the user the remaining errors and ask how to go on.", key, f.n)
	}
	v := validator(env)
	var (
		res          view.Result
		cid, typ, tt string
		err          error
	)
	if kind == store.KindView {
		var pv *view.View
		pv, res, err = v.View(ctx, []byte(src))
		if pv != nil {
			cid, typ, tt = pv.ID, pv.Type, pv.Title
		}
	} else {
		var pp *view.Page
		pp, res, err = v.Page(ctx, []byte(src))
		if pp != nil {
			cid, tt = pp.ID, pp.Title
		}
	}
	if err != nil {
		return Result{}, err
	}
	if !res.OK() {
		return Result{}, r.failed(env, t, key, len(res.Errors())+res.MoreErrors, res.String())
	}
	if testBeforeWrite != nil {
		testBeforeWrite()
	}
	// What the config refers to is checked again in the write: a view or
	// page it uses may have been deleted since it was validated.
	check := func(all []store.Config) error {
		if ms := view.MissingRefs(nil, store.Config{Kind: kind, ID: cid, YAML: src}, all); len(ms) > 0 {
			return &refsGone{issues: ms}
		}
		return nil
	}
	w, err := env.Project.DB.SaveConfig(ctx, store.ConfigSave{Kind: kind, ID: cid, Type: typ, Title: tt, YAML: src, Expected: a.Revision, Source: env.Source, Check: check})
	if err != nil {
		var gone *refsGone
		var conflict *store.ConfigConflictError
		switch {
		case errors.As(err, &gone):
			issues := fmt.Sprintf("%s:\n%s\n", plural(len(gone.issues), "error"), strings.Join(gone.issues, "\n"))
			return Result{}, r.failed(env, t, key, len(gone.issues), issues)
		case errors.As(err, &conflict):
			return Result{}, staleSave(kind, cid, a.Revision, conflict.Current)
		}
		if errors.Is(err, store.ErrInvalid) {
			return Result{}, r.failed(env, t, key, 1, "1 error:\n"+strings.TrimPrefix(err.Error(), "store: ")+"\n")
		}
		return Result{}, err
	}
	if t != nil {
		r.mu.Lock()
		delete(t.failed, key)
		r.mu.Unlock()
	}
	name := string(kind) + " " + w.Config.ID
	var text string
	if w.Op == "" {
		text = fmt.Sprintf("No change: %s is already saved like this (revision %d).", name, w.Config.Revision)
	} else {
		text = fmt.Sprintf("Saved %s, revision %d.", name, w.Config.Revision)
	}
	if ws := res.Warnings(); len(ws) > 0 {
		text += "\n" + res.String()
	}
	out := Result{Text: strings.TrimRight(text, "\n")}
	if w.Op != "" {
		out.Notices = []chat.Notice{ConfigChip(w)}
	}
	return out, nil
}

// testBeforeWrite, set by tests, runs between a save's or a delete's
// checks and its write, where another chat's write may land.
var testBeforeWrite func()

// refsGone is a save refused in its write: views, forms or pages it uses
// were deleted after it was validated.
type refsGone struct{ issues []string }

func (e *refsGone) Error() string { return strings.Join(e.issues, "; ") }

// inUse is a delete refused in its write: other configs use the config.
type inUse struct{ users view.Users }

func (e *inUse) Error() string { return "in use" }

// staleSave is the error for a save whose revision isn't the current one.
func staleSave(kind store.ConfigKind, cid string, read int, cur store.Config) error {
	if !cur.Live() {
		return Errorf("Not saved: the %s %s was deleted after you read revision %d. Ask the user before saving it again.", kind, cid, read)
	}
	return Errorf("Not saved: the %s %s changed after you read revision %d; it is at revision %d now. Read it again with read_config and make your change on that.", kind, cid, read, cur.Revision)
}

// failed counts a failed save of key and returns its error: the issues,
// and at the limit that it was the last try.
func (r *repairs) failed(env *Env, t *turnRepairs, key string, errs int, issues string) error {
	n := 1
	if t != nil {
		r.mu.Lock()
		f := t.failed[key]
		if f == nil {
			f = &failure{}
			t.failed[key] = f
		}
		t.order = append(slices.DeleteFunc(t.order, func(k string) bool { return k == key }), key)
		f.n++
		f.errors = errs
		n = f.n
		check := !t.checked
		t.checked = true
		r.mu.Unlock()
		if check && env.BeforeAnswer != nil {
			env.BeforeAnswer(chat.NoticeEarlyStop, func() string { return r.earlyStop(t) })
		}
	}
	msg := "Not saved: " + strings.TrimRight(issues, "\n")
	if n > RepairAttempts {
		msg += fmt.Sprintf("\nThat was the last try: %d saves of this config failed this turn. Don't save it again; show the user the remaining errors and ask how to go on.", n)
	}
	return &Error{Msg: msg}
}

// earlyStop is the notice for a turn about to end right after a failed
// save it didn't try again (SPEC 10), once a turn. A config at the limit
// was already told to stop. The same for every model.
func (r *repairs) earlyStop(t *turnRepairs) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t.told {
		return ""
	}
	var parts []string
	for _, k := range t.order {
		f := t.failed[k]
		if f == nil || f.n > RepairAttempts {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s failed validation (%s) and was not saved.", k, plural(f.errors, "error")))
	}
	if len(parts) == 0 {
		return ""
	}
	t.told = true
	return strings.Join(parts, " ") + " Fix and save it, or tell the user why not."
}

// yamlIDLine is a config's top-level id line.
var yamlIDLine = regexp.MustCompile(`(?m)^id:[ \t]*["']?([^"'\s#]+)`)

// yamlID is a config's id for its repair count: the top-level id of the
// YAML when it parses, in any style, else the id line; "(no id)" when
// there is none.
func yamlID(src string) string {
	var doc map[string]any
	if yaml.Unmarshal([]byte(src), &doc) == nil {
		if s, ok := doc["id"].(string); ok && strings.TrimSpace(s) != "" {
			return store.NormConfigID(strings.TrimSpace(s))
		}
		return "(no id)"
	}
	if m := yamlIDLine.FindStringSubmatch(src); m != nil {
		return store.NormConfigID(m[1])
	}
	return "(no id)"
}

// ConfigChip is the chat's chip for a saved or deleted view or page: its
// undo is Undo turn's, and a page's chip has Open (P3-03).
func ConfigChip(w store.ConfigWrite) chat.Notice {
	what := "View"
	if w.Config.Kind == store.KindPage {
		what = "Page"
	}
	n := chat.Notice{Kind: chat.NoticeConfigSaved, Text: what + " saved: " + w.Config.ID, Target: w.Config.Target(), Change: string(w.Change)}
	if w.Op == "delete" {
		n.Kind, n.Text = chat.NoticeConfigDeleted, what+" deleted: "+w.Config.ID
	}
	return n
}

func runValidate(ctx context.Context, env *Env, a validateArgs) (Result, error) {
	kind, parsed, res, err := validator(env).Config(ctx, []byte(a.Config))
	if err != nil {
		return Result{}, err
	}
	what := "config"
	switch p := parsed.(type) {
	case *view.View:
		what = "view " + p.ID
	case *view.Page:
		what = "page " + p.ID
	default:
		if kind != "" {
			what = string(kind)
		}
	}
	if len(res.Issues) == 0 {
		return Result{Text: "The " + what + " is valid. Nothing was saved."}, nil
	}
	head := "The " + what + " is valid, with warnings. Nothing was saved.\n"
	if !res.OK() {
		head = "The " + what + " has errors. Nothing was saved.\n"
	}
	return Result{Text: head + strings.TrimRight(res.String(), "\n")}, nil
}

// liveConfig is the view or page id, or the tool error for one that
// isn't there, with the IDs there are.
func liveConfig(ctx context.Context, env *Env, cid string) (store.Config, error) {
	c, err := env.Project.DB.Config(ctx, "", cid)
	if errors.Is(err, store.ErrNotFound) {
		return c, notFound(ctx, env, cid)
	}
	return c, err
}

func notFound(ctx context.Context, env *Env, cid string) error {
	var ids []string
	for _, k := range []store.ConfigKind{store.KindView, store.KindPage} {
		cs, err := env.Project.DB.Configs(ctx, k)
		if err != nil {
			return err
		}
		for _, c := range cs {
			ids = append(ids, c.ID)
		}
	}
	if len(ids) == 0 {
		return Errorf("No view or page %q: the project has none saved.", cid)
	}
	return Errorf("No view or page %q. Saved: %s.", cid, strings.Join(ids, ", "))
}

func runReadConfig(ctx context.Context, env *Env, a idArgs) (Result, error) {
	c, err := liveConfig(ctx, env, a.ID)
	if err != nil {
		return Result{}, err
	}
	head := fmt.Sprintf("view %s (%s), revision %d:\n", c.ID, c.Type, c.Revision)
	if c.Kind == store.KindPage {
		head = fmt.Sprintf("page %s, revision %d:\n", c.ID, c.Revision)
	}
	return Result{Text: head + c.YAML}, nil
}

func runOpenPage(ctx context.Context, env *Env, a openPageArgs) (Result, error) {
	c, err := env.Project.DB.Config(ctx, store.KindPage, a.ID)
	if errors.Is(err, store.ErrNotFound) {
		if v, verr := env.Project.DB.Config(ctx, store.KindView, a.ID); verr == nil {
			return Result{}, Errorf("%s is a view, not a page: open the page that shows it, or save a page with it.", v.ID)
		}
		return Result{}, notFound(ctx, env, a.ID)
	}
	if err != nil {
		return Result{}, err
	}
	n := chat.Notice{Kind: chat.NoticeOpenPage, Text: "Page: " + c.ID, Target: c.Target()}
	if len(a.Filters) > 0 {
		p, _ := view.ParsePage(view.Default, []byte(c.YAML))
		var params []string
		if p != nil {
			for _, f := range p.Filters {
				params = append(params, f.Param)
			}
		}
		var bad []string
		for k := range a.Filters {
			if !slices.Contains(params, k) {
				bad = append(bad, k)
			}
		}
		if len(bad) > 0 {
			slices.Sort(bad)
			if len(params) == 0 {
				return Result{}, Errorf("The page %s has no filters; open it without filters.", c.ID)
			}
			return Result{}, Errorf("The page %s has no filter %s. Its filters: %s.", c.ID, strings.Join(bad, ", "), strings.Join(params, ", "))
		}
		if err := filterValues(c.ID, p, a.Filters); err != nil {
			return Result{}, err
		}
		b, err := json.Marshal(a.Filters)
		if err != nil {
			return Result{}, Errorf("filters: %v", err)
		}
		if len(b) > maxFilterBytes {
			return Result{}, Errorf("The filter values are %d bytes, more than %d; give shorter values.", len(b), maxFilterBytes)
		}
		n.Details = string(b)
	}
	return Result{Text: "Asked the app to open the page " + c.ID + "; the user opens it from the chip.", Notices: []chat.Notice{n}}, nil
}

func runDeleteConfig(ctx context.Context, env *Env, a idArgs) (Result, error) {
	c, err := liveConfig(ctx, env, a.ID)
	if err != nil {
		return Result{}, err
	}
	if testBeforeWrite != nil {
		testBeforeWrite()
	}
	// Who uses it is checked in the delete's write, so a page saved with
	// it meanwhile keeps it. Expected keeps a save of the config itself
	// from being lost to the delete.
	check := func(all []store.Config) error {
		if u := view.UsersOf(nil, all, c.Kind, c.ID); u.Any() {
			return &inUse{users: u}
		}
		return nil
	}
	w, err := env.Project.DB.DeleteConfig(ctx, store.ConfigDelete{Kind: c.Kind, ID: c.ID, Expected: c.Revision, Source: env.Source, Check: check})
	if err != nil {
		var used *inUse
		if errors.As(err, &used) {
			u := used.users
			var by []string
			if len(u.Pages) > 0 {
				by = append(by, word(len(u.Pages), "page ", "pages ")+strings.Join(u.Pages, ", "))
			}
			if len(u.Views) > 0 {
				by = append(by, word(len(u.Views), "view ", "views ")+strings.Join(u.Views, ", "))
			}
			return Result{}, Errorf("Not deleted: the %s %s is used by %s. Take it off them first if the user wants it gone, or leave it.", c.Kind, c.ID, strings.Join(by, " and "))
		}
		if errors.Is(err, store.ErrConflict) {
			return Result{}, Errorf("Not deleted: the %s %s changed meanwhile; read it and try again if the user still wants it gone.", c.Kind, c.ID)
		}
		return Result{}, err
	}
	return Result{Text: fmt.Sprintf("Deleted %s %s. Undo turn brings it back.", c.Kind, c.ID), Notices: []chat.Notice{ConfigChip(w)}}, nil
}

// word is one or many by n.
func word(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// maxFilterBytes caps open_page's filter values as JSON: they go into
// the chip and the dock.
const maxFilterBytes = 4096

// filterValues checks open_page's values against the page's filters: a
// string, number, true/false or null each, fitting the filter's control
// where it says what fits. p is nil for a page that doesn't parse.
func filterValues(page string, p *view.Page, values map[string]any) error {
	controls := map[string]string{}
	if p != nil {
		for _, f := range p.Filters {
			controls[f.Param] = f.Control
		}
	}
	for _, k := range slices.Sorted(maps.Keys(values)) {
		v := values[k]
		switch x := v.(type) {
		case nil:
		case string:
			switch controls[k] {
			case "date":
				if _, rel := view.RelativeDate(x, time.Now()); !rel && !isoDate.MatchString(x) {
					return Errorf("The filter %s of %s is a date: give it as YYYY-MM-DD or as today, today-30d, start_of_month and the like, not %q.", k, page, x)
				}
			case "number":
				return Errorf("The filter %s of %s is a number: give it as a JSON number, not %q.", k, page, x)
			case "checkbox":
				return Errorf("The filter %s of %s is a checkbox: give true or false, not %q.", k, page, x)
			}
		case float64:
			if c := controls[k]; c == "date" || c == "checkbox" {
				return Errorf("The filter %s of %s is a %s, not a number.", k, page, c)
			}
		case bool:
			if c := controls[k]; c != "" && c != "checkbox" {
				return Errorf("The filter %s of %s is a %s, not true or false.", k, page, c)
			}
		default:
			return Errorf("The filter %s of %s takes one value: a string, a number, true, false or null, not a list or an object.", k, page)
		}
	}
	return nil
}

var isoDate = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
