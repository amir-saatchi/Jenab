package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
)

// allocators is a memory dump: process name → allocator → MB.
type allocators map[string]map[string]float64

// memoryDump takes one of Chromium's memory-infra dumps over every
// WebView2 process: what each allocator holds (DOM and Oilpan, layout,
// V8, font caches, Skia, …). It gives the top-level allocators, the
// parts of cc and blink_gc, and the partitions of partition_alloc, by
// effective size.
func (c *cdp) memoryDump(ctx context.Context) (allocators, error) {
	done := c.on("Tracing.tracingComplete")
	err := c.call(ctx, "Tracing.start", map[string]any{
		"traceConfig": map[string]any{
			"includedCategories": []string{"disabled-by-default-memory-infra"},
			"excludedCategories": []string{"*"},
			"memoryDumpConfig":   map[string]any{"triggers": []any{}},
		},
		"transferMode": "ReturnAsStream",
	}, nil)
	if err != nil {
		return nil, err
	}
	var dumped struct {
		Success bool `json:"success"`
	}
	if err := c.call(ctx, "Tracing.requestMemoryDump", map[string]any{"deterministic": true, "levelOfDetail": "detailed"}, &dumped); err != nil {
		return nil, err
	}
	if err := c.call(ctx, "Tracing.end", nil, nil); err != nil {
		return nil, err
	}
	if !dumped.Success {
		return nil, errors.New("the memory dump failed")
	}
	var complete struct {
		Stream string `json:"stream"`
	}
	select {
	case p := <-done:
		if err := json.Unmarshal(p, &complete); err != nil {
			return nil, err
		}
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	trace, err := c.readStream(ctx, complete.Stream)
	if err != nil {
		return nil, err
	}
	return parseDump(trace)
}

func (c *cdp) readStream(ctx context.Context, handle string) ([]byte, error) {
	defer c.call(context.WithoutCancel(ctx), "IO.close", map[string]any{"handle": handle}, nil)
	var out []byte
	for {
		var r struct {
			Base64 bool   `json:"base64Encoded"`
			Data   string `json:"data"`
			EOF    bool   `json:"eof"`
		}
		if err := c.call(ctx, "IO.read", map[string]any{"handle": handle, "size": 1 << 20}, &r); err != nil {
			return nil, err
		}
		if r.Base64 {
			b, err := base64.StdEncoding.DecodeString(r.Data)
			if err != nil {
				return nil, err
			}
			out = append(out, b...)
		} else {
			out = append(out, r.Data...)
		}
		if r.EOF {
			return out, nil
		}
	}
}

func parseDump(trace []byte) (allocators, error) {
	type attr struct {
		Value string `json:"value"`
		Units string `json:"units"`
	}
	type event struct {
		Ph   string `json:"ph"`
		Name string `json:"name"`
		PID  int    `json:"pid"`
		Args struct {
			Name  string `json:"name"`
			Dumps struct {
				Allocators map[string]struct {
					Attrs map[string]attr `json:"attrs"`
				} `json:"allocators"`
			} `json:"dumps"`
		} `json:"args"`
	}
	var t struct {
		Events []event `json:"traceEvents"`
	}
	if err := json.Unmarshal(trace, &t); err != nil {
		// Some versions write a bare array of events.
		if err2 := json.Unmarshal(trace, &t.Events); err2 != nil {
			return nil, err
		}
	}
	names := map[int]string{}
	for _, e := range t.Events {
		if e.Ph == "M" && e.Name == "process_name" {
			names[e.PID] = e.Args.Name
		}
	}
	out := allocators{}
	for _, e := range t.Events {
		if e.Ph != "v" || len(e.Args.Dumps.Allocators) == 0 {
			continue
		}
		proc := names[e.PID]
		if proc == "" {
			proc = "pid " + strconv.Itoa(e.PID)
		}
		if out[proc] == nil {
			out[proc] = map[string]float64{}
		}
		for name, a := range e.Args.Dumps.Allocators {
			top := !strings.Contains(name, "/")
			part := strings.HasPrefix(name, "partition_alloc/partitions/") && strings.Count(name, "/") == 2 ||
				(strings.HasPrefix(name, "cc/") || strings.HasPrefix(name, "blink_gc/")) && strings.Count(name, "/") == 1
			if !top && !part {
				continue
			}
			v, ok := a.Attrs["effective_size"]
			if !ok {
				v, ok = a.Attrs["size"]
			}
			if !ok || v.Units != "bytes" {
				continue
			}
			n, err := strconv.ParseUint(strings.TrimPrefix(v.Value, "0x"), 16, 64)
			if err != nil {
				continue
			}
			out[proc][name] += float64(n) / mb
		}
	}
	if len(out) == 0 {
		return nil, errors.New("the trace has no memory dump")
	}
	return out, nil
}
