package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

// SnapNode is one node of agent-desktop's snapshot tree (envelope data.tree).
type SnapNode struct {
	Role     string     `json:"role"`
	Name     string     `json:"name"`
	Value    string     `json:"value"`
	States   []string   `json:"states"`
	Ref      string     `json:"ref_id"`
	Children []SnapNode `json:"children"`
}

type SnapData struct {
	App        string `json:"app"`
	Complete   bool   `json:"complete"`
	RefCount   int    `json:"ref_count"`
	SnapshotID string `json:"snapshot_id"`
	Window     struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	} `json:"window"`
	Tree SnapNode `json:"tree"`
}

func parseSnap(data json.RawMessage) (*SnapData, error) {
	var s SnapData
	err := json.Unmarshal(data, &s)
	return &s, err
}

// Compact prints one line per element: `- button "Send" [3:e14] (disabled)`, indented by depth.
// Unnamed containers without a ref are dropped and their children moved up; a node without a
// ref that only repeats its parent's name is dropped. ids maps the printed id to the ref.
type Compact struct {
	Text  string
	Lines int
	IDs   map[string]string
	Nodes map[string]SnapNode // printed id -> node (for the password check)
}

const maxCompactLines = 400

func compact(s *SnapData, snapN int) Compact {
	c := compactFrom([]SnapNode{s.Tree}, s.Window.Title, snapN)
	c.Text = fmt.Sprintf("window %q (snapshot %d)\n", s.Window.Title, snapN) + c.Text
	if !s.Complete {
		c.Text += "[the tree is incomplete: the app did not answer in time]\n"
	}
	return c
}

// compactNodes prints the given subtrees (a browser's address bar and page) in the same form.
func compactNodes(roots []SnapNode, snapN int) Compact { return compactFrom(roots, "", snapN) }

func compactFrom(roots []SnapNode, rootName string, snapN int) Compact {
	c := Compact{IDs: map[string]string{}, Nodes: map[string]SnapNode{}}
	var b strings.Builder
	cut := 0
	var walk func(n SnapNode, depth int, parentName string)
	walk = func(n SnapNode, depth int, parentName string) {
		show := n.Ref != "" || ((n.Name != "" || n.Value != "") && !(n.Ref == "" && n.Name == parentName && n.Value == ""))
		childDepth, childParent := depth, parentName
		if show {
			if c.Lines >= maxCompactLines {
				cut++
			} else {
				c.Lines++
				b.WriteString(strings.Repeat("  ", depth))
				b.WriteString("- " + n.Role)
				if n.Name != "" {
					fmt.Fprintf(&b, " %q", clip(n.Name, 120))
				}
				if n.Ref != "" {
					i := strings.LastIndex(n.Ref, ":")
					id := fmt.Sprintf("%d:%s", snapN, n.Ref[i+1:])
					c.IDs[id] = n.Ref
					c.Nodes[id] = n
					b.WriteString(" [" + id + "]")
				}
				if n.Value != "" && n.Value != n.Name && n.Role != "link" { // a link's value is its URL
					fmt.Fprintf(&b, " = %q", clip(n.Value, 200))
				}
				if len(n.States) > 0 {
					b.WriteString(" (" + strings.Join(n.States, ", ") + ")")
				}
				b.WriteByte('\n')
			}
			childDepth, childParent = depth+1, n.Name
		}
		for _, ch := range n.Children {
			walk(ch, childDepth, childParent)
		}
	}
	for _, r := range roots {
		walk(r, 0, rootName)
	}
	if cut > 0 {
		fmt.Fprintf(&b, "[%d more elements left out]\n", cut)
	}
	c.Text = b.String()
	return c
}

func clip(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n]) + "…"
}

func countNodes(n SnapNode) (all, refs int) {
	all = 1
	if n.Ref != "" {
		refs = 1
	}
	for _, c := range n.Children {
		a, r := countNodes(c)
		all += a
		refs += r
	}
	return
}

// pageDoc is a browser window's page: the document node whose value is its address.
func pageDoc(n SnapNode) *SnapNode {
	return findNode(n, func(n SnapNode) bool {
		return n.Role == "document" && (strings.HasPrefix(n.Value, "http://") || strings.HasPrefix(n.Value, "https://"))
	})
}
