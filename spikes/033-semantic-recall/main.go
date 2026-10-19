// Command semanticrecall is the harness of SPIKE-033: it compares word
// search (FTS5 and bm25), vector search and hybrid search (both merged with
// reciprocal rank fusion) over synthetic chat messages and file passages.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "semanticrecall:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	var (
		corpusDir  = flag.String("corpus", "corpus", "the corpus folder")
		modelsFile = flag.String("models", "models.yaml", "the embedders")
		modelList  = flag.String("model", "", "embedders by name, comma-separated, run one at a time; none runs FTS only")
		envFile    = flag.String("env", filepath.Join("..", "..", ".env"), "a .env file with the keys; its values win over the environment")
		resultsDir = flag.String("results", "results", "where vectors.db, queries.jsonl and summary.md go")
		chunkList  = flag.String("chunking", "fixed,heading", "file chunkings, comma-separated")
		check      = flag.Bool("check", false, "only load the corpus, find every quote and print the counts")
		show       = flag.String("chunks", "", "print one document's chunks (its file name) with -chunking, then stop")
		verbose    = flag.Bool("v", false, "log progress")
	)
	flag.Parse()

	var cks []chunker
	for _, n := range split(*chunkList) {
		ck, ok := chunkerByName(n)
		if !ok {
			return fmt.Errorf("-chunking: no chunking %s (fixed, heading)", n)
		}
		cks = append(cks, ck)
	}
	c, err := loadCorpus(*corpusDir)
	if err != nil {
		return err
	}
	if *show != "" {
		return printChunks(ctx, c, *show, cks)
	}
	if *check {
		return printCheck(ctx, c, cks)
	}

	var models []modelSpec
	if names := split(*modelList); len(names) > 0 {
		all, err := loadModels(*modelsFile)
		if err != nil {
			return err
		}
		if models, err = pickModels(all, names); err != nil {
			return err
		}
	}
	k, err := readKeys(*envFile)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(*resultsDir, 0o755); err != nil {
		return err
	}
	cache, err := openCache(filepath.Join(*resultsDir, "vectors.db"))
	if err != nil {
		return err
	}
	defer cache.Close()
	logf := func(string, ...any) {}
	if *verbose {
		logf = func(format string, args ...any) {
			fmt.Fprintf(os.Stderr, time.Now().Format("15:04:05 ")+format+"\n", args...)
		}
	}
	out, err := evaluate(ctx, c, options{Models: models, Keys: k, Cache: cache, Chunkings: cks, Log: logf})
	if out == nil {
		return err
	}
	// Whatever ran is written, also after Ctrl-C.
	werr := writeResults(*resultsDir, out, time.Now())
	for _, s := range out.Stops {
		fmt.Fprintf(os.Stderr, "%s stopped: %s\n", s.Model, oneLine(s.Err))
	}
	fmt.Println("summary:", filepath.Join(*resultsDir, "summary.md"))
	return errors.Join(err, werr)
}

func split(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func printCheck(ctx context.Context, c *corpus, cks []chunker) error {
	fmt.Printf("messages: %d in %d chats; %s queries\n", len(c.Messages), len(c.Chats), countKinds(c.MsgQueries))
	fmt.Printf("documents: %d; %s queries\n", len(c.Docs), countKinds(c.FileQueries))
	for _, ck := range cks {
		var n, most int
		for _, d := range c.Docs {
			cs, err := ck.Chunk(ctx, d)
			if err != nil {
				return err
			}
			n += len(cs)
			for _, x := range cs {
				most = max(most, tokens(x.Text))
			}
		}
		fmt.Printf("%s: %d chunks, the largest %d tokens\n", ck.Name(), n, most)
	}
	return nil
}

func printChunks(ctx context.Context, c *corpus, file string, cks []chunker) error {
	for _, d := range c.Docs {
		if d.File != file {
			continue
		}
		for _, ck := range cks {
			cs, err := ck.Chunk(ctx, d)
			if err != nil {
				return err
			}
			for _, x := range cs {
				fmt.Printf("=== %s %s [%d, %d) %d tokens\n%s\n\n", ck.Name(), x.ID, x.Start, x.End, tokens(x.Text), x.Text)
			}
		}
		return nil
	}
	return fmt.Errorf("no document %s in docs.yaml", file)
}
