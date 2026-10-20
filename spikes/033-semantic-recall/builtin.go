package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"slices"
	"time"

	"burrow/spikes/semanticrecall/builtin"
)

// builtinEmbedder runs the model in builtin/ inside this process, through
// hugot's pure-Go backend. It loads the model before a job that has texts
// to embed, and frees it after the job: the Go backend holds 8 to 12 GB
// while it embeds, until the model is closed. A run that finds every
// vector in the cache never loads it.
type builtinEmbedder struct {
	spec modelSpec
	e    *builtin.Embedder
	dims int
	log  func(format string, args ...any)
}

func newBuiltin(m modelSpec) (*builtinEmbedder, error) {
	if m.Dir == "" {
		return nil, fmt.Errorf("%s: no model folder (dir)", m.Name)
	}
	if _, err := os.Stat(filepath.Join(m.Dir, "tokenizer.json")); err != nil {
		return nil, fmt.Errorf("%s: no model in %s: %w", m.Name, m.Dir, err)
	}
	return &builtinEmbedder{spec: m, log: func(string, ...any) {}}, nil
}

func (b *builtinEmbedder) Name() string { return b.spec.Name }
func (b *builtinEmbedder) Dims() int    { return b.dims }

// load is called by embedAll before a job's first batch, so the load time
// stays out of the embedding time. It is logged instead.
func (b *builtinEmbedder) load(ctx context.Context) error {
	if b.e != nil {
		return nil
	}
	start := time.Now()
	e, err := builtin.New(ctx, b.spec.Dir, builtin.Config{
		Name:      b.spec.Model,
		OnnxFile:  b.spec.OnnxFile,
		Output:    b.spec.Output,
		Pooling:   b.spec.Pooling,
		BatchSize: b.spec.BatchSize,
	})
	if err != nil {
		return err
	}
	b.e, b.dims = e, e.Dims()
	b.log("%s: loaded in %s, %d wide", b.spec.Name, time.Since(start).Round(time.Millisecond), b.dims)
	return nil
}

func (b *builtinEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if err := b.load(ctx); err != nil {
		return nil, err
	}
	if b.spec.MaxTokens > 0 {
		texts = slices.Clone(texts)
		for i, t := range texts {
			if c, ok := b.cut(t); ok {
				b.log("%s: a text of %d tokens cut to %d bytes of %d", b.spec.Name, len(b.e.TokenIDs(t)), len(c), len(t))
				texts[i] = c
			}
		}
	}
	return b.e.Embed(ctx, texts)
}

// cut shortens a text the Go tokenizer makes longer than MaxTokens to its
// longest start that fits. hugot asks the Go tokenizer for at most
// max_position_embeddings tokens, but the limit isn't applied, and the
// model fails on a longer text. The app would have to cut it too.
func (b *builtinEmbedder) cut(t string) (string, bool) {
	if len(b.e.TokenIDs(t)) <= b.spec.MaxTokens {
		return t, false
	}
	r := []rune(t)
	lo, hi := 0, len(r) // the longest fitting prefix is in [lo, hi)
	for hi-lo > 1 {
		mid := (lo + hi) / 2
		if len(b.e.TokenIDs(string(r[:mid]))) <= b.spec.MaxTokens {
			lo = mid
		} else {
			hi = mid
		}
	}
	return string(r[:lo]), true
}

// release closes the model after a job and gives its memory back.
func (b *builtinEmbedder) release() error {
	if b.e == nil {
		return nil
	}
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	err := b.e.Close()
	b.e = nil
	debug.FreeOSMemory()
	b.log("%s: closed; the Go runtime held %.1f GB from the OS before", b.spec.Name, float64(ms.Sys)/(1<<30))
	return err
}

// Embedders that load a model before a job and free it after one.
type loader interface {
	load(ctx context.Context) error
	release() error
}

// cachedEmbedder stands for vectors another tool wrote into vectors.db,
// such as reftok.py's. It can't embed: a text not in the cache
// stops the model.
type cachedEmbedder struct{ spec modelSpec }

func (c cachedEmbedder) Name() string { return c.spec.Name }
func (c cachedEmbedder) Dims() int    { return 0 }

var errNotCached = errors.New("not in vectors.db; this model's vectors are written by another tool")

func (c cachedEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return nil, fmt.Errorf("%d texts are %w", len(texts), errNotCached)
}
