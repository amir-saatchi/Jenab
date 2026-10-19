// Package builtin runs a small embedding model inside the process through
// hugot's pure-Go backend (no cgo). It matches the spike's Embedder shape:
// Name, Dims and Embed. Texts go in as given; vectors come back as the model
// gives them, and the caller normalizes them.
package builtin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/knights-analytics/hugot"
	"github.com/knights-analytics/hugot/options"
	"github.com/knights-analytics/hugot/pipelines"
)

// Config picks the model files inside the model folder.
type Config struct {
	// Name is what Name() returns, for example "embeddinggemma-2-int8".
	Name string
	// OnnxFile is the .onnx file inside the folder, relative to it, for
	// example "model_go.onnx". Empty means the only .onnx file.
	OnnxFile string
	// Output is the model output to read. Empty means "sentence_embedding"
	// when the model has it, else the first output, mean pooled.
	Output string
	// Pooling is how a token-level output becomes one vector: "mean" (the
	// default, what hugot does) or "cls" (the first token, as
	// granite-embedding's sentence-transformers config asks). A model output
	// that is already one vector per text is used as it is.
	Pooling string
	// BatchSize is the most texts sent to the model in one call. 0 means 32.
	BatchSize int
	// SequenceBuckets are the padded sequence lengths the Go backend compiles
	// for. Empty means hugot's default (32, 128, 512).
	SequenceBuckets []int
}

// Embedder is one loaded model. It is safe for use by one goroutine at a time;
// Embed takes a lock.
type Embedder struct {
	name      string
	dims      int
	batchSize int
	cls       bool

	mu       sync.Mutex
	session  *hugot.Session
	pipeline *pipelines.FeatureExtractionPipeline
}

// New loads the model in dir with hugot's Go backend. The folder holds the
// .onnx file (and its .onnx_data, if any) and tokenizer.json.
func New(ctx context.Context, dir string, cfg Config) (*Embedder, error) {
	if _, err := os.Stat(filepath.Join(dir, "tokenizer.json")); err != nil {
		return nil, fmt.Errorf("builtin: no tokenizer.json in %s: %w", dir, err)
	}
	var opts []options.WithOption
	if len(cfg.SequenceBuckets) > 0 {
		opts = append(opts, options.WithGoMLXSequenceBuckets(cfg.SequenceBuckets))
	}
	session, err := hugot.NewGoSession(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("builtin: go session: %w", err)
	}
	e, err := newWithSession(session, dir, cfg)
	if err != nil {
		_ = session.Destroy()
		return nil, err
	}
	return e, nil
}

func newWithSession(session *hugot.Session, dir string, cfg Config) (*Embedder, error) {
	output := cfg.Output
	if output == "" {
		output = "sentence_embedding"
	}
	p, err := newPipeline(session, dir, cfg.OnnxFile, output)
	if err != nil && cfg.Output == "" && strings.Contains(err.Error(), "is not available") {
		p, err = newPipeline(session, dir, cfg.OnnxFile, "")
	}
	if err != nil {
		return nil, fmt.Errorf("builtin: load %s: %w", dir, err)
	}
	dims := p.Output.Dimensions
	if len(dims) == 0 || dims[len(dims)-1] <= 0 {
		return nil, fmt.Errorf("builtin: output %q has no fixed width: %v", p.Output.Name, dims)
	}
	name := cfg.Name
	if name == "" {
		name = filepath.Base(dir)
	}
	batch := cfg.BatchSize
	if batch <= 0 {
		batch = 32
	}
	if cfg.Pooling != "" && cfg.Pooling != "mean" && cfg.Pooling != "cls" {
		return nil, fmt.Errorf("builtin: unknown pooling %q", cfg.Pooling)
	}
	return &Embedder{
		cls:       cfg.Pooling == "cls" && len(dims) == 3,
		name:      name,
		dims:      int(dims[len(dims)-1]),
		batchSize: batch,
		session:   session,
		pipeline:  p,
	}, nil
}

func newPipeline(session *hugot.Session, dir, onnxFile, output string) (*pipelines.FeatureExtractionPipeline, error) {
	cfg := hugot.FeatureExtractionConfig{
		ModelPath:    dir,
		OnnxFilename: onnxFile,
		Name:         "embed-" + output,
	}
	if output != "" {
		cfg.Options = append(cfg.Options, pipelines.WithOutputName(output))
	}
	return hugot.NewPipeline(session, cfg)
}

// Name returns the configured model name.
func (e *Embedder) Name() string { return e.name }

// Dims returns the width of the vectors Embed returns.
func (e *Embedder) Dims() int { return e.dims }

// OutputName returns the model output the vectors come from.
func (e *Embedder) OutputName() string { return e.pipeline.Output.Name }

// Embed returns one vector per text, in batches of BatchSize.
func (e *Embedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.pipeline == nil {
		return nil, errors.New("builtin: embedder is closed")
	}
	out := make([][]float32, 0, len(texts))
	for start := 0; start < len(texts); start += e.batchSize {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		end := min(start+e.batchSize, len(texts))
		vecs, err := e.run(ctx, texts[start:end])
		if err != nil {
			return nil, fmt.Errorf("builtin: embed: %w", err)
		}
		if len(vecs) != end-start {
			return nil, fmt.Errorf("builtin: got %d vectors for %d texts", len(vecs), end-start)
		}
		for _, v := range vecs {
			out = append(out, append([]float32(nil), v...))
		}
	}
	return out, nil
}

func (e *Embedder) run(ctx context.Context, texts []string) ([][]float32, error) {
	if !e.cls {
		res, err := e.pipeline.RunPipeline(ctx, texts)
		if err != nil {
			return nil, err
		}
		return res.Embeddings, nil
	}
	res, err := e.pipeline.RunRaw(ctx, texts)
	if err != nil {
		return nil, err
	}
	vecs := make([][]float32, len(res.HiddenStates))
	for i, tokens := range res.HiddenStates {
		vecs[i] = tokens[0]
	}
	return vecs, nil
}

// Close frees the model. The app unloads it when idle.
func (e *Embedder) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.session == nil {
		return nil
	}
	err := e.session.Destroy()
	e.session, e.pipeline = nil, nil
	return err
}

// TokenIDs returns the ids the Go tokenizer gives text, special tokens
// included, to check them against the reference tokenizer.
func (e *Embedder) TokenIDs(text string) []int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.pipeline.Model.Tokenizer.GoTokenizer.Tokenizer.EncodeWithAnnotations(text).IDs
}
