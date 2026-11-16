package logic

import (
	"slices"
	"strings"
)

// A line diff for a memory edit's card (SPEC 3.3): the lines the edit
// adds and removes, so a planted line can't hide in a long section.

// OpKind is what a diff does with a line.
type OpKind string

const (
	OpSame OpKind = "same"
	OpAdd  OpKind = "add"
	OpDel  OpKind = "del"
)

// LineOp is one line of a diff.
type LineOp struct {
	Kind OpKind
	Text string
}

// Change is a changed line on the card; Blanks counts a run of blank lines
// shown as one row, so blank lines can't push a changed line out of view.
type Change struct {
	Kind   OpKind // OpAdd or OpDel
	Text   string
	Blanks int
}

func diffLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// LineDiff is before and after as kept, removed and added lines, in order.
// A section that isn't there is "".
func LineDiff(before, after string) []LineOp {
	a := diffLines(before)
	b := diffLines(after)
	start := 0
	for start < len(a) && start < len(b) && a[start] == b[start] {
		start++
	}
	ea, eb := len(a), len(b)
	for ea > start && eb > start && a[ea-1] == b[eb-1] {
		ea--
		eb--
	}
	out := make([]LineOp, 0, len(a)+len(b))
	for _, t := range a[:start] {
		out = append(out, LineOp{OpSame, t})
	}
	out = align(a[start:ea], b[start:eb], out)
	for _, t := range a[ea:] {
		out = append(out, LineOp{OpSame, t})
	}
	return out
}

// Changes are the diff's added and removed lines, with each run of blank
// ones of a kind as one row.
func Changes(ops []LineOp) []Change {
	var out []Change
	for _, op := range ops {
		if op.Kind == OpSame {
			continue
		}
		blank := jsTrim(op.Text) == ""
		if n := len(out); blank && n > 0 && out[n-1].Kind == op.Kind && out[n-1].Blanks > 0 {
			out[n-1].Blanks++
			continue
		}
		c := Change{Kind: op.Kind, Text: op.Text}
		if blank {
			c.Text, c.Blanks = "", 1
		}
		out = append(out, c)
	}
	return out
}

// align appends the ops that turn x into y, keeping a longest common run
// of lines (Hirschberg): exact for any size, in memory linear in the
// lines, so no section is too long to diff.
func align(x, y []string, out []LineOp) []LineOp {
	if len(x) == 0 {
		for _, t := range y {
			out = append(out, LineOp{OpAdd, t})
		}
		return out
	}
	if len(y) == 0 {
		for _, t := range x {
			out = append(out, LineOp{OpDel, t})
		}
		return out
	}
	if len(x) == 1 {
		j := slices.Index(y, x[0])
		if j < 0 {
			out = append(out, LineOp{OpDel, x[0]})
			for _, t := range y {
				out = append(out, LineOp{OpAdd, t})
			}
			return out
		}
		for _, t := range y[:j] {
			out = append(out, LineOp{OpAdd, t})
		}
		out = append(out, LineOp{OpSame, x[0]})
		for _, t := range y[j+1:] {
			out = append(out, LineOp{OpAdd, t})
		}
		return out
	}
	half := len(x) >> 1
	head := lcsRow(x[:half], y)
	tail := lcsRow(reversed(x[half:]), reversed(y))
	k := 0
	for j := 1; j <= len(y); j++ {
		if head[j]+tail[len(y)-j] > head[k]+tail[len(y)-k] {
			k = j
		}
	}
	out = align(x[:half], y[:k], out)
	return align(x[half:], y[k:], out)
}

func reversed(s []string) []string {
	r := slices.Clone(s)
	slices.Reverse(r)
	return r
}

// lcsRow is, for each j, the longest common run of x and y's first j lines.
func lcsRow(x, y []string) []uint32 {
	prev := make([]uint32, len(y)+1)
	cur := make([]uint32, len(y)+1)
	for _, line := range x {
		for j := 1; j <= len(y); j++ {
			if line == y[j-1] {
				cur[j] = prev[j-1] + 1
			} else {
				cur[j] = max(prev[j], cur[j-1])
			}
		}
		prev, cur = cur, prev
	}
	return prev
}
