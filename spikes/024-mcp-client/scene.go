package main

import (
	"regexp"
	"strconv"
	"strings"
)

// parseScene reads get_scene's Scene Index. An entry is
//
//   - Name (kind) pos [x, y, z] size [x, y, z]  id:<id>
//     scale [..] · #rrggbb metal 0 rough 0.6
//
// Members of a group say "(box, in group <id>)". Sizes are left out when the
// 3D view can't measure them; then a primitive's height is its scale.
func parseScene(txt string) scene {
	sc := scene{Text: txt, Parents: true}
	in := false
	type open struct {
		indent int
		id     string
	}
	var stack []open
	for _, line := range strings.Split(txt, "\n") {
		if strings.HasPrefix(line, "## ") {
			in = strings.HasPrefix(line, "## Scene Index")
			continue
		}
		if !in {
			continue
		}
		if m := entryRe.FindStringSubmatch(line); m != nil {
			indent := len(m[1])
			for len(stack) > 0 && stack[len(stack)-1].indent >= indent {
				stack = stack[:len(stack)-1]
			}
			o := sceneObj{Name: m[2], Kind: m[3], ID: m[5]}
			if k, g, ok := strings.Cut(o.Kind, ", in group "); ok {
				o.Kind, o.Parent = k, g
			} else if len(stack) > 0 {
				o.Parent = stack[len(stack)-1].id
			}
			o.Light = strings.Contains(o.Kind, "light")
			if p, ok := vec(posRe, m[4]); ok {
				o.Pos = p
			}
			if s, ok := vec(sizeRe, m[4]); ok {
				o.Bottom, o.Top, o.HasSpan = o.Pos[1]-s[1]/2, o.Pos[1]+s[1]/2, true
			}
			o.Hidden = strings.Contains(strings.ToLower(m[4]), "hidden")
			sc.Objects = append(sc.Objects, o)
			stack = append(stack, open{indent, o.ID})
			continue
		}
		// a detail line belongs to the last entry
		if len(sc.Objects) == 0 || strings.TrimSpace(line) == "" {
			continue
		}
		o := &sc.Objects[len(sc.Objects)-1]
		if c := colorRe.FindString(line); c != "" && o.Color == "" {
			o.Color = c
		}
		if strings.Contains(strings.ToLower(line), "hidden") {
			o.Hidden = true
		}
		if s, ok := vec(scaleRe, line); ok && !o.HasSpan && unitShape(o.Kind) {
			o.Bottom, o.Top, o.HasSpan = o.Pos[1]-s[1]/2, o.Pos[1]+s[1]/2, true
		}
	}
	for i := range sc.Objects {
		o := &sc.Objects[i]
		if !o.HasSpan && unitShape(o.Kind) && o.Parent == "" {
			o.Bottom, o.Top, o.HasSpan = o.Pos[1]-0.5, o.Pos[1]+0.5, true
		}
	}
	return sc
}

// sceneIndex is get_scene's output cut down to its Scene Index entries and
// their detail lines. The rest is Viso's own text, which isn't committed.
func sceneIndex(txt string) string {
	var out []string
	in := false
	for _, line := range strings.Split(txt, "\n") {
		if strings.HasPrefix(line, "## ") {
			in = strings.HasPrefix(line, "## Scene Index")
			continue
		}
		if in && (entryRe.MatchString(line) || strings.HasPrefix(line, "    ") && len(out) > 0) {
			out = append(out, line)
		}
	}
	body := "(no objects)"
	if len(out) > 0 {
		body = strings.Join(out, "\n")
	}
	return "[get_scene output trimmed to its scene index; the rest is Viso's text, not committed]\n" + body
}

var (
	entryRe = regexp.MustCompile(`^(\s*)- (.+?) \(([^)]+)\)(.*?)\s*id:(\S+)`)
	posRe   = regexp.MustCompile(`pos \[([^\]]+)\]`)
	sizeRe  = regexp.MustCompile(`size \[([^\]]+)\]`)
	scaleRe = regexp.MustCompile(`scale \[([^\]]+)\]`)
	colorRe = regexp.MustCompile(`#[0-9a-fA-F]{6}`)
)

func unitShape(kind string) bool {
	switch kind {
	case "box", "sphere", "cylinder", "cone":
		return true
	}
	return false
}

func vec(re *regexp.Regexp, s string) ([3]float64, bool) {
	var v [3]float64
	m := re.FindStringSubmatch(s)
	if m == nil {
		return v, false
	}
	parts := strings.Split(m[1], ",")
	if len(parts) != 3 {
		return v, false
	}
	for i, p := range parts {
		f, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil {
			return v, false
		}
		v[i] = f
	}
	return v, true
}
