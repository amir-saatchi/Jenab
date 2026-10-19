// Command measure loads a built-in embedding model through hugot's Go backend
// and measures it: load time, memory while embedding, throughput over about
// 100 pages in batches of 32, and the time for one query. It can also compare
// the vectors with Ollama's for the same texts.
//
//	go run ./cmd/measure -dir models/embeddinggemma-2 -onnx model_go.onnx probe
//	go run ./cmd/measure -dir ... bench
//	go run ./cmd/measure -dir ... -ollama embeddinggemma-2:270m ollama
package main

import (
	"context"
	"flag"
	"fmt"
	"math"
	"os"
	"runtime"
	"runtime/debug"
	"slices"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"burrow/spikes/semanticrecall/builtin"
)

func main() {
	dir := flag.String("dir", "", "model folder")
	onnx := flag.String("onnx", "", "onnx file inside the folder")
	name := flag.String("name", "", "model name")
	output := flag.String("output", "", "model output to read")
	pooling := flag.String("pooling", "", "mean or cls")
	qPrefix := flag.String("qprefix", "", "query prefix for the persian mode")
	pPrefix := flag.String("pprefix", "", "passage prefix for the persian mode")
	pages := flag.Int("pages", 100, "pages to embed in bench")
	batch := flag.Int("batch", 32, "batch size")
	ollamaModel := flag.String("ollama", "embeddinggemma-2:270m", "Ollama model for the ollama mode")
	flag.Parse()
	mode := flag.Arg(0)
	if mode == "texts" {
		if err := dumpTexts(flag.Arg(1), *pages); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if *dir == "" || mode == "" {
		fmt.Fprintln(os.Stderr, "usage: measure -dir DIR [-onnx FILE] probe|bench|reference|persian|ollama")
		os.Exit(2)
	}
	ctx := context.Background()
	fmt.Printf("machine: %s, %d logical CPUs, GOMAXPROCS %d, %s/%s, %s\n",
		cpuName(), runtime.NumCPU(), runtime.GOMAXPROCS(0), runtime.GOOS, runtime.GOARCH, runtime.Version())
	cfg := builtin.Config{Name: *name, OnnxFile: *onnx, Output: *output, Pooling: *pooling, BatchSize: *batch}

	start := time.Now()
	e, err := builtin.New(ctx, *dir, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	load := time.Since(start)
	defer e.Close()
	fmt.Printf("model: %s, output %s, %d wide\n", e.Name(), e.OutputName(), e.Dims())
	fmt.Printf("load: %s; memory after load: %s\n", load.Round(time.Millisecond), memLine())

	switch mode {
	case "probe":
		probe(ctx, e)
	case "bench":
		bench(ctx, e, *pages, *batch)
	case "tokens":
		dumpTokens(e, flag.Arg(1))
	case "vectors":
		dumpVectors(ctx, e, flag.Arg(1))
	case "persian":
		persian(ctx, e, *qPrefix, *pPrefix)
	case "reference":
		reference(ctx, e)
	case "ollama":
		compareOllama(ctx, e, *ollamaModel)
	default:
		fmt.Fprintln(os.Stderr, "unknown mode", mode)
		os.Exit(2)
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	return v
}

// probe embeds the queries and a few sentences and prints which sentence each
// query is closest to, as a quick check that the vectors mean something.
func probe(ctx context.Context, e *builtin.Embedder) {
	docs := []string{
		sentences["en"][1], sentences["en"][11], sentences["en"][8],
		sentences["de"][15], sentences["de"][2], sentences["de"][20],
		sentences["fa"][16], sentences["fa"][10], sentences["fa"][3],
	}
	start := time.Now()
	qv := must(e.Embed(ctx, queries))
	dv := must(e.Embed(ctx, docs))
	fmt.Printf("first calls (compile included): %s\n", time.Since(start).Round(time.Millisecond))
	fmt.Printf("norm of first vector: %.4f\n", norm(qv[0]))
	hits := 0
	for i, q := range qv {
		best, bestScore := -1, -2.0
		for j, d := range dv {
			if s := cosine(q, d); s > bestScore {
				best, bestScore = j, s
			}
		}
		ok := best == i
		if ok {
			hits++
		}
		fmt.Printf("  %-45.45s -> %-45.45s %.3f %v\n", queries[i], docs[best], bestScore, ok)
	}
	fmt.Printf("probe: %d/%d queries closest to their sentence\n", hits, len(qv))
}

// bench measures single-query time and throughput over the pages, sampling
// the process memory while it embeds.
func bench(ctx context.Context, e *builtin.Embedder, pages, batch int) {
	// Warm up: the Go backend compiles a graph per batch and sequence bucket.
	start := time.Now()
	must(e.Embed(ctx, queries[:1]))
	fmt.Printf("first query (compile included): %s\n", time.Since(start).Round(time.Millisecond))

	var lat []time.Duration
	for range 3 {
		for _, q := range queries {
			t := time.Now()
			must(e.Embed(ctx, []string{q}))
			lat = append(lat, time.Since(t))
		}
	}
	slices.Sort(lat)
	fmt.Printf("single query, %d runs: p50 %s, p95 %s, max %s\n", len(lat),
		lat[len(lat)/2].Round(time.Millisecond), lat[len(lat)*95/100].Round(time.Millisecond), lat[len(lat)-1].Round(time.Millisecond))

	texts := chunks(pages, 33)
	chars := 0
	for _, t := range texts {
		chars += len([]rune(t))
	}
	// Warm the big-batch, long-sequence graph first, so throughput is steady state.
	start = time.Now()
	must(e.Embed(ctx, texts[:batch]))
	fmt.Printf("first batch of %d chunks (compile included): %s\n", batch, time.Since(start).Round(time.Millisecond))

	var peak atomic.Uint64
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		t := time.NewTicker(100 * time.Millisecond)
		defer t.Stop()
		for {
			if ws := workingSet(); ws > peak.Load() {
				peak.Store(ws)
			}
			select {
			case <-stop:
				return
			case <-t.C:
			}
		}
	}()
	var cpu0 = cpuTime()
	start = time.Now()
	vecs := must(e.Embed(ctx, texts))
	took := time.Since(start)
	cpu := cpuTime() - cpu0
	close(stop)
	<-done
	fmt.Printf("throughput: %d pages, %d chunks, %d chars in %s: %.1f chunks/s, %s per 100 pages; CPU time %s (%.1f cores busy)\n",
		pages, len(texts), chars, took.Round(time.Millisecond), float64(len(texts))/took.Seconds(),
		(took * 100 / time.Duration(pages)).Round(time.Second), cpu.Round(time.Second), cpu.Seconds()/took.Seconds())
	fmt.Printf("memory while embedding: peak working set %s, peak private %s; now %s\n", mb(peak.Load()), mb(peakPrivate()), memLine())
	// Most of the heap is GoMLX's sync.Pool of buffers and garbage; two GCs
	// free the pool, so this is what the loaded model keeps.
	runtime.GC()
	runtime.GC()
	debug.FreeOSMemory()
	fmt.Printf("memory after GC: %s\n", memLine())
	// The app unloads the model when idle; this is what closing gives back.
	if err := e.Close(); err != nil {
		fmt.Println("close:", err)
	}
	runtime.GC()
	runtime.GC()
	debug.FreeOSMemory()
	fmt.Printf("memory after Close and GC: %s\n", memLine())
	_ = vecs
}

func norm(v []float32) float64 {
	var s float64
	for _, x := range v {
		s += float64(x) * float64(x)
	}
	return math.Sqrt(s)
}

func cosine(a, b []float32) float64 {
	var dot float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
	}
	return dot / (norm(a) * norm(b))
}

func mb(b uint64) string { return fmt.Sprintf("%.0f MB", float64(b)/(1<<20)) }

func memLine() string {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return fmt.Sprintf("working set %s, private %s, Go heap in use %s, Go sys %s",
		mb(workingSet()), mb(privateBytes()), mb(m.HeapInuse), mb(m.Sys))
}

func summarize(xs []float64) string {
	s := slices.Clone(xs)
	sort.Float64s(s)
	return fmt.Sprintf("min %.5f, p50 %.5f, max %.5f", s[0], s[len(s)/2], s[len(s)-1])
}

func short(s string, n int) string {
	r := []rune(strings.ReplaceAll(s, "\n", " "))
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
