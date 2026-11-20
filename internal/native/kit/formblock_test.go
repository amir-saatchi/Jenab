package kit

import (
	"errors"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// formTest builds a form block between two buttons, in a scroll, and
// keeps what it submitted.
type formTest struct {
	spec *FormSpec
	st   FormState
	got  []map[string]FormValue
}

func (ft *formTest) view(c *Context) {
	Scroll(c).Height(580).Children(func() {
		between(c, func() {
			Column(c).Width(420).Children(func() {
				if r := FormBlock(c, "New keyword", ft.spec, ft.st); r.Submitted {
					ft.got = append(ft.got, r.Values)
				}
			})
		})
	})
}

// submitted is the values the form submitted last, failing unless it
// submitted once since the last call.
func (ft *formTest) submitted(t *testing.T) map[string]FormValue {
	t.Helper()
	if len(ft.got) != 1 {
		t.Fatalf("the form submitted %d times", len(ft.got))
	}
	v := ft.got[0]
	ft.got = nil
	return v
}

func pressEnter(tt *ui.Tester) {
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
}

var allControls = []FormField{
	{Name: "keyword", Label: "Keyword", Control: FormText, Required: true},
	{Name: "notes", Label: persian, Control: FormTextarea, Description: "What it is for"},
	{Name: "weight", Label: "Weight", Control: FormNumber},
	{Name: "due", Label: "Due", Control: FormDate},
	{Name: "coin", Label: "Coin", Control: FormSelect, Options: []PickerOption{{Value: "btc", Label: "BTC"}, {Value: "eth", Label: "اتریوم"}}},
	{Name: "active", Label: "Active", Control: FormCheckbox, Description: "Watch it daily"},
}

func TestFormBlockControls(t *testing.T) {
	ft := &formTest{}
	themes(t, ft.view, func(t *testing.T, tt *ui.Tester) {
		ft.spec, ft.st, ft.got = &FormSpec{Fields: allControls, Submit: "Add"}, FormState{}, nil
		tt.Frame()
		tt.UseScreenReader()
		shows(t, tt, "Keyword", persian, "What it is for", "Weight", "Due", "Coin", "Active", "Watch it daily", "Add")
		if !HasRole(tt, "group", "New keyword") {
			t.Error("the form is not a group named by its label")
		}

		tabTo(t, tt, "Keyword")
		tt.Type("bitcoin halving")
		tabTo(t, tt, persian)
		tt.Type(persian)
		tabTo(t, tt, "Weight")
		tt.Type("۱٬۲۳۴٫۵") // Persian digits, separators and decimal point
		tabTo(t, tt, "Choose a date, Due")
		pressEnter(tt) // chooses today
		tabTo(t, tt, "Coin")
		tt.Key(0, ui.KeyDown) // opens the options, none chosen
		tt.Frame()
		tt.Key(0, ui.KeyDown)
		tt.Key(0, ui.KeyDown)
		pressEnter(tt)
		tabTo(t, tt, "Active")
		tt.Key(0, ui.KeySpace)
		tt.Frame()
		shows(t, tt, persian, "اتریوم")
		tabTo(t, tt, "Add")
		pressEnter(tt)

		v := ft.submitted(t)
		if v["keyword"].Text != "bitcoin halving" || v["notes"].Text != persian {
			t.Errorf("the texts: %+v, %+v", v["keyword"], v["notes"])
		}
		if n := v["weight"].Number; n == nil || *n != 1234.5 {
			t.Errorf("the number: %+v", v["weight"])
		}
		today := tt.Now()
		if d := v["due"].Date; d == nil || d.Year() != today.Year() || d.YearDay() != today.YearDay() {
			t.Errorf("the date: %v, today %v", d, today)
		}
		if v["coin"].Text != "eth" || !v["active"].Checked {
			t.Errorf("the select and the check box: %+v, %+v", v["coin"], v["active"])
		}
		// The date submitted is a copy: changing the field doesn't change it.
		submittedDay := *v["due"].Date
		tabTo(t, tt, "Due")
		shown := focusable(tt, "Due").Value
		pressEnter(tt) // opens the calendar
		tt.Key(0, ui.KeyRight)
		pressEnter(tt) // chooses the next day
		if focusable(tt, "Due").Value == shown {
			t.Error("the arrow did not change the date")
		}
		if !v["due"].Date.Equal(submittedDay) {
			t.Error("the date submitted changed with the field")
		}

		// Enter in a one-line field submits too.
		tabTo(t, tt, "Keyword")
		pressEnter(tt)
		ft.submitted(t)
	})
}

func TestFormBlockRequired(t *testing.T) {
	ft := &formTest{}
	fields := []FormField{
		{Name: "keyword", Label: "Keyword", Control: FormText, Required: true},
		{Name: "weight", Label: "Weight", Control: FormNumber},
		{Name: "due", Label: "Due", Control: FormDate, Required: true},
		{Name: "agree", Label: "I agree", Control: FormCheckbox, Required: true},
	}
	themes(t, ft.view, func(t *testing.T, tt *ui.Tester) {
		ft.spec, ft.st, ft.got = &FormSpec{Fields: fields}, FormState{}, nil
		tt.Frame()
		tt.UseScreenReader()
		tabTo(t, tt, "Weight")
		tt.Type("twelve")
		tabTo(t, tt, "Save")
		pressEnter(tt)
		tt.Frame()
		if len(ft.got) != 0 {
			t.Fatal("a form with required fields left empty submitted")
		}
		shows(t, tt, "Required", "Enter a number")
		if !tt.Focused("Keyword") {
			t.Error("the first field not filled did not take the focus")
		}
		if d := focusable(tt, "Keyword").Description; d != "Required" {
			t.Errorf("assistive technology reads %q with the field", d)
		}
		if a := focusable(tt, "I agree"); !a.Invalid {
			t.Error("the check box left unchecked is not marked")
		}

		tt.Type(persian) // a change clears the field's mark
		tt.Frame()
		tabTo(t, tt, "Weight")
		tt.Key(ui.Ctrl, ui.KeyA)
		tt.Type("12")
		tabTo(t, tt, "Choose a date, Due")
		pressEnter(tt)
		tabTo(t, tt, "I agree")
		tt.Key(0, ui.KeySpace)
		tabTo(t, tt, "Save")
		pressEnter(tt)
		v := ft.submitted(t)
		if v["keyword"].Text != persian || *v["weight"].Number != 12 || v["due"].Date == nil || !v["agree"].Checked {
			t.Errorf("submitted %+v", v)
		}
		// An optional date can be cleared; a required one can't.
		if _, ok := tt.Access("Clear Due"); ok {
			t.Error("a required date has a button clearing it")
		}
	})
}

func TestFormBlockWriteErrorsAndBusy(t *testing.T) {
	ft := &formTest{}
	fields := []FormField{
		{Name: "keyword", Label: "Keyword", Control: FormText},
		{Name: "coin", Label: "Coin", Control: FormSelect, Options: []PickerOption{{Value: "btc", Label: "BTC"}}},
	}
	themes(t, ft.view, func(t *testing.T, tt *ui.Tester) {
		ft.spec, ft.got = &FormSpec{Fields: fields, Submit: "Add"}, nil
		ft.st = FormState{Busy: true}
		tt.Frame()
		tt.UseScreenReader()
		shows(t, tt, "Saving…")
		if a, ok := tt.Access("Add"); !ok || !a.Busy || !a.Disabled {
			t.Errorf("the busy button: %+v", a)
		}
		tt.Key(0, ui.KeyTab)
		tt.Key(0, ui.KeyTab)
		if tt.Focused("Keyword") || tt.Focused("Add") {
			t.Error("a busy form's controls take the focus")
		}

		ft.st = FormState{
			Errors:  map[string]string{"keyword": "a row with this value already exists", "owner": "no such user"},
			Problem: "already running since 08:00 " + persian,
		}
		tt.Frame()
		shows(t, tt, "a row with this value already exists", "already running since 08:00 "+persian+"\nowner: no such user")

		// A new submit's write on its way hides the last one's errors.
		busy := ft.st
		busy.Busy = true
		ft.st = busy
		tt.Frame()
		if Shows(tt, "a row with this value already exists") || Shows(tt, "already running since 08:00 "+persian+"\nowner: no such user") {
			t.Error("the last write's errors show while the form saves again")
		}
		busy.Busy = false
		ft.st = busy
		tt.Frame()
		if d := focusable(tt, "Keyword").Description; d != "a row with this value already exists" {
			t.Errorf("assistive technology reads %q with the field", d)
		}
		tabTo(t, tt, "Keyword")
		tt.Type("x")
		tt.Frame()
		if Shows(tt, "a row with this value already exists") {
			t.Error("the write's error stayed as the field changed")
		}
		if len(ft.got) != 0 {
			t.Error("the form submitted unasked")
		}
	})
}

func TestFormBlockUpdate(t *testing.T) {
	due := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	n := 2.5
	ft := &formTest{}
	themes(t, ft.view, func(t *testing.T, tt *ui.Tester) {
		ft.st, ft.got = FormState{}, nil
		ft.spec = &FormSpec{Fields: append(allControls, FormField{Name: "file", Label: "File", Control: FormFile}), Values: map[string]FormValue{
			"keyword": {Text: persian},
			"notes":   {Text: "line one\nline two"},
			"weight":  {Number: &n},
			"due":     {Date: &due},
			"coin":    {Text: "eth"},
			"active":  {Checked: true},
			"file":    {Text: "uploads/report.pdf"},
		}}
		tt.Frame()
		shows(t, tt, persian, "line one\nline two", "2.5", "اتریوم", "report.pdf")
		if a := focusable(tt, "Due"); a.Value == "" {
			t.Error("the date is not shown")
		}
		if a := focusable(tt, "Active"); !a.Checked {
			t.Error("the check box is not checked")
		}
		tabTo(t, tt, "Save")
		pressEnter(tt)
		v := ft.submitted(t)
		if v["keyword"].Text != persian || !v["due"].Date.Equal(due) || v["file"].Text != "uploads/report.pdf" {
			t.Errorf("an unchanged update form submitted %+v", v)
		}

		// A new spec fills the form anew.
		ft.spec = &FormSpec{Fields: allControls, Values: map[string]FormValue{"keyword": {Text: "next row"}}}
		tt.Frame()
		shows(t, tt, "next row")
		if Shows(tt, "اتریوم") {
			t.Error("the last row's values stayed")
		}
	})
}

func TestFormBlockFile(t *testing.T) {
	var progress func(float64)
	var done func(string, error)
	var picked []string
	ft := &formTest{}
	themes(t, ft.view, func(t *testing.T, tt *ui.Tester) {
		ft.st, ft.got, picked = FormState{}, nil, nil
		ft.spec = &FormSpec{
			Fields: []FormField{{Name: "doc", Label: "Document", Control: FormFile, Required: true}},
			Pick: func(field string, chosen func(string)) {
				picked = append(picked, field)
				chosen("/tmp/scratch/گزارش.pdf")
			},
			Upload: func(field, path string, p func(float64), d func(string, error)) {
				progress, done = p, d
			},
		}
		tt.Frame()
		tt.UseScreenReader()
		shows(t, tt, "Document", "Choose file", "or drop a file here")
		tabTo(t, tt, "Choose file, Document")
		pressEnter(tt)
		if len(picked) != 1 || picked[0] != "doc" {
			t.Fatalf("picked %q", picked)
		}
		progress(0.4)
		tt.Frame()
		shows(t, tt, "گزارش.pdf", "40%")
		if a, ok := tt.Access("Uploading گزارش.pdf"); !ok || a.Now != 0.4 {
			t.Errorf("no progress bar read out: %+v", a)
		}
		tabTo(t, tt, "Save")
		pressEnter(tt)
		tt.Frame()
		shows(t, tt, "Wait for the upload to finish")
		if len(ft.got) != 0 {
			t.Fatal("the form submitted with a file uploading")
		}

		done("", errors.New("disk full"))
		tt.Frame()
		shows(t, tt, "disk full")
		failed := done

		// A file dropped on the field uploads in its place.
		r, ok := tt.Find("or drop a file here")
		if !ok {
			t.Fatal("no place to drop a file")
		}
		if !tt.DropFiles(r.X+2, r.Y+2, "/tmp/scratch/notes.txt") {
			t.Fatal("the drop was not taken")
		}
		tt.Frame()
		shows(t, tt, "notes.txt", "0%")
		done("uploads/notes.txt", nil)
		failed("uploads/other.txt", nil) // the upload replaced: no change
		tt.Frame()
		if Shows(tt, "%") {
			t.Error("the progress stayed after the upload")
		}
		tabTo(t, tt, "Save")
		pressEnter(tt)
		if v := ft.submitted(t); v["doc"].Text != "uploads/notes.txt" {
			t.Errorf("submitted the file %q", v["doc"].Text)
		}

		// While the form saves, a drop is ignored.
		ft.st = FormState{Busy: true}
		tt.Frame()
		if r, ok := tt.Find("or drop a file here"); ok {
			tt.DropFiles(r.X+2, r.Y+2, "/tmp/scratch/late.txt")
		}
		tt.Frame()
		if Shows(tt, "late.txt") {
			t.Error("a busy form took a dropped file")
		}
		ft.st = FormState{}
		tt.Frame()

		// Removing the file empties the field; an upload replaced lands nowhere.
		tabTo(t, tt, "Choose another, Document")
		pressEnter(tt)
		old := done
		if err := tt.Click("Remove گزارش.pdf"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		old("uploads/late.pdf", nil)
		tt.Frame()
		tabTo(t, tt, "Save")
		pressEnter(tt)
		tt.Frame()
		shows(t, tt, "Required")
		if len(ft.got) != 0 {
			t.Error("a required file field left empty submitted")
		}
	})
}

func TestFormBlockStates(t *testing.T) {
	ft := &formTest{}
	themes(t, ft.view, func(t *testing.T, tt *ui.Tester) {
		ft.spec = &FormSpec{Fields: allControls}
		ft.st = FormState{BlockState: BlockState{Loading: true}}
		tt.Frame()
		tt.UseScreenReader()
		shows(t, tt, "Loading New keyword")
		passesOver(t, tt)

		ft.st = FormState{BlockState: BlockState{Error: "no such table: " + persian}}
		tt.Frame()
		shows(t, tt, "Can't show New keyword", "no such table: "+persian)
		passesOver(t, tt)

		ft.st = FormState{BlockState: BlockState{Notice: "Edit the row " + persian}}
		tt.Frame()
		shows(t, tt, "Edit the row "+persian, "Keyword")

		ft.st = FormState{Missing: true}
		tt.Frame()
		shows(t, tt, "Nothing to edit", "This row is no longer there.")
		passesOver(t, tt)
	})
}

// A whole number past 2^53 is kept exactly, and an optional select goes
// back to no choice.
func TestFormBlockExactNumberAndNoChoice(t *testing.T) {
	ft := &formTest{}
	fields := []FormField{
		{Name: "id", Label: "ID", Control: FormNumber},
		{Name: "coin", Label: "Coin", Control: FormSelect, Options: []PickerOption{{Value: "btc", Label: "BTC"}}},
		{Name: "kind", Label: "Kind", Control: FormSelect, Required: true, Options: []PickerOption{{Value: "a", Label: "A"}}},
	}
	big := int64(9007199254740993)
	themes(t, ft.view, func(t *testing.T, tt *ui.Tester) {
		ft.st, ft.got = FormState{}, nil
		ft.spec = &FormSpec{Fields: fields, Values: map[string]FormValue{"id": {Int: &big}, "coin": {Text: "btc"}, "kind": {Text: "a"}}}
		tt.Frame()
		tt.UseScreenReader()
		shows(t, tt, "9007199254740993", "BTC")
		// A required select has no empty choice.
		tabTo(t, tt, "Kind")
		tt.Key(0, ui.KeyDown)
		tt.Frame()
		if Shows(tt, "None") {
			t.Error("a required select offers no choice")
		}
		tt.Key(0, ui.KeyEscape)
		tt.Frame()

		tabTo(t, tt, "Coin")
		tt.Key(0, ui.KeyDown) // opens the options, BTC chosen
		tt.Frame()
		tt.Key(0, ui.KeyHome)
		pressEnter(tt)
		shows(t, tt, "None")
		tabTo(t, tt, "Save")
		pressEnter(tt)
		v := ft.submitted(t)
		if i := v["id"].Int; i == nil || *i != big || v["id"].Text != "9007199254740993" {
			t.Errorf("the number: %+v", v["id"])
		}
		if v["coin"].Text != "" {
			t.Errorf("the optional select: %q", v["coin"].Text)
		}
		// A typed number with a fraction has no Int; a whole one in Persian
		// digits does.
		tabTo(t, tt, "ID")
		tt.Key(ui.Ctrl, ui.KeyA)
		tt.Type("2.5")
		pressEnter(tt)
		if v := ft.submitted(t); v["id"].Int != nil || *v["id"].Number != 2.5 {
			t.Errorf("2.5 read as %+v", v["id"])
		}
		tt.Key(ui.Ctrl, ui.KeyA)
		tt.Type("۱۲٬۰۰۰")
		pressEnter(tt)
		if v := ft.submitted(t); v["id"].Int == nil || *v["id"].Int != 12000 {
			t.Errorf("۱۲٬۰۰۰ read as %+v", v["id"])
		}
	})
}

func TestReadDecimal(t *testing.T) {
	for s, want := range map[string]float64{"12": 12, " -3.5 ": -3.5, "1,234.5": 1234.5, "۱۲٫۵": 12.5, "١٢": 12} {
		if got, ok := readDecimal(s); !ok || got != want {
			t.Errorf("readDecimal(%q) = %v, %v", s, got, ok)
		}
	}
	for _, s := range []string{"", "twelve", "1e999", "NaN"} {
		if _, ok := readDecimal(s); ok {
			t.Errorf("readDecimal(%q) read a number", s)
		}
	}
	for s, want := range map[string]int64{"9007199254740993": 9007199254740993, "-۱۲": -12, "1,000": 1000} {
		if got, ok := readWhole(s); !ok || got != want {
			t.Errorf("readWhole(%q) = %v, %v", s, got, ok)
		}
	}
	for _, s := range []string{"2.5", "99999999999999999999", ""} {
		if _, ok := readWhole(s); ok {
			t.Errorf("readWhole(%q) read a whole number", s)
		}
	}
}
