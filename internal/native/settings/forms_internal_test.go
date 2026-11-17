package settings

import (
	"testing"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/native/settings/logic"
)

// A closed dialog's form is dropped with the key and placeholder values
// typed in it, and a late connect closes only its own dialog (TASK-068).
func TestClosedDialogsDropTheirForms(t *testing.T) {
	p := &modelsPage{}
	p.openAdd()
	add := p.add
	add.key, add.fields = "sk-test", map[string]string{"account_id": "abc"}
	p.closeForms()
	if p.add != add || add.key == "" {
		t.Fatal("an open dialog's form was dropped")
	}
	p.adding = false
	p.closeForms()
	if p.add != nil || add.key != "" || len(add.fields) != 0 {
		t.Errorf("after closing: form %v, key %q, fields %v", p.add, add.key, add.fields)
	}

	p.openReplace(&logic.Replace{Name: "gemini", Kind: "gemini"})
	old := p.replace
	old.key = "sk-old"
	p.replacing = false
	p.openReplace(&logic.Replace{Name: "gemini", Kind: "gemini"})
	if old.key != "" || p.replace == old {
		t.Error("a new key dialog kept the old form")
	}
	old.Connected(app.ConnectResult{})
	if !p.replacing {
		t.Error("the old form's connect closed the new dialog")
	}
	p.replace.Connected(app.ConnectResult{})
	if p.replacing {
		t.Error("the form's own connect didn't close its dialog")
	}
}

// Clear empties the key and the placeholder values.
func TestConnectFormClear(t *testing.T) {
	f := &ConnectForm{key: "k", fields: map[string]string{"account_id": "a"}, name: "n", url: "u"}
	fields := f.fields
	f.Clear()
	if f.key != "" || len(f.fields) != 0 || len(fields) != 0 {
		t.Errorf("after Clear: %q, %v, %v", f.key, f.fields, fields)
	}
}
