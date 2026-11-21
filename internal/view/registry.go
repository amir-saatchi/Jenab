package view

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// The block registry (SPEC 5.10, R-47). Each block type is one unit: its
// JSON Schema, its Go validator, and a slot for its kit component. A new
// block type is added by registering one unit; the loader, the page checks
// and the tools find it in the registry and change nothing.
//
// The package imports no UI: the native layer puts each unit's component
// in its slot (SetComponent), so the tools that validate configs stay
// headless.

// Kind is what a unit is: a data block, a saved view with a type (table,
// chart, stat, form), or a page block, a key in a page's row (view,
// heading, text, card, filter, button, image).
type Kind int

const (
	Data Kind = iota + 1
	PageBlock
)

func (k Kind) String() string {
	switch k {
	case Data:
		return "data"
	case PageBlock:
		return "page"
	}
	return fmt.Sprintf("Kind(%d)", int(k))
}

// Unit is one block type. T is its parsed form: the fields of its schema
// other than the common ones (5.1 for a view, span and rows for a page
// block), decoded from JSON.
type Unit[T any] struct {
	// Type is a view's type (type: table), or the key that makes an
	// object in a page's row this block ({ heading: "Prices" }).
	Type string
	Kind Kind
	// Schema is the unit's JSON Schema (2020-12). It may $ref DefsURL. A
	// data unit's schema checks the whole view, with the common fields; a
	// page unit's checks one block, span too.
	Schema []byte
	// Container: a page block with rows of blocks inside it, on their own
	// 12-column grid, as a card. A container can't be inside a container.
	Container bool
	// Check is the unit's Go validator. It runs after the schema passed,
	// with spec decoded, and reports through c.
	Check func(c *Check, spec *T)
	// Component is the unit's kit component, set by the native layer.
	Component any
}

// unit is a registered Unit with its type erased.
type unit struct {
	typ       string
	kind      Kind
	schema    *jsonschema.Schema
	container bool
	fields    []string // a page block's other fields, from its schema
	decode    func(v any) (any, error)
	check     func(c *Check, spec any)

	mu        sync.RWMutex
	component any
}

// Registry holds the units. Its methods are safe for concurrent use.
type Registry struct {
	mu    sync.RWMutex
	units map[Kind]map[string]*unit
}

// NewRegistry returns an empty registry. Default holds the built-in units.
func NewRegistry() *Registry {
	return &Registry{units: map[Kind]map[string]*unit{Data: {}, PageBlock: {}}}
}

// Register compiles u's schema and adds u to r. A type registered twice
// for its kind, a schema that doesn't compile, or a missing Check is an
// error.
func Register[T any](r *Registry, u Unit[T]) error {
	if u.Type == "" {
		return fmt.Errorf("view: a unit needs a type")
	}
	if u.Kind != Data && u.Kind != PageBlock {
		return fmt.Errorf("view: unit %s: kind must be Data or PageBlock", u.Type)
	}
	if u.Container && u.Kind != PageBlock {
		return fmt.Errorf("view: unit %s: only a page block can be a container", u.Type)
	}
	s, err := compileSchema(schemaBase+u.Kind.String()+"-"+u.Type+".json", u.Schema)
	if err != nil {
		return fmt.Errorf("view: unit %s: %w", u.Type, err)
	}
	var top struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	_ = json.Unmarshal(u.Schema, &top) // it compiled
	var fields []string
	for f := range top.Properties {
		if f != u.Type {
			fields = append(fields, f)
		}
	}
	slices.Sort(fields)
	check := u.Check
	x := &unit{
		typ:       u.Type,
		kind:      u.Kind,
		schema:    s,
		container: u.Container,
		fields:    fields,
		decode: func(v any) (any, error) {
			var spec T
			err := decodeInto(v, &spec)
			return &spec, err
		},
		check: func(c *Check, spec any) {
			if check != nil {
				check(c, spec.(*T))
			}
		},
		component: u.Component,
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.units[u.Kind][u.Type]; dup {
		return fmt.Errorf("view: a %s unit %s is registered already", u.Kind, u.Type)
	}
	// A page block is known by its key, so its type can't be a field of a
	// block, and its fields can't be a block's type.
	if u.Kind == PageBlock {
		if u.Type == "span" || u.Type == "rows" {
			return fmt.Errorf("view: unit %s: every block may have %s, so it can't be a block type", u.Type, u.Type)
		}
		for _, o := range r.units[PageBlock] {
			if slices.Contains(o.fields, u.Type) {
				return fmt.Errorf("view: unit %s: the %s block has a field %s, so it can't be a block type", u.Type, o.typ, u.Type)
			}
			if slices.Contains(fields, o.typ) {
				return fmt.Errorf("view: unit %s: %s is a block type, so it can't be a field", u.Type, o.typ)
			}
		}
	}
	r.units[u.Kind][u.Type] = x
	return nil
}

// MustRegister is Register that panics on an error, for units registered
// at start-up.
func MustRegister[T any](r *Registry, u Unit[T]) {
	if err := Register(r, u); err != nil {
		panic(err)
	}
}

func (r *Registry) unit(kind Kind, typ string) *unit {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.units[kind][typ]
}

// Types lists the registered types of kind, sorted.
func (r *Registry) Types(kind Kind) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return slices.Sorted(maps.Keys(r.units[kind]))
}

// Has reports whether kind has the type.
func (r *Registry) Has(kind Kind, typ string) bool { return r.unit(kind, typ) != nil }

// Container reports whether the page block typ holds rows of blocks.
func (r *Registry) Container(typ string) bool {
	u := r.unit(PageBlock, typ)
	return u != nil && u.container
}

// Component returns the kit component of a unit, nil when none is set.
func (r *Registry) Component(kind Kind, typ string) any {
	u := r.unit(kind, typ)
	if u == nil {
		return nil
	}
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.component
}

// SetComponent puts c in the slot of the unit, for the native layer.
func (r *Registry) SetComponent(kind Kind, typ string, c any) error {
	u := r.unit(kind, typ)
	if u == nil {
		return fmt.Errorf("view: no %s unit %s", kind, typ)
	}
	u.mu.Lock()
	u.component = c
	u.mu.Unlock()
	return nil
}

// Default is the registry of the built-in units: table, chart, stat and
// form views, and view, heading, text, card, filter, button and image
// blocks.
var Default = Builtin()

// Builtin returns a new registry with the built-in units, for a caller
// that adds units of its own without changing Default.
func Builtin() *Registry {
	r := NewRegistry()
	registerData(r)
	registerPage(r)
	return r
}
