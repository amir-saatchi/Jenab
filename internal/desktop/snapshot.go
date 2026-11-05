package desktop

import (
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"unicode/utf8"
)

// Node is one element of agent-desktop's snapshot tree.
type Node struct {
	Role     string   `json:"role"`
	Name     string   `json:"name"`
	Value    string   `json:"value"`
	States   []string `json:"states"`
	Ref      string   `json:"ref_id"`
	Children []Node   `json:"children"`
}

func (n Node) has(state string) bool { return slices.Contains(n.States, state) }

// snapData is a snapshot envelope's data.
type snapData struct {
	Complete bool `json:"complete"`
	// Unproven: the window wasn't read whole in time, so agent-desktop
	// refuses its refs (AMBIGUOUS_TARGET) until a new snapshot (1.1.0).
	Unproven bool `json:"identity_unproven"`
	Window   struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	} `json:"window"`
	Tree Node `json:"tree"`
}

func parseSnap(data json.RawMessage) (*snapData, error) {
	var s snapData
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("desktop: reading a snapshot: %w", err)
	}
	return &s, nil
}

// maxLines bounds a snapshot the model gets (8.10).
const maxLines = 400

// view is what the model sees of one snapshot: the compact form, one line
// per element, `- button "Send" [3:e14] (disabled)`, indented by depth.
// Unnamed containers without a ref are left out and their children move
// up. Element ids are Jenab's, snapshot number and element; ids maps them
// to agent-desktop's refs, and nodes to the elements, for the guards.
type view struct {
	text  string
	lines []string // without ids and indent, for the change line
	ids   map[string]string
	nodes map[string]Node
}

// compact prints roots for snapshot n.
func compact(roots []Node, n int) view {
	v := view{ids: map[string]string{}, nodes: map[string]Node{}}
	var b strings.Builder
	cut := 0
	var walk func(nd Node, depth int, parent string)
	walk = func(nd Node, depth int, parent string) {
		show := nd.Ref != "" || (nd.Name != "" && nd.Name != parent) || nd.Value != ""
		next, nextParent := depth, parent
		if show {
			if len(v.lines) >= maxLines {
				cut++
			} else {
				line := describe(nd)
				v.lines = append(v.lines, line)
				b.WriteString(strings.Repeat("  ", depth))
				b.WriteString("- " + nd.Role)
				if nd.Name != "" {
					fmt.Fprintf(&b, " %q", clip(nd.Name, 120))
				}
				if nd.Ref != "" {
					id := fmt.Sprintf("%d:%s", n, nd.Ref[strings.LastIndex(nd.Ref, ":")+1:])
					v.ids[id] = nd.Ref
					v.nodes[id] = nd
					b.WriteString(" [" + id + "]")
				}
				b.WriteString(strings.TrimPrefix(line, head(nd)))
				b.WriteByte('\n')
			}
			next, nextParent = depth+1, nd.Name
		}
		for _, c := range nd.Children {
			walk(c, next, nextParent)
		}
	}
	for _, r := range roots {
		walk(r, 0, "")
	}
	if cut > 0 {
		fmt.Fprintf(&b, "[%d more elements left out; take a full snapshot of less, or act and look again]\n", cut)
	}
	v.text = b.String()
	return v
}

// head is an element's role and name; describe adds its value and states.
func head(n Node) string {
	s := n.Role
	if n.Name != "" {
		s += fmt.Sprintf(" %q", clip(n.Name, 120))
	}
	return s
}

func describe(n Node) string {
	s := head(n)
	if n.Value != "" && n.Value != n.Name && n.Role != "link" { // a link's value is its URL
		s += fmt.Sprintf(" = %q", clip(n.Value, 200))
	}
	if len(n.States) > 0 {
		s += " (" + strings.Join(n.States, ", ") + ")"
	}
	return s
}

// changes is the line after an action (8.10): what is new in the window
// since the last snapshot, such as `Display is "4,827"`.
func changes(before, after []string) string {
	count := map[string]int{}
	for _, l := range before {
		count[l]++
	}
	var added []string
	for _, l := range after {
		if count[l] > 0 {
			count[l]--
		} else {
			added = append(added, l)
		}
	}
	removed := 0
	for _, c := range count {
		removed += c
	}
	if len(added) == 0 && removed == 0 {
		return "Nothing in the window changed."
	}
	var b strings.Builder
	b.WriteString("Changed: ")
	for i, l := range added[:min(3, len(added))] {
		if i > 0 {
			b.WriteString("; ")
		}
		b.WriteString(l)
	}
	if len(added) == 0 {
		b.WriteString("nothing new")
	}
	if more := len(added) - 3; more > 0 {
		fmt.Fprintf(&b, "; and %d more", more)
	}
	if removed > 0 {
		fmt.Fprintf(&b, " (%d elements gone)", removed)
	}
	return b.String() + "."
}

// Browsers by their program: in one, the model sees only the page and
// the address bar, as an origin (8.10).
var browsers = map[string]bool{
	"chrome.exe": true, "msedge.exe": true, "firefox.exe": true, "brave.exe": true,
	"opera.exe": true, "vivaldi.exe": true, "chromium.exe": true,
}

func isBrowser(app string) bool { return browsers[strings.ToLower(app)] }

// browserArgs read a page deep enough: agent-desktop's default depth of
// 10 hid Google's results (SPIKE-039).
var browserArgs = []string{"--max-depth", "40", "--compact"}

// pageNode is the tab's page: the first document with an address. A
// browser's own UI has documents too, without one.
func pageNode(n *Node) *Node {
	return find(n, func(n *Node) bool { return n.Role == "document" && n.Value != "" })
}

// addressBar is the browser's address field: an editable element outside
// the page with an address-like name, or else one whose value is on the
// page's host.
func addressBar(root, page *Node) *Node {
	var named, byHost *Node
	host := ""
	if page != nil {
		if u, err := url.Parse(page.Value); err == nil {
			host = u.Host
		}
	}
	var walk func(n *Node)
	walk = func(n *Node) {
		if n == page {
			return
		}
		if n.Ref != "" && (n.Role == "textfield" || n.Role == "combobox") {
			if named == nil && reAddress.MatchString(n.Name) {
				named = n
			}
			if byHost == nil && host != "" && strings.Contains(n.Value, host) {
				byHost = n
			}
		}
		for i := range n.Children {
			walk(&n.Children[i])
		}
	}
	walk(root)
	if named != nil {
		return named
	}
	return byHost
}

func find(n *Node, pred func(*Node) bool) *Node {
	if pred(n) {
		return n
	}
	for i := range n.Children {
		if r := find(&n.Children[i], pred); r != nil {
			return r
		}
	}
	return nil
}

// origin is an address's scheme and host: never its path, which for a
// file:// page has the Windows user name. An address bar often shows a
// host without a scheme.
func origin(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	u, err := url.Parse(v)
	if err == nil && u.Scheme != "" {
		switch {
		case u.Scheme == "file":
			return "file://"
		case u.Host != "":
			return strings.ToLower(u.Scheme + "://" + u.Host)
		case u.Opaque != "":
			return strings.ToLower(u.Scheme + ":" + strings.SplitN(u.Opaque, "/", 2)[0])
		}
	}
	host, _, _ := strings.Cut(v, "/")
	if !strings.ContainsAny(host, " \t") && strings.Contains(host, ".") {
		return strings.ToLower(host)
	}
	return "" // text typed into the bar, not an address
}

// blank reports whether an origin is no site yet: a new tab or an empty
// page. Such a window takes the origin of the first site it opens.
func blank(o string) bool {
	if o == "" {
		return true
	}
	scheme, _, _ := strings.Cut(o, ":")
	switch scheme {
	case "about", "chrome", "edge", "brave", "vivaldi", "opera", "chrome-search", "moz-extension":
		return true
	}
	return false
}

// allText is the names and values of a subtree, for the CAPTCHA check.
func allText(n *Node) string {
	var b strings.Builder
	var walk func(*Node)
	walk = func(n *Node) {
		b.WriteString(n.Name + " " + n.Value + " ")
		for i := range n.Children {
			walk(&n.Children[i])
		}
	}
	walk(n)
	return b.String()
}

func anySecure(n *Node) bool {
	return find(n, func(n *Node) bool { return n.has("secure") }) != nil
}

func clip(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}
