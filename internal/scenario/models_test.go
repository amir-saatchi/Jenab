package scenario

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/provider"
)

func TestModels(t *testing.T) {
	dir := t.TempDir()
	mf := filepath.Join(dir, "models.yaml")
	os.WriteFile(mf, []byte(`providers:
  - name: free
    kind: openai_compatible
    base_url: https://example.test/v1/
    key_env: JENAB_TEST_FREE_KEY
    models: [{id: a}, {id: b}]
  - name: paid
    kind: anthropic
    key_env: JENAB_TEST_PAID_KEY
    paid: true
    models: [{id: c}]
  - name: local
    kind: ollama
    models: [{id: d}]
`), 0o644)
	ef := filepath.Join(dir, ".env")
	os.WriteFile(ef, []byte("# keys\nexport JENAB_TEST_FREE_KEY=\"free-key-123456\"\n\nJENAB_TEST_PAID_KEY = paid-key-123456\nnot a line\n"), 0o644)

	f, err := ReadModels(mf)
	if err != nil {
		t.Fatal(err)
	}
	file, err := ReadEnvFile(ef)
	if err != nil {
		t.Fatal(err)
	}
	if file["JENAB_TEST_FREE_KEY"] != "free-key-123456" || file["JENAB_TEST_PAID_KEY"] != "paid-key-123456" || len(file) != 2 {
		t.Fatalf("env file %v", file)
	}

	// Without names: every model of the free providers.
	m, err := f.Select(nil, Env{File: file})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(m.Run, ",") != "free/a,free/b,local/d" || len(m.Providers) != 2 {
		t.Errorf("run %v, providers %d", m.Run, len(m.Providers))
	}
	if got := m.redact("bad key free-key-123456"); got != "bad key [redacted]" {
		t.Errorf("redact: %q", got)
	}

	// A paid model when named; the process environment works too.
	t.Setenv("JENAB_TEST_PAID_KEY", "env-key-123456")
	m, err = f.Select([]string{"paid/c", "paid/c"}, Env{})
	if err != nil || strings.Join(m.Run, ",") != "paid/c" || m.settings()["paid"].Kind != "anthropic" {
		t.Fatalf("%v %v", m, err)
	}
	if v, _ := m.keyring().Get(provider.KeyName("paid")); v != "env-key-123456" {
		t.Errorf("keyring has %q", v)
	}

	// Errors name the variable, never a value.
	if _, err := f.Select([]string{"free/a"}, Env{}); err == nil || !strings.Contains(err.Error(), "JENAB_TEST_FREE_KEY") {
		t.Errorf("missing key: %v", err)
	}
	if _, err := f.Select([]string{"free/x"}, Env{File: file}); err == nil {
		t.Error("unknown model accepted")
	}
	if _, err := f.Select([]string{"free"}, Env{File: file}); err == nil {
		t.Error("a name without a model accepted")
	}

	for _, bad := range []string{
		"providers: [{name: a/b, kind: ollama, models: [{id: m}]}]",
		"providers: [{name: a, kind: llama, models: [{id: m}]}]",
		"providers: [{name: a, kind: ollama}]",
		"providers: [{name: a, kind: ollama, models: [{id: m}]}, {name: a, kind: ollama, models: [{id: n}]}]",
		"providers: [{name: a, kind: ollama, key: x, models: [{id: m}]}]",
	} {
		os.WriteFile(mf, []byte(bad), 0o644)
		if _, err := ReadModels(mf); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
}
