package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"time"

	_ "modernc.org/sqlite"
)

// vectorCache is results/vectors.db: the model's raw vectors, per model,
// use (messages, a chunking, or queries), prefix setting and the sha256 of
// the text as sent, so a rerun doesn't embed again. The 256 and int8 forms
// are made from the full vector when loaded. Each vector keeps its share of
// its batch's time, so the speed measures survive a rerun.
type vectorCache struct {
	db *sql.DB
}

func openCache(path string) (*vectorCache, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS vectors (
		model TEXT NOT NULL,
		use TEXT NOT NULL,
		prefix INTEGER NOT NULL,
		sha TEXT NOT NULL,
		dims INTEGER NOT NULL,
		vec BLOB NOT NULL,
		ns INTEGER NOT NULL,
		PRIMARY KEY (model, use, prefix, sha)
	) WITHOUT ROWID`); err != nil {
		db.Close()
		return nil, err
	}
	return &vectorCache{db: db}, nil
}

func (c *vectorCache) Close() error { return c.db.Close() }

func textSHA(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func encodeVec(v []float32) []byte {
	b := make([]byte, 4*len(v))
	for i, x := range v {
		binary.LittleEndian.PutUint32(b[4*i:], math.Float32bits(x))
	}
	return b
}

func decodeVec(b []byte) []float32 {
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[4*i:]))
	}
	return v
}

type cacheKey struct {
	model, use string
	prefix     bool
}

func (c *vectorCache) get(ctx context.Context, k cacheKey, sha string) ([]float32, int64, bool, error) {
	var b []byte
	var ns int64
	err := c.db.QueryRowContext(ctx, `SELECT vec, ns FROM vectors WHERE model = ? AND use = ? AND prefix = ? AND sha = ?`,
		k.model, k.use, k.prefix, sha).Scan(&b, &ns)
	if err == sql.ErrNoRows {
		return nil, 0, false, nil
	}
	return decodeVec(b), ns, err == nil, err
}

func (c *vectorCache) put(ctx context.Context, k cacheKey, shas []string, vecs [][]float32, ns int64) error {
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i, v := range vecs {
		if _, err := tx.Exec(`INSERT OR REPLACE INTO vectors (model, use, prefix, sha, dims, vec, ns) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			k.model, k.use, k.prefix, shas[i], len(v), encodeVec(v), ns); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// embedded is a set of texts' vectors and the time they took.
type embedded struct {
	vecs  [][]float32
	ns    int64 // the sum of each vector's share of its batch's time
	fresh int   // embedded in this run, not read from the cache
}

// embedAll embeds texts with the prefix put in front, batch by batch, and
// reads the vectors already in the cache. A batch of 1 times each text
// alone, as for queries.
func embedAll(ctx context.Context, e embedder, c *vectorCache, k cacheKey, prefix string, texts []string, batch int) (embedded, error) {
	out := embedded{vecs: make([][]float32, len(texts))}
	shas := make([]string, len(texts))
	var todo []int
	for i, t := range texts {
		shas[i] = textSHA(prefix + t)
		v, ns, ok, err := c.get(ctx, k, shas[i])
		if err != nil {
			return out, err
		}
		if ok {
			out.vecs[i], out.ns = v, out.ns+ns
			continue
		}
		todo = append(todo, i)
	}
	for len(todo) > 0 {
		n := min(batch, len(todo))
		part := todo[:n]
		todo = todo[n:]
		in := make([]string, n)
		ss := make([]string, n)
		for j, i := range part {
			in[j], ss[j] = prefix+texts[i], shas[i]
		}
		start := time.Now()
		vecs, err := e.Embed(ctx, in)
		if err != nil {
			return out, fmt.Errorf("%s: %w", e.Name(), err)
		}
		if len(vecs) != n {
			return out, fmt.Errorf("%s: %d vectors for %d texts", e.Name(), len(vecs), n)
		}
		ns := time.Since(start).Nanoseconds() / int64(n)
		if err := c.put(ctx, k, ss, vecs, ns); err != nil {
			return out, err
		}
		for j, i := range part {
			out.vecs[i] = vecs[j]
		}
		out.ns += ns * int64(n)
		out.fresh += n
	}
	return out, nil
}
