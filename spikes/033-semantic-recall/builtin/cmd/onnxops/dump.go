package main

import (
	"fmt"
	"strings"

	"github.com/gomlx/compute-onnx/support/protos"
)

// dump prints the nodes whose op type matches, with inputs, outputs and
// attributes, and the producers of their inputs one level up.
func dump(g *protos.GraphProto, op string, limit int) {
	producer := map[string]*protos.NodeProto{}
	for _, n := range g.Node {
		for _, o := range n.Output {
			producer[o] = n
		}
	}
	shown := 0
	for _, n := range g.Node {
		if n.OpType != op || shown >= limit {
			continue
		}
		shown++
		fmt.Printf("%s %s\n  in:  %s\n  out: %s\n", n.OpType, n.Name, strings.Join(n.Input, ", "), strings.Join(n.Output, ", "))
		for _, a := range n.Attribute {
			fmt.Printf("  attr %s = i:%d f:%g s:%q ints:%v\n", a.Name, a.I, a.F, a.S, a.Ints)
		}
		for _, in := range n.Input {
			if p := producer[in]; p != nil {
				fmt.Printf("  %s <- %s %s(%s)\n", in, p.OpType, p.Name, strings.Join(p.Input, ", "))
			}
		}
	}
}
