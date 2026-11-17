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

// Names lists the keychain, as Credential Manager can.
func (k testKeyring) Names() ([]string, error) {
	var out []string
	for n := range k {
		out = append(out, n)
	}
	return out, nil
}

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

// A crash from before a provider was removed keeps that provider's key
// out of its log (TASK-076): the keychain still has it, so it is hidden by
// value; a key the keychain no longer has is hidden by its shape.
func TestACrashHidesARemovedProvidersKey(t *testing.T) {
	dir := t.TempDir()
	const (
		kept    = "sk-test-0123456789abcdef"
		removed = "removed-0123456789abcdef"   // no known shape: found by value
		gone    = "gone0123456789abcdefABCDEF" // not in the keychain
	)
	raw := filepath.Join(dir, "crash-20261117-100000-8.raw")
	out := "panic: the call broke\n" +
		"POST /v1/chat headers map[Authorization:[Bearer " + removed + "]]\n" +
		"GET /v1/models Authorization: Bearer " + gone + "\n" +
		"kept " + kept + "\n\ngoroutine 1 [running]:\n"
	if err := os.WriteFile(raw, []byte(out), 0o600); err != nil {
		t.Fatal(err)
	}
	var s config.Settings
	s.LLM.Providers = map[string]config.ProviderSettings{"test": {Kind: "openai"}}
	secrets := secret.New(testKeyring{"provider:test": kept, "provider:old": removed})
	res, err := crash.Collect(dir, "", crashRedactor(s, secrets))
	if err != nil || len(res.Logs) != 1 {
		t.Fatalf("Collect: %v, %v", res, err)
	}
	b, _ := os.ReadFile(res.Logs[0])
	for _, k := range []string{kept, removed, gone} {
		if strings.Contains(string(b), k) {
			t.Errorf("the crash log shows %q:\n%s", k, b)
		}
	}
	for _, want := range []string{"[secret:provider:test]", "[secret:provider:old]", "Bearer [secret]", "goroutine 1 [running]"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("the crash log has no %q:\n%s", want, b)
		}
	}
}

// countingKeyring counts the keychain's reads and listings.
type countingKeyring struct {
	testKeyring
	reads int
}

func (k *countingKeyring) Get(name string) (string, error) {
	k.reads++
	return k.testKeyring.Get(name)
}

func (k *countingKeyring) Names() ([]string, error) {
	k.reads++
	return k.testKeyring.Names()
}

// A start with no crash output doesn't read the keychain (TASK-078); one
// with crash output reads it then, and hides the keys.
func TestTheCrashRedactorReadsTheKeychainForACrashOnly(t *testing.T) {
	dir := t.TempDir()
	const key = "sk-test-0123456789abcdef"
	var s config.Settings
	s.LLM.Providers = map[string]config.ProviderSettings{"test": {Kind: "openai"}}
	kr := &countingKeyring{testKeyring: testKeyring{"provider:test": key, "provider:old": "removed-0123456789abcdef"}}
	secrets := secret.New(kr)

	// No crash output: nothing at all, then an empty file of a run that
	// ended without stop.
	if _, err := crash.Collect(dir, "", crashRedactor(s, secrets)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "crash-20261117-100000-7.raw"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := crash.Collect(dir, "", crashRedactor(s, secrets)); err != nil {
		t.Fatal(err)
	}
	if kr.reads != 0 {
		t.Fatalf("a start with no crash read the keychain %d times", kr.reads)
	}

	raw := filepath.Join(dir, "crash-20261117-100000-8.raw")
	if err := os.WriteFile(raw, []byte("panic: the call to "+key+" broke\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := crash.Collect(dir, "", crashRedactor(s, secrets))
	if err != nil || len(res.Logs) != 1 {
		t.Fatalf("Collect: %v, %v", res, err)
	}
	if kr.reads == 0 {
		t.Error("the crash's keys weren't read")
	}
	b, _ := os.ReadFile(res.Logs[0])
	if strings.Contains(string(b), key) || !strings.Contains(string(b), "[secret:provider:test]") {
		t.Errorf("the crash log:\n%s", b)
	}
}
