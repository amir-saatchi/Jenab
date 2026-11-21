package view

import (
	"bytes"
	"cmp"
	"encoding/json"
	"slices"
	"strings"

	"github.com/amir-saatchi/jenab/internal/store"
)

// The parsed configs, for the runtime (P3-05) and the page screen (P3-09).
// A config is parsed only after it validated, so these types hold what the
// schemas allow and no more; ParseView and ParsePage read a saved config
// without checking it again.

// View is a saved view (SPEC 5.1): the common fields, and Spec, the parsed
// form of its type's unit: *Table, *Chart, *Stat, *Form, or a registered
// unit's own type.
type View struct {
	Version     int            `json:"version"`
	ID          string         `json:"id"` // NFC (store.NormConfigID)
	Title       string         `json:"title"`
	Type        string         `json:"type"`
	Description string         `json:"description,omitempty"`
	Query       string         `json:"query,omitempty"`
	Params      map[string]any `json:"params,omitempty"` // string, int64, float64 or bool
	Filters     []Filter       `json:"filters,omitempty"`
	Actions     []Action       `json:"actions,omitempty"`
	Spec        any            `json:"-"`
}

// Filter is a control bound to a query parameter (SPEC 5.2).
type Filter struct {
	Param    string   `json:"param"`
	Control  string   `json:"control"` // text, number, date, select, checkbox
	Label    string   `json:"label,omitempty"`
	Default  any      `json:"default,omitempty"` // string, int64, float64 or bool
	Required bool     `json:"required,omitempty"`
	Options  *Options `json:"options,omitempty"`
}

// Options are a select's choices: a list, or a query that returns one
// column.
type Options struct {
	List  []any  // string, int64, float64 or bool
	Query string // "" for a list
}

func (o *Options) UnmarshalJSON(b []byte) error {
	if bytes.HasPrefix(bytes.TrimSpace(b), []byte("{")) {
		var q struct {
			Query string `json:"query"`
		}
		err := json.Unmarshal(b, &q)
		o.Query = q.Query
		return err
	}
	if bytes.HasPrefix(bytes.TrimSpace(b), []byte("[")) {
		return unmarshalNumbers(b, &o.List)
	}
	return nil // the schema reports it
}

func (o Options) MarshalJSON() ([]byte, error) {
	if o.Query != "" {
		return json.Marshal(map[string]string{"query": o.Query})
	}
	return json.Marshal(o.List)
}

// Action is a button of a view or of a page's header (SPEC 5.6, 5.9):
// one target of RunPipeline, OpenPage and OpenForm; views have only
// RunPipeline.
type Action struct {
	Label       string `json:"label"`
	RunPipeline string `json:"run_pipeline,omitempty"`
	OpenPage    string `json:"open_page,omitempty"`
	OpenForm    string `json:"open_form,omitempty"`
	Confirm     bool   `json:"confirm,omitempty"`
}

// Table is a table view's own fields (SPEC 5.3).
type Table struct {
	Columns     []Column    `json:"columns"`
	DefaultSort Sort        `json:"default_sort"`
	PageSize    int         `json:"page_size,omitempty"` // 0 for the default, 50
	RowsFrom    *RowsFrom   `json:"rows_from,omitempty"`
	RowActions  []RowAction `json:"row_actions,omitempty"`
}

// DefaultPageSize is a table's rows a page when page_size isn't set.
const DefaultPageSize = 50

// Column is a column of a table view.
type Column struct {
	Field         string         `json:"field"`
	Label         string         `json:"label,omitempty"`
	Format        string         `json:"format,omitempty"`
	FormatOptions *FormatOptions `json:"format_options,omitempty"`
	Align         string         `json:"align,omitempty"`
}

// FormatOptions are a format's options (SPEC 5.3).
type FormatOptions struct {
	Currency string            `json:"currency,omitempty"`
	Decimals *int              `json:"decimals,omitempty"`
	Tones    map[string]string `json:"tones,omitempty"` // value → tone (5.11)
}

// Sort is a table's first sort.
type Sort struct {
	Field     string `json:"field"`
	Direction string `json:"direction"` // asc or desc
}

// RowsFrom is the base table of a table's rows and its key column.
type RowsFrom struct {
	Table string `json:"table"`
	Key   string `json:"key"`
}

// RowAction is a button on each row: one of OpenForm, Set and Delete.
type RowAction struct {
	Label    string         `json:"label"`
	OpenForm string         `json:"open_form,omitempty"`
	Delete   bool           `json:"delete,omitempty"`
	Set      map[string]any `json:"set,omitempty"` // nil, string, int64, float64 or bool; "now()" and "today()" are the time
	Bulk     bool           `json:"bulk,omitempty"`
	Confirm  bool           `json:"confirm,omitempty"`
}

// Chart is a chart view's own fields (SPEC 5.4).
type Chart struct {
	ChartType string   `json:"chart_type"` // line, bar, scatter or pie
	X         Axis     `json:"x"`
	Y         []Series `json:"y"`
	SeriesBy  string   `json:"series_by,omitempty"`
}

// Axis is a chart's x.
type Axis struct {
	Field  string `json:"field"`
	Label  string `json:"label,omitempty"`
	Format string `json:"format,omitempty"`
}

// Series is one of a chart's y.
type Series struct {
	Field string `json:"field"`
	Label string `json:"label,omitempty"`
}

// Stat is a stat view's own fields (SPEC 5.10).
type Stat struct {
	Value   StatValue   `json:"value"`
	Change  *StatChange `json:"change,omitempty"`
	Caption string      `json:"caption,omitempty"`
}

// StatValue is the number a stat shows.
type StatValue struct {
	Field         string         `json:"field"`
	Format        string         `json:"format,omitempty"`
	FormatOptions *FormatOptions `json:"format_options,omitempty"`
}

// StatChange is a stat's change, shown green or red.
type StatChange struct {
	Field  string `json:"field"`
	Format string `json:"format,omitempty"`
}

// Form is a form view's own fields (SPEC 5.5).
type Form struct {
	Fields []FormField `json:"fields"`
	Submit Submit      `json:"submit"`
	Load   string      `json:"load,omitempty"`
}

// FormField is one of a form's fields.
type FormField struct {
	Field    string   `json:"field"`
	Label    string   `json:"label,omitempty"`
	Control  string   `json:"control"` // text, textarea, number, date, select, checkbox, file
	Required bool     `json:"required,omitempty"`
	Default  any      `json:"default,omitempty"`
	Options  *Options `json:"options,omitempty"`
	Prefix   string   `json:"prefix,omitempty"` // file: the bucket prefix uploads go under
}

// Submit is what a form's submit does.
type Submit struct {
	Action   string `json:"action"` // insert, update or run_pipeline
	Table    string `json:"table,omitempty"`
	Key      string `json:"key,omitempty"`
	Pipeline string `json:"pipeline,omitempty"`
}

// Page is a saved page (SPEC 5.9).
type Page struct {
	Version     int       `json:"version"`
	ID          string    `json:"id"` // NFC (store.NormConfigID)
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	Filters     []Filter  `json:"filters,omitempty"`
	Actions     []Action  `json:"actions,omitempty"`
	Rows        [][]Block `json:"-"`
}

// Block is one block of a page's row. Spec is its unit's parsed form:
// *ViewBlock, *HeadingBlock, *TextBlock, *CardBlock, *FilterBlock,
// *ButtonBlock, *ImageBlock, or a registered unit's own type. A container
// (a card) has its blocks in Rows.
type Block struct {
	Type string
	// Span is the block's columns of 12, with the default filled in: 12
	// divided by the blocks in its row.
	Span int
	Rows [][]Block
	Spec any
}

// ViewBlock shows a saved view.
type ViewBlock struct {
	View string `json:"view"` // NFC
}

// HeadingBlock is a section title.
type HeadingBlock struct {
	Heading string `json:"heading"`
}

// TextBlock is static Markdown.
type TextBlock struct {
	Text string `json:"text"`
}

// CardBlock is a titled frame; its blocks are the Block's Rows.
type CardBlock struct {
	Card string `json:"card"`
}

// FilterBlock shows a page filter in the page instead of in its header.
type FilterBlock struct {
	Filter string `json:"filter"` // the param of a page filter
}

// ButtonBlock is a button: one target of RunPipeline, OpenPage and
// OpenForm.
type ButtonBlock struct {
	Button      string `json:"button"`
	RunPipeline string `json:"run_pipeline,omitempty"`
	OpenPage    string `json:"open_page,omitempty"`
	OpenForm    string `json:"open_form,omitempty"`
	Tone        string `json:"tone,omitempty"`
	Confirm     bool   `json:"confirm,omitempty"`
}

// ImageBlock is a picture from the bucket.
type ImageBlock struct {
	Image string `json:"image"`
	Alt   string `json:"alt,omitempty"`
}

// decodeInto decodes the loaded value v into p through JSON, keeping
// whole numbers whole.
func decodeInto(v any, p any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return unmarshalNumbers(b, p)
}

// unmarshalNumbers is json.Unmarshal with numbers in any as int64 when
// they are whole and float64 otherwise, as the loader gives them.
func unmarshalNumbers(b []byte, p any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	err := d.Decode(p)
	fixNumbers(p)
	return err
}

// fixNumbers turns the json.Numbers in what p points to into int64 or
// float64. It walks the any values the config types hold: maps and lists
// of any, Default, Params, Set, Options.List.
func fixNumbers(p any) {
	switch p := p.(type) {
	case *any:
		*p = number(*p)
	case *[]any:
		for i := range *p {
			(*p)[i] = number((*p)[i])
		}
	case *map[string]any:
		for k, v := range *p {
			(*p)[k] = number(v)
		}
	case *View:
		fixMap(p.Params)
		fixFilters(p.Filters)
	case *Page:
		fixFilters(p.Filters)
	case *Table:
		for i := range p.RowActions {
			fixMap(p.RowActions[i].Set)
		}
	case *Form:
		for i := range p.Fields {
			p.Fields[i].Default = number(p.Fields[i].Default)
			fixOptions(p.Fields[i].Options)
		}
	}
}

func fixMap(m map[string]any) {
	for k, v := range m {
		m[k] = number(v)
	}
}

func fixFilters(fs []Filter) {
	for i := range fs {
		fs[i].Default = number(fs[i].Default)
		fixOptions(fs[i].Options)
	}
}

func fixOptions(o *Options) {
	if o == nil {
		return
	}
	for i := range o.List {
		o.List[i] = number(o.List[i])
	}
}

// number turns a json.Number into int64 or float64, and walks lists and
// maps.
func number(v any) any {
	switch v := v.(type) {
	case json.Number:
		if !strings.ContainsAny(string(v), ".eE") {
			if i, err := v.Int64(); err == nil {
				return i
			}
		}
		f, _ := v.Float64()
		return f
	case []any:
		for i := range v {
			v[i] = number(v[i])
		}
	case map[string]any:
		for k, x := range v {
			v[k] = number(x)
		}
	}
	return v
}

// ParseView reads a saved view without checking it: the runtime's view of
// a config that validated when it was saved. A config that doesn't load,
// or a type reg doesn't know, is an error; its Spec is then nil.
func ParseView(reg *Registry, src []byte) (*View, error) {
	d, err := load(src)
	if err != nil {
		return nil, err
	}
	return parseView(reg, d.value)
}

func parseView(reg *Registry, v any) (*View, error) {
	// The decode goes on past a value of the wrong type, so a config that
	// failed its schema still parses as far as it can for the Go checks.
	var out View
	err := decodeInto(v, &out)
	out.ID = store.NormConfigID(out.ID)
	u := reg.unit(Data, out.Type)
	if u == nil {
		return &out, &unknownType{out.Type}
	}
	spec, serr := u.decode(v)
	out.Spec = spec
	return &out, cmp.Or(err, serr)
}

type unknownType struct{ typ string }

func (e *unknownType) Error() string { return "view: unknown view type " + e.typ }

// ParsePage reads a saved page without checking it. Blocks of types reg
// doesn't know are left out.
func ParsePage(reg *Registry, src []byte) (*Page, error) {
	d, err := load(src)
	if err != nil {
		return nil, err
	}
	return parsePage(reg, d.value)
}

func parsePage(reg *Registry, v any) (*Page, error) {
	var out Page
	err := decodeInto(v, &out)
	out.ID = store.NormConfigID(out.ID)
	m, _ := v.(map[string]any)
	out.Rows = parseRows(reg, m["rows"])
	return &out, err
}

func parseRows(reg *Registry, v any) [][]Block {
	rows, _ := v.([]any)
	out := make([][]Block, 0, len(rows))
	for _, r := range rows {
		blocks, _ := r.([]any)
		row := make([]Block, 0, len(blocks))
		for _, b := range blocks {
			m, _ := b.(map[string]any)
			typ := blockType(reg, m)
			if len(typ) != 1 {
				continue
			}
			u := reg.unit(PageBlock, typ[0])
			blk := Block{Type: typ[0]}
			if s, ok := m["span"].(int64); ok {
				blk.Span = int(s)
			}
			spec, _ := u.decode(m)
			if vb, ok := spec.(*ViewBlock); ok {
				vb.View = store.NormConfigID(vb.View)
			}
			blk.Spec = spec
			if u.container {
				blk.Rows = parseRows(reg, m["rows"])
			}
			row = append(row, blk)
		}
		fillSpans(row)
		out = append(out, row)
	}
	return out
}

// fillSpans gives the blocks without a span what is left of the 12
// columns (SPEC 5.9).
func fillSpans(row []Block) {
	given := make([]int, len(row))
	for i, b := range row {
		given[i] = b.Span
	}
	for i, s := range shareSpans(given) {
		row[i].Span = s
	}
}

// shareSpans fills the zeros of given, the blocks without a span, with
// what the others leave of the 12 columns, shared out evenly; the first
// ones get one more when it doesn't divide. When nothing is left, each
// gets 1, which makes the row too wide: the validator says so.
func shareSpans(given []int) []int {
	out := slices.Clone(given)
	sum, free := 0, 0
	for _, s := range given {
		if s == 0 {
			free++
		}
		sum += s
	}
	if free == 0 {
		return out
	}
	rest := 12 - sum
	each, extra := max(rest/free, 1), max(rest%free, 0)
	if rest < free {
		extra = 0
	}
	for i, s := range out {
		if s == 0 {
			out[i] = each
			if extra > 0 {
				out[i]++
				extra--
			}
		}
	}
	return out
}

// blockType returns the registered page-block types whose key m has:
// one for a block, none or several for a mistake.
func blockType(reg *Registry, m map[string]any) []string {
	var out []string
	for _, t := range reg.Types(PageBlock) {
		if _, ok := m[t]; ok {
			out = append(out, t)
		}
	}
	return out
}
