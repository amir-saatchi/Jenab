package kit

import (
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// FormControl is how a form field is edited (SPEC 5.5).
type FormControl string

const (
	FormText     FormControl = "text"
	FormTextarea FormControl = "textarea"
	FormNumber   FormControl = "number"
	FormDate     FormControl = "date"
	FormSelect   FormControl = "select"
	FormCheckbox FormControl = "checkbox"
	FormFile     FormControl = "file"
)

// FormField is a field of a form block: Name is its key in the values,
// Label names it, Options are a select's choices, and Description shows
// under it. A Required field must be filled, and a required check box
// checked, before the form submits.
type FormField struct {
	Name        string
	Label       string
	Control     FormControl
	Required    bool
	Options     []PickerOption
	Description string
}

// FormValue is a field's value, in plain Go: Text for text, textarea and
// select, a number as typed, and a file's bucket key once it is uploaded;
// Number the number read from it as the form submits (nil for none); Int
// the same number exactly, when it is a whole one that fits, since a
// float64 rounds those past 2^53; Date a date (nil for none); Checked a
// check box.
//
// Text is the exact value of a number as typed. To fill a number field,
// give Text, Int or Number, in that order of preference.
type FormValue struct {
	Text    string
	Number  *float64
	Int     *int64
	Date    *time.Time
	Checked bool
}

// FormSpec is what a form block shows: its fields, the submit button's
// text ("Save" when ""), and the values to fill the fields with, as the
// row an update form's load reads (nil for an empty form).
//
// A file field uploads what is dropped on it, or what Pick chooses
// (nil for no button to choose): Upload stores the file at path and
// reports its progress, from 0 to 1, and then its bucket key or why it
// failed. Pick's chosen and Upload's progress and done must be called on
// the UI thread, as the stores' changes are (state.Post).
//
// The form keeps what is typed while it is given the same *FormSpec:
// give it a new one to fill it anew, as with the next row, or empty
// after an insert.
type FormSpec struct {
	Fields []FormField
	Submit string
	Values map[string]FormValue
	Pick   func(field string, chosen func(path string))
	Upload func(field, path string, progress func(float64), done func(key string, err error))
}

// FormState is how far a form block is: BlockState for an update form's
// row on its way, an error in place of the form or a notice above it, as
// for the other data blocks (BlockBody); Missing when its row
// is no longer there; Busy while its submit's write is on its way;
// Errors the write's errors, on their fields by name, as "a row with this
// value already exists" (one under a name that is no field's shows above
// the button); and Problem the write's error on no field, as "already
// running since 08:00". While Busy the last write's errors are hidden,
// as they are about to be replaced.
type FormState struct {
	BlockState
	Missing bool
	Busy    bool
	Errors  map[string]string
	Problem string
}

// FormResult is what a form block did in this frame: Submitted with the
// values of its fields, by name, once they are filled as required.
type FormResult struct {
	Element
	Submitted bool
	Values    map[string]FormValue
}

// formState is what a form block keeps between frames.
type formState struct {
	spec   *FormSpec
	fields map[string]*fieldState
	// focus names the field to take the focus in the next frame, as the
	// first one not filled as required when the form is submitted.
	focus string
}

// fieldState is a field as it is edited.
type fieldState struct {
	text    string
	date    *time.Time
	checked bool
	// problem is the form's own error on the field, as "Required".
	problem string
	// edited is set as the field changes after a submit, which hides the
	// write's error on it.
	edited bool
	file   fileState
}

// fileState is a file field's upload: the file's name, its progress
// while uploading, and why it failed. uploads counts them, so the result
// of one replaced doesn't land.
type fileState struct {
	name      string
	uploading bool
	progress  float64
	err       string
	uploads   int
}

// FormBlock is a form block of a page (SPEC 5.5): a field for each of
// spec's fields, its label above it, and the submit button. Enter in a
// one-line field submits too. A field left empty that is required, or a
// number that isn't one, is marked with why, and the form doesn't submit
// but focuses the first; a file still uploading holds the submit. The
// write's errors show on their fields until they change. While st is
// Busy the fields and the button are disabled, the button showing
// "Saving…". label names the form.
func FormBlock(c *Context, label string, spec *FormSpec, st FormState) FormResult {
	col := ui.Column(c).AlignItems(ui.Stretch).Label(label).Role(ui.RoleGroup)
	fs := ui.Local(col, "form", func() formState { return formState{} })
	res := FormResult{Element: col}
	col.Children(func() {
		BlockBody(c, label, st.BlockState, func() {
			res.Values, res.Submitted = fs.show(c, spec, st)
		})
	})
	return res
}

// show builds the form's fields and its button, and submits it as they
// say: Enter in a one-line field or the button, unless it is busy.
func (fs *formState) show(c *Context, spec *FormSpec, st FormState) (values map[string]FormValue, submitted bool) {
	k := TokensOf(c)
	switch {
	case st.Missing:
		Empty(c, IconInbox, "Nothing to edit", "This row is no longer there.", nil)
		return nil, false
	case spec == nil:
		return nil, false
	}
	ui.Column(c).Gap(16).AlignItems(ui.Stretch).Children(func() {
		fs.fill(spec)
		submit := false
		for _, f := range spec.Fields {
			if fs.field(c, spec, f, st) {
				submit = true
			}
		}
		if problems := formProblems(spec, st); len(problems) > 0 {
			Alert(c, Red, "Not saved", strings.Join(problems, "\n"))
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			text := spec.Submit
			if text == "" {
				text = "Save"
			}
			var b Element
			if st.Busy {
				// The button keeps its name, so the focus stays on it.
				b = Disable(button(c, Default).PaddingX(16).Label(text).Busy(true), true)
				b.Children(func() {
					ui.Spinner(c).Size(14, 14).TextColor(k.PrimaryForeground)
					ui.Text(c, "Saving…").SingleLine()
				})
			} else {
				b = Button(c, text, Default)
			}
			if b.Clicked() {
				submit = true
			}
		})
		if submit && !st.Busy {
			values, submitted = fs.submit(spec)
		}
	})
	return values, submitted
}

// formProblems is the write's errors that show above the button: the one
// on no field, and those on names that are no field's, by name.
func formProblems(spec *FormSpec, st FormState) []string {
	if st.Busy {
		return nil
	}
	var out []string
	if st.Problem != "" {
		out = append(out, st.Problem)
	}
	var names []string
	for name, e := range st.Errors {
		if e != "" && !slices.ContainsFunc(spec.Fields, func(f FormField) bool { return f.Name == name }) {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	for _, name := range names {
		out = append(out, name+": "+st.Errors[name])
	}
	return out
}

// fill fills the fields from spec's values, as the form is given a new
// spec.
func (fs *formState) fill(spec *FormSpec) {
	if fs.spec == spec {
		return
	}
	fs.spec, fs.focus = spec, ""
	fs.fields = make(map[string]*fieldState, len(spec.Fields))
	for _, f := range spec.Fields {
		v := spec.Values[f.Name]
		st := &fieldState{text: v.Text, checked: v.Checked}
		if v.Date != nil {
			d := *v.Date
			st.date = &d
		}
		if f.Control == FormNumber && v.Text == "" {
			switch {
			case v.Int != nil:
				st.text = strconv.FormatInt(*v.Int, 10)
			case v.Number != nil:
				st.text = strconv.FormatFloat(*v.Number, 'f', -1, 64)
			}
		}
		if f.Control == FormFile && v.Text != "" {
			st.file.name = path.Base(v.Text)
		}
		fs.fields[f.Name] = st
	}
}

// submit checks the fields and, filled as required, returns their
// values; otherwise it marks those that aren't and focuses the first.
func (fs *formState) submit(spec *FormSpec) (map[string]FormValue, bool) {
	values := make(map[string]FormValue, len(spec.Fields))
	fs.focus = ""
	for _, f := range spec.Fields {
		st := fs.fields[f.Name]
		st.problem, st.edited = "", false
		v := FormValue{Text: st.text, Checked: st.checked}
		if st.date != nil {
			// A copy: the date control edits its own in place.
			d := *st.date
			v.Date = &d
		}
		empty := strings.TrimSpace(st.text) == ""
		switch f.Control {
		case FormDate:
			empty = st.date == nil
		case FormCheckbox:
			empty = !st.checked
		case FormNumber:
			if !empty {
				n, ok := readDecimal(st.text)
				if !ok {
					st.problem = "Enter a number"
				}
				v.Number = &n
				if i, ok := readWhole(st.text); ok {
					v.Int = &i
				}
			}
		case FormFile:
			if st.file.uploading {
				st.problem = "Wait for the upload to finish"
			}
		}
		if empty && f.Required && st.problem == "" {
			st.problem = "Required"
		}
		if st.problem != "" && fs.focus == "" {
			fs.focus = f.Name
		}
		values[f.Name] = v
	}
	if fs.focus != "" {
		return nil, false
	}
	return values, true
}

// readDecimal reads a number as typed, with Persian or Arabic digits too,
// and commas between groups of digits.
func readDecimal(s string) (float64, bool) {
	n, err := strconv.ParseFloat(plainNumber(s), 64)
	return n, err == nil && finite(n)
}

// readWhole reads a whole number as typed, as readDecimal does, exactly.
func readWhole(s string) (int64, bool) {
	n, err := strconv.ParseInt(plainNumber(s), 10, 64)
	return n, err == nil
}

// plainNumber is a number as typed in Latin digits, without the commas
// between groups of digits, and with a point for a Persian decimal one.
func plainNumber(s string) string {
	s = latinDigits(strings.TrimSpace(s))
	return strings.NewReplacer(",", "", "٬", "", "٫", ".").Replace(s)
}

// field builds a field, and reports whether Enter in it submits the form.
func (fs *formState) field(c *Context, spec *FormSpec, f FormField, st FormState) (submit bool) {
	k := TokensOf(c)
	fst := fs.fields[f.Name]
	problem := fst.problem
	if e := st.Errors[f.Name]; e != "" && problem == "" && !fst.edited && !st.Busy {
		problem = e
	}
	description := f.Description
	if description == "" && f.Required && f.Control != FormCheckbox {
		description = "Required"
	}
	focus := fs.focus == f.Name
	if focus {
		fs.focus = ""
	}
	control := func(e Element) Element {
		if st.Busy {
			e.Disabled(true)
		}
		if focus {
			e.Focus()
			focus = false
		}
		if e.Changed() {
			fst.edited, fst.problem = true, ""
		}
		return e
	}
	if f.Control == FormCheckbox {
		ui.Column(c).Gap(6).AlignItems(ui.Stretch).Children(func() {
			box := control(CheckField(c, &fst.checked, f.Label, f.Description))
			if problem != "" {
				box.Invalid(true).ErrorMessage(Text(c, problem).FontSize(13).TextColor(k.Destructive).MarginStart(24))
			}
		})
		return false
	}
	Field(c, f.Label, description, problem, func() {
		switch f.Control {
		case FormTextarea:
			control(TextArea(c, &fst.text, f.Label).Lines(3, 8))
		case FormNumber:
			in := control(field(c, ui.TextInput(c, &fst.text)).Height(36).Label(f.Label).
				FontFeatures("tnum").Direction(ui.LeftToRight).MaxWidth(240))
			submit = in.Submitted()
		case FormDate:
			fs.dateField(c, f, fst, control)
		case FormSelect:
			options := f.Options
			if !f.Required {
				// An optional select can go back to no choice.
				options = append([]PickerOption{{Value: "", Label: "None"}}, options...)
			}
			control(Picker(c, f.Label, &fst.text, options).Height(36))
		case FormFile:
			fs.fileField(c, spec, f, fst, st.Busy, control)
		default:
			submit = control(TextInput(c, &fst.text, f.Label)).Submitted()
		}
	})
	return submit
}

// dateField is a date picker, or a button choosing today while there is
// no date, and a button clearing an optional one.
func (fs *formState) dateField(c *Context, f FormField, fst *fieldState, control func(Element) Element) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		if fst.date == nil {
			// The button's name says what it does and for which field.
			if control(Button(c, "Choose a date", Outline).Label("Choose a date, " + f.Label)).Clicked() {
				now := c.Now()
				d := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
				fst.date, fst.edited, fst.problem = &d, true, ""
				fs.focus = f.Name
			}
			return
		}
		control(ui.DateInput(c, fst.date).Label(f.Label).Height(36).Radius(Radius * 0.8).FontSize(14))
		if !f.Required && control(IconButton(c, IconX, "Clear "+f.Label, Ghost)).Clicked() {
			fst.date, fst.edited = nil, true
		}
	})
}

// fileField is a file field: a button choosing a file and a place to drop
// one; the upload's progress; then the file, with a button removing it.
func (fs *formState) fileField(c *Context, spec *FormSpec, f FormField, fst *fieldState, busy bool, control func(Element) Element) {
	k := TokensOf(c)
	file := &fst.file
	start := func(p string) {
		file.uploads++
		n := file.uploads
		*file = fileState{name: filepath.Base(p), uploading: true, uploads: n}
		fst.text, fst.edited, fst.problem = "", true, ""
		if spec.Upload == nil {
			file.uploading, file.err = false, "Files can't be uploaded here."
			return
		}
		spec.Upload(f.Name, p, func(done float64) {
			if file.uploads == n && file.uploading {
				file.progress = max(0, min(done, 1))
			}
		}, func(key string, err error) {
			if file.uploads != n {
				return
			}
			file.uploading = false
			if err != nil {
				file.err = err.Error()
				return
			}
			file.progress, fst.text = 1, key
		})
	}
	box := ui.Column(c).Gap(8).Padding(12).Radius(Radius*0.8).Border(1, k.Input).AlignItems(ui.Stretch)
	// While the form saves, a drop is ignored, as the buttons are.
	for _, p := range box.DroppedFiles() {
		if !busy {
			start(p)
		}
	}
	box.Children(func() {
		if file.name != "" {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, IconPaperclip).Size(16, 16).Shrink(0).TextColor(k.MutedForeground)
				// A file's name runs left to right, as a path does.
				Text(c, file.name).FontSize(14).Grow(1).Shrink(1).Direction(ui.LeftToRight)
				if file.uploading {
					Text(c, strconv.Itoa(int(file.progress*100))+"%").FontSize(12).FontFeatures("tnum").TextColor(k.MutedForeground)
				}
				if control(IconButton(c, IconX, "Remove "+file.name, Ghost)).Clicked() {
					file.uploads++
					*file = fileState{uploads: file.uploads}
					fst.text, fst.edited = "", true
				}
			})
			if file.uploading {
				Progress(c, file.progress, "Uploading "+file.name)
			}
			if file.err != "" {
				Text(c, file.err).FontSize(13).TextColor(k.Destructive).Role(ui.RoleStatus)
			}
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if spec.Pick != nil {
				text := "Choose file"
				if file.name != "" {
					text = "Choose another"
				}
				if control(Button(c, text, Outline).Label(text + ", " + f.Label)).Clicked() {
					spec.Pick(f.Name, start)
				}
			}
			ui.Text(c, "or drop a file here").FontSize(13).TextColor(k.MutedForeground)
		})
	})
}
