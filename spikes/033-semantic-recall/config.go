package main

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/amir-saatchi/jenab/internal/scenario"
)

// modelSpec is one embedder in models.yaml. The kind is chosen, never
// guessed from the base URL.
type modelSpec struct {
	Name    string `yaml:"name"`
	Kind    string `yaml:"kind"` // ollama, openai_compatible, builtin, cached, fake
	BaseURL string `yaml:"base_url"`
	Model   string `yaml:"model"`
	KeyEnv  string `yaml:"key_env"`
	// BatchSize is the most texts in one request.
	BatchSize int `yaml:"batch_size"`
	// Matryoshka is set for models trained to be cut to a smaller width;
	// only they get the 256 run.
	Matryoshka bool `yaml:"matryoshka"`
	// The prefixes from the model's card, used only in the prefix run.
	QueryPrefix    string `yaml:"query_prefix"`
	DocumentPrefix string `yaml:"document_prefix"`
	// Dims is the fake embedder's width; the others report their own.
	Dims int `yaml:"dims"`
	// The built-in embedder's settings. Dir is the model folder; a relative
	// one is inside -builtin-models. The other three pick the graph, its
	// output and the pooling, as builtin.Config does.
	Dir      string `yaml:"dir"`
	OnnxFile string `yaml:"onnx_file"`
	Output   string `yaml:"output"`
	Pooling  string `yaml:"pooling"`
	// MaxTokens cuts a longer text to fit the model's window (built-in only).
	MaxTokens int `yaml:"max_tokens"`
}

var kinds = []string{"ollama", "openai_compatible", "builtin", "cached", "fake"}

func (m modelSpec) hasPrefixes() bool { return m.QueryPrefix != "" || m.DocumentPrefix != "" }

type modelsFile struct {
	Embedders []modelSpec `yaml:"embedders"`
}

func loadModels(path string) ([]modelSpec, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f modelsFile
	if err := yaml.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	seen := map[string]bool{}
	for i, m := range f.Embedders {
		switch {
		case m.Name == "":
			return nil, fmt.Errorf("%s: embedder %d has no name", path, i+1)
		case seen[m.Name]:
			return nil, fmt.Errorf("%s: two embedders are named %s", path, m.Name)
		case !slices.Contains(kinds, m.Kind):
			return nil, fmt.Errorf("%s: %s: kind must be one of %s", path, m.Name, strings.Join(kinds, ", "))
		case (m.Kind == "ollama" || m.Kind == "openai_compatible") && (m.BaseURL == "" || m.Model == ""):
			return nil, fmt.Errorf("%s: %s: a %s embedder needs base_url and model", path, m.Name, m.Kind)
		case m.Kind == "builtin" && m.Dir == "":
			return nil, fmt.Errorf("%s: %s: a builtin embedder needs dir, its model folder", path, m.Name)
		}
		if m.BatchSize <= 0 {
			f.Embedders[i].BatchSize = 32
		}
		seen[m.Name] = true
	}
	return f.Embedders, nil
}

func pickModels(all []modelSpec, names []string) ([]modelSpec, error) {
	var out []modelSpec
	for _, n := range names {
		i := slices.IndexFunc(all, func(m modelSpec) bool { return m.Name == n })
		if i < 0 {
			return nil, fmt.Errorf("no embedder %s in the models file", n)
		}
		out = append(out, all[i])
	}
	return out, nil
}

// keys reads the .env file once. Its values win over the environment, and
// they stay in memory.
type keys map[string]string

func readKeys(envFile string) (keys, error) {
	if envFile == "" {
		return keys{}, nil
	}
	k, err := scenario.ReadEnvFile(envFile)
	if errors.Is(err, os.ErrNotExist) {
		return keys{}, nil
	}
	return keys(k), err
}

func (k keys) get(name string) string {
	if v := k[name]; v != "" {
		return v
	}
	return os.Getenv(name)
}
