package kit

import (
	"go/parser"
	"go/token"
	"image"
	"image/color"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// persian is right-to-left text every component shows in its test.
const persian = "سلام، این یک آزمایش است"

// window builds view in Root, in a light or a dark window.
func window(t *testing.T, dark bool, view func(c *Context)) *ui.Tester {
	t.Helper()
	if err := RegisterFonts(); err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(func(c *ui.Context) { Root(c, func() { view(c) }) }, 800, 600)
	tt.SetDark(dark)
	tt.Frame()
	return tt
}

// themes runs test in a light and a dark window, checking the window
// takes the theme's background.
func themes(t *testing.T, view func(c *Context), test func(t *testing.T, tt *ui.Tester)) {
	for _, dark := range []bool{false, true} {
		name := "light"
		if dark {
			name = "dark"
		}
		t.Run(name, func(t *testing.T) {
			tt := window(t, dark, view)
			want := Light.Background
			if dark {
				want = Dark.Background
			}
			if got := tt.Image().RGBAAt(799, 599); !near(got, want) {
				t.Fatalf("the window's corner is %v, want the background %v", got, want)
			}
			test(t, tt)
		})
	}
}

func near(a color.RGBA, b Color) bool {
	d := func(x, y uint8) bool { return x-y < 3 || y-x < 3 }
	return d(a.R, b.R) && d(a.G, b.G) && d(a.B, b.B)
}

// tabTo presses Tab until label has the focus.
func tabTo(t *testing.T, tt *ui.Tester, label string) {
	t.Helper()
	for range 12 {
		tt.Key(0, ui.KeyTab)
		if tt.Focused(label) {
			return
		}
	}
	t.Fatalf("Tab never focused %q", label)
}

// shows fails unless the window shows each text, or an element it names.
func shows(t *testing.T, tt *ui.Tester, texts ...string) {
	t.Helper()
	for _, s := range texts {
		if _, ok := tt.Find(s); !ok && !tt.HasText(s) {
			t.Errorf("no %q in %q", s, tt.Texts())
		}
	}
}

// passesOver checks Tab goes from Before to After, over what is between.
func passesOver(t *testing.T, tt *ui.Tester) {
	t.Helper()
	tabTo(t, tt, "Before")
	tt.Key(0, ui.KeyTab)
	if !tt.Focused("After") {
		t.Error("Tab stopped between Before and After")
	}
}

// between builds fn between two buttons, Before and After.
func between(c *Context, fn func()) {
	Row(c).Gap(8).Children(func() {
		Button(c, "Before", Outline)
		fn()
		Button(c, "After", Outline)
	})
}

// ink is the box of the pixels of img unlike its corner's, and how many.
func ink(img *image.RGBA) (image.Rectangle, int) {
	bg := img.RGBAAt(img.Rect.Max.X-1, img.Rect.Max.Y-1)
	var r image.Rectangle
	n := 0
	for y := img.Rect.Min.Y; y < img.Rect.Max.Y; y++ {
		for x := img.Rect.Min.X; x < img.Rect.Max.X; x++ {
			if img.RGBAAt(x, y) != bg {
				n++
				r = r.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	return r, n
}

func TestThemesFollowTheTokens(t *testing.T) {
	for _, dark := range []bool{false, true} {
		th, k := Theme(dark), &Light
		if dark {
			k = &Dark
		}
		if th.Dark != dark || th.Background != k.Background || th.Accent != k.Primary || th.Text != k.Foreground || th.Font != FontSans {
			t.Errorf("Theme(%v) = %+v", dark, th)
		}
		for _, tone := range Tones {
			if c := k.Tone(tone); c.Text == c.Soft {
				t.Errorf("tone %s has no contrast", tone)
			}
		}
	}
	if Light.Tone("teal") != Light.Tones[Neutral] {
		t.Error("an unknown tone is not Neutral")
	}
}

// Root lays the window out left to right in a right-to-left language too
// (SPEC 5.8, 11: no mirrored layout), while Persian text still starts on
// the right of its line.
func TestRightToLeftTextWithoutAMirroredLayout(t *testing.T) {
	for _, dir := range []ui.Direction{ui.LeftToRight, ui.RightToLeft} {
		tt := ui.NewTester(func(c *ui.Context) {
			Root(c, func() {
				Row(c).Children(func() { Text(c, "first"); Text(c, "second") })
				Text(c, persian).Width(600)
			})
		}, 800, 200)
		tt.SetDirection(dir)
		tt.Frame()
		first, _ := tt.Find("first")
		second, _ := tt.Find("second")
		if first.X > second.X {
			t.Errorf("direction %v: the row is mirrored (%v, %v)", dir, first, second)
		}
		p, _ := tt.Find(persian)
		if p.X > 1 {
			t.Errorf("direction %v: the paragraph moved to %v", dir, p)
		}
		img := tt.Image()
		r, _ := ink(img.SubImage(image.Rect(0, int(p.Y), 600, int(p.Y+p.H))).(*image.RGBA))
		if r.Min.X < 300 || r.Max.X < 560 {
			t.Errorf("direction %v: Persian text inks %v, want it on the right of 600", dir, r)
		}
	}
}

// The fonts are variable: weights between 400 and 700 draw differently,
// in Latin (Geist) and in Persian (Vazirmatn), and Geist is not the
// system's font.
func TestFontsHaveVariableWeights(t *testing.T) {
	if err := RegisterFonts(); err != nil {
		t.Fatal(err)
	}
	draw := func(s, font string, weight int) int {
		tt := ui.NewTester(func(c *ui.Context) {
			Root(c, func() { ui.Text(c, s).Font(font).FontWeight(weight).FontSize(24) })
		}, 800, 80)
		tt.Frame()
		_, n := ink(tt.Image())
		return n
	}
	for _, f := range []struct{ font, text string }{{"Geist", "Hamburgefonstiv 0123"}, {"Vazirmatn", persian}, {"Geist Mono", "func f() {}"}} {
		w := map[int]int{}
		for _, weight := range []int{300, 400, 500, 600, 700} {
			w[weight] = draw(f.text, f.font, weight)
		}
		t.Logf("%s: ink by weight %v", f.font, w)
		for _, pair := range [][2]int{{300, 400}, {400, 500}, {500, 600}, {600, 700}} {
			if w[pair[0]] >= w[pair[1]] {
				t.Errorf("%s: weight %d inks %d, no less than weight %d's %d", f.font, pair[0], w[pair[0]], pair[1], w[pair[1]])
			}
		}
	}
	if draw("Hamburgefonstiv", "Geist", 400) == draw("Hamburgefonstiv", "Segoe UI", 400) {
		t.Error("Geist draws as Segoe UI")
	}
}

// Screens and stores use the kit, never MyGo's ui themselves (SPIKE-048):
// only the kit imports github.com/egoist/mygo/ui and its packages.
func TestOnlyTheKitImportsMyGoUI(t *testing.T) {
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	kit, _ := filepath.Abs(".")
	checked := 0
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "node_modules", ".git", "frontend", "build", "bin", ".claude":
				return filepath.SkipDir
			}
			if path == kit {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		checked++
		for _, im := range f.Imports {
			p, _ := strconv.Unquote(im.Path.Value)
			if p == "github.com/egoist/mygo/ui" || strings.HasPrefix(p, "github.com/egoist/mygo/ui/") {
				rel, _ := filepath.Rel(root, path)
				t.Errorf("%s imports %s: use the kit", filepath.ToSlash(rel), p)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked < 100 {
		t.Fatalf("checked %d Go files: the walk missed the repository", checked)
	}
}
