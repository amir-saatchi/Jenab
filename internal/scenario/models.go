package scenario

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/secret"
)

// ModelsFile lists the providers a developer can run scenarios on. Nothing
// is built in: each developer lists their own, with the environment
// variable that holds each key (SPEC 6.7). Keys never go in this file or
// in a report.
type ModelsFile struct {
	Providers []ProviderSpec `yaml:"providers"`
}

// ProviderSpec is one provider and its models.
type ProviderSpec struct {
	Name    string                 `yaml:"name"`
	Kind    string                 `yaml:"kind"`     // as in config.yaml
	BaseURL string                 `yaml:"base_url"` // "" means the kind's own
	KeyEnv  string                 `yaml:"key_env"`  // the variable with the key; "" for a local Ollama
	Paid    bool                   `yaml:"paid"`     // its models only run when named
	Models  []config.ModelSettings `yaml:"models"`
}

// Models are the providers of a run, their keys and the models to run.
type Models struct {
	Providers []ProviderSpec
	Run       []string // "provider/id"
	keys      map[string]string
}

// ReadModels reads a models file.
func ReadModels(path string) (*ModelsFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f ModelsFile
	if err := strict(b, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	seen := map[string]bool{}
	for i, p := range f.Providers {
		switch {
		case p.Name == "" || strings.Contains(p.Name, "/"):
			return nil, fmt.Errorf("%s: providers[%d]: a name without /", path, i)
		case seen[p.Name]:
			return nil, fmt.Errorf("%s: providers[%d]: %s is listed twice", path, i, p.Name)
		case !slices.Contains(config.ProviderKinds, p.Kind):
			return nil, fmt.Errorf("%s: %s: kind must be one of %s", path, p.Name, strings.Join(config.ProviderKinds, ", "))
		case len(p.Models) == 0:
			return nil, fmt.Errorf("%s: %s: no models", path, p.Name)
		}
		seen[p.Name] = true
	}
	return &f, nil
}

// Env is where keys come from: the process environment, and a .env file
// if one is given, which wins.
type Env struct {
	File map[string]string
}

// ReadEnvFile reads KEY=value lines; # starts a comment line.
func ReadEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string]string{}
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
		out[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"'`)
	}
	return out, sc.Err()
}

func (e Env) get(name string) string {
	if v, ok := e.File[name]; ok && v != "" {
		return v
	}
	return os.Getenv(name)
}

// Select picks the models to run: those named, as "provider/id", or else
// every model of a provider that isn't paid. It reads the keys of the
// providers it needs; a missing key is an error that names the variable,
// never a value.
func (f *ModelsFile) Select(names []string, env Env) (*Models, error) {
	m := &Models{keys: map[string]string{}}
	find := func(name string) (*ProviderSpec, bool) {
		p, id, ok := strings.Cut(name, "/")
		if !ok {
			return nil, false
		}
		for i := range f.Providers {
			if f.Providers[i].Name == p {
				return &f.Providers[i], slices.ContainsFunc(f.Providers[i].Models, func(s config.ModelSettings) bool { return s.ID == id })
			}
		}
		return nil, false
	}
	if len(names) == 0 {
		for _, p := range f.Providers {
			if p.Paid {
				continue
			}
			for _, s := range p.Models {
				names = append(names, p.Name+"/"+s.ID)
			}
		}
	}
	for _, n := range names {
		p, ok := find(n)
		if !ok {
			return nil, fmt.Errorf("no model %s in the models file", n)
		}
		if !slices.Contains(m.Run, n) {
			m.Run = append(m.Run, n)
		}
		if slices.ContainsFunc(m.Providers, func(q ProviderSpec) bool { return q.Name == p.Name }) {
			continue
		}
		m.Providers = append(m.Providers, *p)
		if p.KeyEnv != "" {
			v := env.get(p.KeyEnv)
			if v == "" {
				return nil, fmt.Errorf("%s needs its key in %s", p.Name, p.KeyEnv)
			}
			m.keys[provider.KeyName(p.Name)] = v
		}
	}
	if len(m.Run) == 0 {
		return nil, errors.New("no models to run")
	}
	return m, nil
}

// settings are the providers as config.yaml has them.
func (m *Models) settings() map[string]config.ProviderSettings {
	out := map[string]config.ProviderSettings{}
	for _, p := range m.Providers {
		out[p.Name] = config.ProviderSettings{Kind: p.Kind, BaseURL: p.BaseURL, Models: p.Models}
	}
	return out
}

// keyring holds the keys in memory for the provider registry.
func (m *Models) keyring() secret.Keyring {
	k := &memKeyring{m: map[string]string{}}
	for n, v := range m.keys {
		k.m[n] = v
	}
	return k
}

// redact replaces every key in s.
func (m *Models) redact(s string) string {
	for _, v := range m.keys {
		if len(v) >= 8 {
			s = strings.ReplaceAll(s, v, "[redacted]")
		}
	}
	return s
}

// Static are Models with set providers and keys, for tests and for
// callers that read keys themselves.
func Static(ps []ProviderSpec, keys map[string]string, run ...string) *Models {
	m := &Models{Providers: ps, Run: run, keys: map[string]string{}}
	for p, v := range keys {
		m.keys[provider.KeyName(p)] = v
	}
	return m
}

type memKeyring struct {
	mu sync.Mutex
	m  map[string]string
}

func (k *memKeyring) Get(name string) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	v, ok := k.m[name]
	if !ok {
		return "", secret.ErrNotFound
	}
	return v, nil
}

func (k *memKeyring) Set(name, v string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.m[name] = v
	return nil
}

func (k *memKeyring) Delete(name string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	delete(k.m, name)
	return nil
}
