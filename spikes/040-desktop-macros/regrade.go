package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// regrade grades the Chrome runs of a folder again from their logs and saved reference lists,
// joining every answer text of a run (the reminder can split the results from the last answer).
func runRegrade(dir string) error {
	logs, _ := filepath.Glob(filepath.Join(dir, "chrome-*.jsonl"))
	for _, l := range logs {
		var ref []Result
		b, err := os.ReadFile(strings.TrimSuffix(l, ".jsonl") + "-reference.json")
		if err != nil || json.Unmarshal(b, &ref) != nil {
			fmt.Printf("%s: no reference\n", filepath.Base(l))
			continue
		}
		f, err := os.Open(l)
		if err != nil {
			return err
		}
		var answers []string
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 4<<20), 64<<20)
		for sc.Scan() {
			var d struct {
				Answer *string `json:"answer"`
			}
			if json.Unmarshal(sc.Bytes(), &d) == nil && d.Answer != nil && *d.Answer != "" {
				answers = append(answers, *d.Answer)
			}
		}
		f.Close()
		m, h := gradeResults(ref, strings.Join(answers, "\n"))
		fmt.Printf("%s: titles %d/5 in order, hosts %d/5 (%d answer texts)\n", filepath.Base(l), m, h, len(answers))
	}
	return nil
}
