package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
)

// The agent loop Burrow owns. It streams a response, writes the assistant message and its
// tool_call parts to the (fake) chats.db, and only then runs tools (SPEC 2.3 "message before change").

type FakeDB struct{ Log []string }

func (d *FakeDB) add(format string, a ...any) { d.Log = append(d.Log, fmt.Sprintf(format, a...)) }

// libraryRanTool counts tool executions that did not come from our loop (a library calling a Go
// function it was handed). It must stay 0.
var libraryRanTool atomic.Int64

type ToolFunc func(args json.RawMessage) []Part

type LoopResult struct {
	Final    *Response
	Events   []Event
	Requests int
	DB       *FakeDB
	Usage    []Usage
}

// placeCachePoints sets the two SPEC 3.1 cache points: after context block 5 and at the end of
// the latest message. It clears older message cache points so a request never has more than two.
func placeCachePoints(req *Request) {
	for i := range req.System {
		req.System[i].Cache = i == 4
	}
	for i := range req.Messages {
		for j := range req.Messages[i].Parts {
			req.Messages[i].Parts[j].Cache = false
		}
	}
	if n := len(req.Messages); n > 0 {
		last := &req.Messages[n-1]
		if k := len(last.Parts); k > 0 {
			last.Parts[k-1].Cache = true
		}
	}
}

func runLoop(ctx context.Context, p Provider, req *Request, tools map[string]ToolFunc, cache bool, maxTurns int) (*LoopResult, error) {
	res := &LoopResult{DB: &FakeDB{}}
	for turn := 0; turn < maxTurns; turn++ {
		if cache {
			placeCachePoints(req)
		}
		resp, err := p.Stream(ctx, req, func(e Event) { res.Events = append(res.Events, e) })
		res.Requests++
		if err != nil {
			return res, err
		}
		res.Usage = append(res.Usage, resp.Usage)
		var calls []Part
		for _, pt := range resp.Message.Parts {
			if pt.Type == PToolCall {
				calls = append(calls, pt)
			}
		}
		// 1. message before change: write the assistant message and its tool_call parts first.
		res.DB.add("write assistant message (%d parts, %d tool_call)", len(resp.Message.Parts), len(calls))
		req.Messages = append(req.Messages, resp.Message)
		res.Final = resp
		if len(calls) == 0 {
			return res, nil
		}
		// 2. only now run the tools.
		var results []Part
		for _, c := range calls {
			f := tools[c.Name]
			if f == nil {
				results = append(results, Part{Type: PToolResult, CallID: c.CallID, IsError: true, Content: []Part{{Type: PText, Text: "unknown tool"}}})
				continue
			}
			res.DB.add("run tool %s %s", c.Name, c.CallID)
			out := f(json.RawMessage(c.Args))
			res.DB.add("write tool_result %s", c.CallID)
			results = append(results, Part{Type: PToolResult, CallID: c.CallID, Content: out})
		}
		req.Messages = append(req.Messages, Message{Role: "tool", Parts: results})
	}
	return res, fmt.Errorf("max turns reached")
}

// messageBeforeChange checks the fake DB log: every "run tool" comes after a "write assistant message".
func messageBeforeChange(db *FakeDB) bool {
	wrote := false
	for _, l := range db.Log {
		if len(l) > 5 && l[:5] == "write" && !wrote {
			wrote = true
		}
		if len(l) > 3 && l[:3] == "run" && !wrote {
			return false
		}
	}
	return true
}
