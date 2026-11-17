package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/crash"
	"github.com/amir-saatchi/jenab/internal/secret"
)

type testKeyring map[string]string

func (k testKeyring) Get(name string) (string, error) {
	if v, ok := k[name]; ok {
		return v, nil
	}
	return "", secret.ErrNotFound
}
func (k testKeyring) Set(name, value string) error { k[name] = value; return nil }
func (k testKeyring) Delete(name string) error     { delete(k, name); return nil }

// The start after a crash turns its output into a log without the keys
// and tells the user, with a button that opens the log (TASK-067).
func TestTheNextStartShowsTheCrash(t *testing.T) {
	dir := t.TempDir()
	const key = "sk-test-0123456789abcdef"
	raw := filepath.Join(dir, "crash-20261117-100000-8.raw")
	if err := os.WriteFile(raw, []byte("panic: the call to "+key+" broke\n\ngoroutine 1 [running]:\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var s config.Settings
	s.LLM.Providers = map[string]config.ProviderSettings{"test": {Kind: "openai"}}
	secrets := secret.New(testKeyring{"provider:test": key})
	res, err := crash.Collect(dir, "", crashRedactor(s, secrets))
	if err != nil || len(res.Logs) != 1 {
		t.Fatalf("Collect: %v, %v", res, err)
	}
	logs := res.Logs
	b, _ := os.ReadFile(logs[0])
	if strings.Contains(string(b), key) || !strings.Contains(string(b), "[secret:provider:test]") {
		t.Fatalf("the crash log:\n%s", b)
	}

	var opened string
	toast, ok := crashNotice(res, func(p string) error { opened = p; return nil })
	if !ok || !toast.Sticky || toast.Action != "Open the log" || !strings.Contains(toast.Text, "closed unexpectedly") {
		t.Fatalf("toast %+v, %v", toast, ok)
	}
	toast.OnAction()
	if opened != logs[0] {
		t.Fatalf("opened %q; want %q", opened, logs[0])
	}
	if _, ok := crashNotice(crash.Result{}, nil); ok {
		t.Fatal("a notice with no crash")
	}
}

// A crash whose log couldn't be kept is still told, without a button that
// would open nothing (TASK-073).
func TestALostCrashLogIsTold(t *testing.T) {
	toast, ok := crashNotice(crash.Result{Lost: []string{"crash-20261117-100000-8.raw"}}, nil)
	if !ok || !toast.Sticky || !toast.Warning || toast.Action != "" || toast.OnAction != nil {
		t.Fatalf("toast %+v, %v", toast, ok)
	}
	for _, want := range []string{"closed unexpectedly", "couldn't be written", "deleted"} {
		if !strings.Contains(toast.Text, want) {
			t.Errorf("the text has no %q: %q", want, toast.Text)
		}
	}
}
