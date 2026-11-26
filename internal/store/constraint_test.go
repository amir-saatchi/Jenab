package store

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
)

// A broken constraint is a *ConstraintError naming its kind, table and
// columns, so a form puts it on its field; the error text and the errors
// it wraps stay as they were.
func TestConstraintError(t *testing.T) {
	ctx := context.Background()
	p, _ := openTestProject(t)
	mustExec(t, p.DB, `CREATE TABLE owners(id INTEGER PRIMARY KEY)`)
	mustExec(t, p.DB, `INSERT INTO owners VALUES (1)`)
	mustExec(t, p.DB, `CREATE TABLE kw(id INTEGER PRIMARY KEY, word TEXT NOT NULL UNIQUE, a TEXT, b TEXT,
		weight INTEGER CHECK (weight >= 0), owner INTEGER REFERENCES owners(id), UNIQUE (a, b))`)
	mustExec(t, p.DB, `CREATE TABLE strict_t(id INTEGER PRIMARY KEY, n INTEGER) STRICT`)
	insert := func(tab string, r Row) error {
		_, err := p.Insert(ctx, limit.Interactive, id.SourceUser, tab, []Row{r}, ConflictFail)
		return err
	}
	if err := insert("kw", Row{"id": 1, "word": "btc", "a": "x", "b": "y"}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name    string
		tab     string
		row     Row
		kind    string
		cols    string
		wrapped error
	}{
		{"unique", "kw", Row{"word": "btc"}, ConstraintUnique, "[word]", ErrNotUnique},
		{"unique pair", "kw", Row{"word": "eth", "a": "x", "b": "y"}, ConstraintUnique, "[a b]", ErrNotUnique},
		{"not null", "kw", Row{"a": "z"}, ConstraintNotNull, "[word]", ErrInvalid},
		{"check", "kw", Row{"word": "eth", "weight": -1}, ConstraintCheck, "[]", ErrInvalid},
		{"foreign key", "kw", Row{"word": "eth", "owner": 9}, ConstraintForeignKey, "[]", ErrInvalid},
		{"datatype", "strict_t", Row{"n": "abc"}, ConstraintDatatype, "[n]", ErrInvalid},
	} {
		err := insert(c.tab, c.row)
		var ce *ConstraintError
		if !errors.As(err, &ce) || ce.Kind != c.kind || fmt.Sprint(ce.Columns) != c.cols || !errors.Is(err, c.wrapped) || !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v (%+v)", c.name, err, ce)
			continue
		}
		if c.kind != ConstraintForeignKey && c.kind != ConstraintCheck && ce.Table != c.tab {
			t.Errorf("%s: table %q", c.name, ce.Table)
		}
		if c.kind == ConstraintCheck && ce.Name != "weight >= 0" {
			t.Errorf("check: name %q", ce.Name)
		}
	}
	// An update that breaks one is the same.
	if err := insert("kw", Row{"id": 2, "word": "eth"}); err != nil {
		t.Fatal(err)
	}
	_, err := p.Update(ctx, limit.Interactive, id.SourceUser, UpdateReq{Table: "kw", Set: Row{"word": "btc"}, Keys: []Row{{"id": 2}}})
	var ce *ConstraintError
	if !errors.As(err, &ce) || ce.Kind != ConstraintUnique || fmt.Sprint(ce.Columns) != "[word]" {
		t.Errorf("update: %v", err)
	}
	// A missing file in an object_ref column names its key.
	mustExec(t, p.DB, `INSERT INTO _jenab_columns VALUES ('kw', 'a', 'object_ref')`)
	err = insert("kw", Row{"word": "doc", "a": "files/gone.pdf"})
	if key, ok := MissingObject(err); !ok || key != "files/gone.pdf" {
		t.Errorf("missing object: %q %v (%v)", key, ok, err)
	}
	if _, ok := MissingObject(errors.New("other")); ok {
		t.Error("MissingObject of another error")
	}
}
