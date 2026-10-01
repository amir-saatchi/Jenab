package main

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// The keys live in the repo-root .env (git-ignored). Only this function reads that file. Values stay
// in memory: they are never printed, logged, written or passed on a command line.

var keyNames = []string{"GROQ_API_KEY", "OLLAMA_API_KEY", "GEMINI_API_KEY", "Z_API_KEY"}

var secrets = map[string]string{}

func loadEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
			v = v[1 : len(v)-1]
		} else if i := strings.Index(v, " #"); i >= 0 {
			v = strings.TrimSpace(v[:i])
		}
		for _, n := range keyNames {
			if k == n && v != "" {
				secrets[n] = v
			}
		}
	}
	return sc.Err()
}

// account IDs in error messages (Groq puts the organization ID there) are not secrets, but
// they identify the user's account, so they are masked too.
var reOrg = regexp.MustCompile(`org_[A-Za-z0-9]{8,}`)

// redact replaces any key value in s.
func redact(s string) string {
	for _, v := range secrets {
		if len(v) >= 8 {
			s = strings.ReplaceAll(s, v, "[REDACTED]")
		}
	}
	return reOrg.ReplaceAllString(s, "org_[masked]")
}

// redactWriter redacts every write. Writes are line-buffered so a key can't be split across writes.
type redactWriter struct {
	mu  sync.Mutex
	w   io.Writer
	buf bytes.Buffer
}

func (r *redactWriter) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf.Write(p)
	for {
		i := bytes.IndexByte(r.buf.Bytes(), '\n')
		if i < 0 {
			break
		}
		line := string(r.buf.Next(i + 1))
		if _, err := io.WriteString(r.w, redact(line)); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

func (r *redactWriter) Flush() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.buf.Len() > 0 {
		io.WriteString(r.w, redact(r.buf.String()))
		r.buf.Reset()
	}
}

// secretScan checks every file in dir (recursively) for any key value. It returns file names only.
func secretScan(dir string) []string {
	var hits []string
	filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		for _, v := range secrets {
			if len(v) > 0 && bytes.Contains(b, []byte(v)) {
				rel, _ := filepath.Rel(dir, p)
				hits = append(hits, rel)
				break
			}
		}
		return nil
	})
	return hits
}
