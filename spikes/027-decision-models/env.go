package main

import (
	"bufio"
	"os"
	"strings"
)

// The keys live in the repo-root .env (git-ignored). Only loadEnv reads that file, and only the
// names below. Values stay in memory: they are never printed, logged, written or put on a
// command line. redact removes them from anything that is printed or written.

var keyNames = []string{"CLOUDFLARE_ID", "CLOUDFLARE_TOKEN", "OLLAMA_API_KEY", "Z_API_KEY"}

var secrets = map[string]string{}

func loadEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	want := map[string]bool{}
	for _, k := range keyNames {
		want[k] = true
	}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(strings.TrimPrefix(line, "export "), "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if !want[k] {
			continue
		}
		if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
			v = v[1 : len(v)-1]
		}
		secrets[k] = v
	}
	return sc.Err()
}

func redact(s string) string {
	for k, v := range secrets {
		if len(v) >= 6 {
			s = strings.ReplaceAll(s, v, "<"+k+">")
		}
	}
	return s
}
