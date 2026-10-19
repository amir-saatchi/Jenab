// Command onnxops lists the op types an ONNX graph uses, with their domain
// and count, and the graph's inputs and outputs. It reads only the graph
// file, not the external weights.
//
//	go run ./cmd/onnxops models/embeddinggemma-2/model.onnx
package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/gomlx/compute-onnx/support/protos"
	"google.golang.org/protobuf/proto"
)

func main() {
	b, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var m protos.ModelProto
	if err := proto.Unmarshal(b, &m); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, o := range m.OpsetImport {
		fmt.Printf("opset %q %d\n", o.Domain, o.Version)
	}
	for _, in := range m.Graph.Input {
		fmt.Printf("input  %s %v\n", in.Name, in.Type.GetTensorType().GetShape())
	}
	for _, out := range m.Graph.Output {
		fmt.Printf("output %s %v\n", out.Name, out.Type.GetTensorType().GetShape())
	}
	if len(os.Args) > 2 {
		dump(m.Graph, os.Args[2], 3)
		return
	}
	counts := map[string]int{}
	for _, n := range m.Graph.Node {
		counts[n.Domain+"::"+n.OpType]++
	}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("%5d %s\n", counts[k], k)
	}
}
