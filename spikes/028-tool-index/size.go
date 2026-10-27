package main

// The index's size against the full schemas of the same tools, measured
// on the registry each set's runs really get, as the app's own estimate
// (4 bytes a token) counts a request's tools and system text.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/skill"
	"github.com/amir-saatchi/jenab/internal/tool"
)

// size is one set's tool definitions in a main chat, in estimated tokens.
type size struct {
	Core      int `json:"core"`       // the core tools, native in both conditions
	CoreTools int `json:"core_tools"` // how many
	Full      int `json:"full"`       // the other tools' full definitions (condition 1)
	Rare      int `json:"rare_tools"` // how many
	Index     int `json:"index"`      // their index lines in block 1, with describe_tool and call_tool (condition 2)
	IndexText int `json:"index_text"` // of that, the lines in block 1
}

func defsTokens(ts []tool.Tool) int {
	var req provider.Request
	for _, t := range ts {
		s := t.Spec()
		req.Tools = append(req.Tools, provider.ToolDef{Name: s.Name, Description: s.Description, Schema: s.Schema})
	}
	return provider.EstimateTokens(req)
}

func measureSize(all *tool.Registry, skills *skill.Set) size {
	var sz size
	var core, rare []tool.Tool
	for _, t := range all.For(chat.KindChat) {
		if isCore(t.Spec().Name) {
			core = append(core, t)
		} else {
			rare = append(rare, t)
		}
	}
	reg, text := newIndex(all, skills)
	var added []tool.Tool
	for _, t := range reg.For(chat.KindChat) {
		if n := t.Spec().Name; n == describeName || n == callName {
			added = append(added, t)
		}
	}
	idx := text(chat.KindChat)
	sz.Core, sz.CoreTools = defsTokens(core), len(core)
	sz.Full, sz.Rare = defsTokens(rare), len(rare)
	sz.IndexText = provider.EstimateTokens(provider.Request{System: []provider.Block{{Text: "\n\n" + idx}}})
	sz.Index = sz.IndexText + defsTokens(added)
	return sz
}

// measure records a set's size once and writes sizes.json.
func (h *harness) measure(set string, all *tool.Registry, skills *skill.Set) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.sizes[set]; ok {
		return
	}
	h.sizes[set] = measureSize(all, skills)
	raw, _ := json.MarshalIndent(h.sizes, "", "  ")
	if err := os.WriteFile(filepath.Join(h.dir, "sizes.json"), raw, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "sizes.json:", err)
	}
}

// writeSizes adds the sizes the folders measured; a set measured twice
// is the same registry, so the first is kept.
func writeSizes(b *strings.Builder, dirs []string) {
	sizes := map[string]size{}
	for _, d := range dirs {
		raw, err := os.ReadFile(filepath.Join(d, "sizes.json"))
		if err != nil {
			continue
		}
		var got map[string]size
		if json.Unmarshal(raw, &got) != nil {
			continue
		}
		for k, v := range got {
			sizes[k] = v // the later folder's, as with runs
		}
	}
	if len(sizes) == 0 {
		return
	}
	b.WriteString("\n## Tool definitions in a main chat (estimated tokens)\n\n")
	b.WriteString("| set | core tools | core tokens | other tools | their full definitions (c1) | index lines + describe_tool + call_tool (c2) | of that, index lines | saved |\n")
	b.WriteString("|---|--:|--:|--:|--:|--:|--:|--:|\n")
	for _, set := range setOrder {
		sz, ok := sizes[set]
		if !ok {
			continue
		}
		fmt.Fprintf(b, "| %s | %d | %d | %d | %d | %d | %d | %s |\n", set, sz.CoreTools, sz.Core, sz.Rare, sz.Full, sz.Index, sz.IndexText,
			pct(sz.Full-sz.Index, sz.Full))
	}
}
