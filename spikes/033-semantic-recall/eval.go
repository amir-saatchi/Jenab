package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"unicode/utf8"
)

// pageRunes is one page: a standard page of 1,800 characters.
const pageRunes = 1800

// item is one thing to find: a message, or a chunk of a file.
type item struct {
	ID, Text string
}

// set is what a query searches: the messages, or the files cut by one
// chunking.
type set struct {
	Name     string // messages, files/fixed, files/heading
	Use      string // the cache's use: messages or the chunking's name
	Chunking string // "" for messages
	Items    []item
	Queries  []query
	// TextBytes is the text indexed (for FTS's 0.4 × text size), Pages the
	// documents' length; 0 for messages.
	TextBytes int64
	Pages     float64
	fts       func(context.Context, string) ([]scored, error)
	chunks    map[string]chunk // files only
}

// hit reports whether an item is one of the query's targets: the message,
// or a chunk that overlaps a target's byte range.
func (s *set) hit(q query, itemID string) bool {
	for _, t := range q.Targets {
		if s.chunks == nil {
			if t.Message == itemID {
				return true
			}
			continue
		}
		c, ok := s.chunks[itemID]
		if ok && c.File == t.File && c.Start < t.End && t.Start < c.End {
			return true
		}
	}
	return false
}

// result is one query under one condition: the ranks of its targets and
// the top 5, as written to results/queries.jsonl.
type result struct {
	Set       string `json:"set"`
	Model     string `json:"model"` // "" for FTS
	Prefix    bool   `json:"prefix"`
	Form      form   `json:"form,omitempty"`
	Condition string `json:"condition"` // fts, vector, hybrid
	Query     string `json:"query"`
	Kind      string `json:"kind"`
	Lang      string `json:"lang"`
	// TargetRanks has each target's rank, from 1; 0 when it isn't in the
	// top 50. For files, a target's rank is its best overlapping chunk's.
	TargetRanks []int    `json:"target_ranks"`
	First       int      `json:"first"` // the best target rank; 0 for none
	Returned    int      `json:"returned"`
	Top5        []scored `json:"top5"`
	// VecTop is the vector list's top cosine and VecHit the cosine of its
	// first target (0 when not in the top 50), for vector and hybrid.
	VecTop float64 `json:"vec_top,omitempty"`
	VecHit float64 `json:"vec_hit,omitempty"`
}

func rankResult(s *set, q query, list []scored) result {
	r := result{Set: s.Name, Query: q.ID, Kind: q.Kind, Lang: q.Lang, Returned: len(list), TargetRanks: make([]int, len(q.Targets))}
	r.Top5 = slices.Clone(list[:min(5, len(list))])
	for ti, t := range q.Targets {
		one := query{Targets: []target{t}}
		for i, x := range list {
			if s.hit(one, x.ID) {
				r.TargetRanks[ti] = i + 1
				break
			}
		}
	}
	for i, x := range list {
		if s.hit(q, x.ID) {
			r.First = i + 1
			break
		}
	}
	return r
}

// speed is one model's embedding time for one set, from the vectors' own
// times, cached ones too.
type speed struct {
	Model  string
	Prefix bool
	Set    string
	Items  int
	NS     int64
	Pages  float64
	Fresh  int
	Query  []int64 // ns per query
}

// stop is a model that stopped, and why.
type stop struct {
	Model string
	Err   string
}

type modelInfo struct {
	Spec modelSpec
	Dims int
}

type outcome struct {
	Sets    []*set
	Models  []modelInfo
	Results []result
	Speeds  []speed
	Stops   []stop
	// MessageBytes is the messages' text, for the storage table.
	MessageBytes int64
	Messages     int
}

// buildSets loads the FTS side: a chats.db for the messages, and a chunk
// table per chunking for the files. close frees them.
func buildSets(ctx context.Context, c *corpus, tmp string, chunkings []chunker) (sets []*set, close func(), err error) {
	var closers []io.Closer
	close = func() {
		for _, c := range closers {
			c.Close()
		}
	}
	defer func() {
		if err != nil {
			close()
		}
	}()
	if len(c.Messages) > 0 {
		f, err := buildMessageFTS(ctx, filepath.Join(tmp, "chats"), c)
		if err != nil {
			return nil, close, err
		}
		closers = append(closers, f)
		s := &set{Name: "messages", Use: "messages", Queries: c.MsgQueries, TextBytes: f.bytes, fts: f.search}
		for _, m := range c.Messages {
			s.Items = append(s.Items, item{ID: m.ID, Text: m.Text})
		}
		sets = append(sets, s)
	}
	if len(c.Docs) > 0 {
		var runes int
		for _, d := range c.Docs {
			runes += utf8.RuneCountInString(d.Text)
		}
		for _, ck := range chunkings {
			s := &set{Name: "files/" + ck.Name(), Use: ck.Name(), Chunking: ck.Name(), Queries: c.FileQueries,
				Pages: float64(runes) / pageRunes, chunks: map[string]chunk{}}
			var all []chunk
			for _, d := range c.Docs {
				cs, err := ck.Chunk(ctx, d)
				if err != nil {
					return nil, close, fmt.Errorf("%s: %s: %w", ck.Name(), d.File, err)
				}
				all = append(all, cs...)
			}
			for _, ch := range all {
				s.Items = append(s.Items, item{ID: ch.ID, Text: ch.Text})
				s.chunks[ch.ID] = ch
				s.TextBytes += int64(len(ch.Text))
			}
			f, err := buildChunkFTS(ctx, filepath.Join(tmp, "chunks-"+ck.Name()+".db"), all)
			if err != nil {
				return nil, close, err
			}
			closers = append(closers, f)
			s.fts = f.search
			sets = append(sets, s)
		}
	}
	return sets, close, nil
}

type options struct {
	Models    []modelSpec
	Keys      keys
	Cache     *vectorCache
	Chunkings []chunker
	Log       func(format string, args ...any)
	// newEmbedder is replaced in tests.
	newEmbedder func(modelSpec, keys) (embedder, error)
}

// evaluate runs every condition over every query: FTS once per set, then
// for each model, prefix setting, set and form, vector and hybrid.
func evaluate(ctx context.Context, c *corpus, o options) (*outcome, error) {
	if o.newEmbedder == nil {
		o.newEmbedder = newEmbedder
	}
	if o.Log == nil {
		o.Log = func(string, ...any) {}
	}
	tmp, err := os.MkdirTemp("", "spike033-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	sets, closeSets, err := buildSets(ctx, c, tmp, o.Chunkings)
	if err != nil {
		return nil, err
	}
	defer closeSets()
	out := &outcome{Sets: sets, Messages: len(c.Messages)}
	for _, m := range c.Messages {
		out.MessageBytes += int64(len(m.Text))
	}

	ftsLists := map[string]map[string][]scored{} // set → query → list
	for _, s := range sets {
		ftsLists[s.Name] = map[string][]scored{}
		for _, q := range s.Queries {
			l, err := s.fts(ctx, q.Query)
			if err != nil {
				return nil, fmt.Errorf("%s: fts %s: %w", s.Name, q.ID, err)
			}
			ftsLists[s.Name][q.ID] = l
			r := rankResult(s, q, l)
			r.Condition = "fts"
			out.Results = append(out.Results, r)
		}
		o.Log("%s: fts over %d items, %d queries", s.Name, len(s.Items), len(s.Queries))
	}

	for _, spec := range o.Models {
		e, err := o.newEmbedder(spec, o.Keys)
		if err != nil {
			out.Stops = append(out.Stops, stop{Model: spec.Name, Err: err.Error()})
			o.Log("%s: %v", spec.Name, err)
			continue
		}
		if b, ok := e.(*builtinEmbedder); ok {
			b.log = o.Log
		}
		dims, err := evaluateModel(ctx, spec, e, sets, ftsLists, o, out)
		if err := release(e); err != nil {
			o.Log("%s: close: %v", spec.Name, err)
		}
		out.Models = append(out.Models, modelInfo{Spec: spec, Dims: dims})
		if err != nil {
			if ctx.Err() != nil {
				return out, ctx.Err()
			}
			out.Stops = append(out.Stops, stop{Model: spec.Name, Err: err.Error()})
			o.Log("%s: stopped: %v", spec.Name, err)
		}
	}
	return out, nil
}

func evaluateModel(ctx context.Context, spec modelSpec, e embedder, sets []*set, ftsLists map[string]map[string][]scored, o options, out *outcome) (int, error) {
	prefixes := []bool{false}
	if spec.hasPrefixes() {
		prefixes = append(prefixes, true)
	}
	dims := 0
	model := spec.Name + "/" + spec.Model
	for _, withPrefix := range prefixes {
		docPrefix, queryPrefix := "", ""
		if withPrefix {
			docPrefix, queryPrefix = spec.DocumentPrefix, spec.QueryPrefix
		}
		for _, s := range sets {
			texts := make([]string, len(s.Items))
			ids := make([]string, len(s.Items))
			for i, it := range s.Items {
				texts[i], ids[i] = it.Text, it.ID
			}
			o.Log("%s: embedding %d items of %s (prefix %v)", spec.Name, len(texts), s.Name, withPrefix)
			docs, err := embedAll(ctx, e, o.Cache, cacheKey{model, s.Use, withPrefix}, docPrefix, texts, spec.BatchSize)
			if err == nil {
				err = release(e)
			}
			if err != nil {
				return dims, err
			}
			qtexts := make([]string, len(s.Queries))
			for i, q := range s.Queries {
				qtexts[i] = q.Query
			}
			sp := speed{Model: spec.Name, Prefix: withPrefix, Set: s.Name, Items: len(texts), NS: docs.ns, Pages: s.Pages, Fresh: docs.fresh}
			qvecs := make([][]float32, len(qtexts))
			for i, t := range qtexts {
				one, err := embedAll(ctx, e, o.Cache, cacheKey{model, "query", withPrefix}, queryPrefix, []string{t}, 1)
				if err != nil {
					return dims, err
				}
				qvecs[i] = one.vecs[0]
				sp.Query = append(sp.Query, one.ns)
			}
			if err := release(e); err != nil {
				return dims, err
			}
			out.Speeds = append(out.Speeds, sp)
			if dims, err = width(dims, docs.vecs, qvecs); err != nil {
				return dims, fmt.Errorf("%s: %w", s.Name, err)
			}
			for _, f := range formsFor(spec, dims) {
				ix := newIndex(f, ids, docs.vecs)
				for i, q := range s.Queries {
					vec := ix.search(qvecs[i], topN)
					hyb := rrf(topN, ftsLists[s.Name][q.ID], vec)
					var vecTop, vecHit float64
					if len(vec) > 0 {
						vecTop = vec[0].Score
					}
					for _, x := range vec {
						if s.hit(q, x.ID) {
							vecHit = x.Score
							break
						}
					}
					for _, l := range []struct {
						cond string
						list []scored
					}{{"vector", vec}, {"hybrid", hyb}} {
						r := rankResult(s, q, l.list)
						r.Model, r.Prefix, r.Form, r.Condition, r.VecTop, r.VecHit = spec.Name, withPrefix, f, l.cond, vecTop, vecHit
						out.Results = append(out.Results, r)
					}
				}
			}
		}
	}
	return dims, nil
}

// release frees a model that loads, such as the built-in one, after each
// embedding job.
func release(e embedder) error {
	if l, ok := e.(loader); ok {
		return l.release()
	}
	return nil
}

var errWidth = errors.New("vectors of different widths")

func width(dims int, sets ...[][]float32) (int, error) {
	for _, vs := range sets {
		for _, v := range vs {
			if dims == 0 {
				dims = len(v)
			}
			if len(v) != dims {
				return dims, fmt.Errorf("%w: %d and %d (delete results/vectors.db if the model changed)", errWidth, dims, len(v))
			}
		}
	}
	return dims, nil
}
