package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

func findNode(n SnapNode, pred func(SnapNode) bool) *SnapNode {
	if pred(n) {
		return &n
	}
	for _, c := range n.Children {
		if r := findNode(c, pred); r != nil {
			return r
		}
	}
	return nil
}

func (c *CLI) snapshot(winID string, interactive bool, extra ...string) (*SnapData, []byte, time.Duration, error) {
	args := []string{"snapshot", "--window-id", winID}
	if interactive {
		args = append(args, "-i")
	}
	args = append(args, extra...)
	env, raw, took, err := c.Run(30*time.Second, args...)
	if err != nil {
		return nil, raw, took, err
	}
	s, err := parseSnap(env.Data)
	return s, raw, took, err
}

func writeJSON(path string, v any) error {
	os.MkdirAll(filepath.Dir(path), 0o755)
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}
