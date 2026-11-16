package logic

// Ported from the React app's chat/line-diff.test.ts.

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func kinds(before, after string) []string {
	var out []string
	for _, o := range LineDiff(before, after) {
		out = append(out, string(o.Kind)+" "+o.Text)
	}
	return out
}

func TestLineDiff(t *testing.T) {
	cases := []struct {
		before, after string
		want          []string
	}{
		{"a\nb", "a\nb", []string{"same a", "same b"}},
		{"", "a\nb", []string{"add a", "add b"}},
		{"a\nb", "", []string{"del a", "del b"}},
		{"a\nb\nc", "a\nx\nc", []string{"same a", "del b", "add x", "same c"}},
		{"a\nb\nc", "a\nb\nc\n\n\nplanted", []string{"same a", "same b", "same c", "add ", "add ", "add planted"}},
		{"a\nb\nc\nd", "b\nc\nplanted\nd", []string{"del a", "same b", "same c", "add planted", "same d"}},
		{"قیمت طلا\nb", "قیمت طلا\nb\nپاسخ کوتاه", []string{"same قیمت طلا", "same b", "add پاسخ کوتاه"}},
	}
	for _, c := range cases {
		if got := kinds(c.before, c.after); !reflect.DeepEqual(got, c.want) {
			t.Errorf("LineDiff(%q, %q) = %q, want %q", c.before, c.after, got, c.want)
		}
	}
}

func TestChanges(t *testing.T) {
	cases := []struct {
		before, after string
		want          []Change
	}{
		{"a\nb", "a\n\n\n \nplanted\nb", []Change{{OpAdd, "", 3}, {OpAdd, "planted", 0}}},
		{"a\nb", "x", []Change{{OpDel, "a", 0}, {OpDel, "b", 0}, {OpAdd, "x", 0}}},
		{"a", "a", nil},
	}
	for _, c := range cases {
		if got := Changes(LineDiff(c.before, c.after)); !reflect.DeepEqual(got, c.want) {
			t.Errorf("Changes(%q, %q) = %v, want %v", c.before, c.after, got, c.want)
		}
	}
}

func numbered(n int, f string) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf(f, i)
	}
	return out
}

func TestLineDiffLongSections(t *testing.T) {
	before := strings.Join(numbered(300, "line %d"), "\n")
	after := strings.Replace(before, "line 150", "line 150\nplanted", 1)
	if got, want := Changes(LineDiff(before, after)), []Change{{OpAdd, "planted", 0}}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestLineDiffPastAFewThousandLines(t *testing.T) {
	old := numbered(1100, "line %d")
	now := append([]string(nil), old...)
	now[0] = "line 0, changed"
	now[len(now)-1] = "line 1099, changed"
	now = append(now[:550], append([]string{"planted"}, now[550:]...)...)
	want := []Change{
		{OpDel, "line 0", 0},
		{OpAdd, "line 0, changed", 0},
		{OpAdd, "planted", 0},
		{OpDel, "line 1099", 0},
		{OpAdd, "line 1099, changed", 0},
	}
	if got := Changes(LineDiff(strings.Join(old, "\n"), strings.Join(now, "\n"))); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}

	// The most lines the cap allows, 4,000 a side, all of them changed.
	a := numbered(4000, "a%d")
	b := numbered(4000, "b%d")
	var dels, adds []string
	for _, o := range LineDiff(strings.Join(a, "\n"), strings.Join(b, "\n")) {
		switch o.Kind {
		case OpDel:
			dels = append(dels, o.Text)
		case OpAdd:
			adds = append(adds, o.Text)
		}
	}
	if !reflect.DeepEqual(dels, a) || !reflect.DeepEqual(adds, b) {
		t.Errorf("4,000 lines a side: %d removed, %d added", len(dels), len(adds))
	}
}
