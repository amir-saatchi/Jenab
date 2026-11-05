package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveRecorded(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "store.json"))
	if err != nil {
		t.Fatal(err)
	}
	s := &Session{store: st, Win: Window{AppName: "CalculatorApp.exe"}, model: "test", record: true}
	for _, n := range []string{"Seven", "Clear", "Four", "Eight", "Two", "Seven", "Plus", "Nine", "Three", "Five", "Six", "Equals"} {
		s.did(Step{Do: "click", Role: "button", Name: n})
	}
	out := s.saveRecorded([]byte(`{"name":"add_two","description":"Adds","params":["a=4827","b=9356"],"log":[{"role":"statictext","name":"Display is"}]}`))
	m := st.macro("CalculatorApp.exe", "add_two")
	if m == nil {
		t.Fatal(out)
	}
	var got []string
	for _, x := range m.Steps {
		got = append(got, x.String())
	}
	t.Log(out)
	want := []string{"click button \"Seven\"", "click button \"Clear\"", "click_each", "click button \"Plus\"", "click_each", "click button \"Equals\"", "log"}
	for i, w := range want {
		if !strings.HasPrefix(got[i], w) {
			t.Fatalf("step %d: %q, want %q", i+1, got[i], w)
		}
	}
	// skip the stray Seven and Clear
	out = s.saveRecorded([]byte(`{"name":"add_two","description":"Adds","params":["a=4827","b=9356"],"log":[{"role":"statictext","name":"Display is"}],"skip":[1,2]}`))
	if m := st.macro("CalculatorApp.exe", "add_two"); len(m.Steps) != 5 || m.Version != 2 {
		t.Fatal(out)
	}
	s2 := &Session{store: st, Win: Window{AppName: "spike040.exe"}, model: "test", record: true}
	s2.did(Step{Do: "set_value", Role: "textfield", Text: "wrong"})
	s2.did(Step{Do: "set_value", Role: "textfield", Text: "The train leaves at 9:15."})
	out = s2.saveRecorded([]byte(`{"name":"write","description":"Writes","params":["text=The train leaves at 9:15."],"log":[{"role":"textfield"}]}`))
	if m := st.macro("spike040.exe", "write"); m == nil || len(m.Steps) != 2 || m.Steps[0].Text != "{text}" {
		t.Fatal(out)
	}
	if out := s2.saveRecorded([]byte(`{"name":"write2","description":"Writes","params":["text=nothing like it"],"log":[{"role":"textfield"}]}`)); !strings.HasPrefix(out, "not saved") {
		t.Fatal(out)
	}
}
