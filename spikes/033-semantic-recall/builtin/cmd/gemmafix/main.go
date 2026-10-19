// Command gemmafix rewrites the graph of onnx-community/embeddinggemma-2-ONNX
// (fp32 text tower) so hugot's Go backend can load it. It touches only the
// graph file; the weights stay in the same model.onnx_data.
//
//   - com.microsoft SkipSimplifiedLayerNormalization, which onnx-gomlx lacks,
//     becomes Add plus SimplifiedLayerNormalization, which it has.
//
//   - com.microsoft RotaryEmbedding gets position_ids [0], which ONNX Runtime
//     reads as a start offset, so token i takes row i of the cos and sin
//     cache. onnx-gomlx instead gathers row 0 for every token, which drops
//     the positions. The cache here is already one row per position, so
//     the rewrite leaves position_ids out and onnx-gomlx uses the cache as is.
//
//   - The image, video and audio feature inputs, which hugot's text pipeline
//     can't fill, become constant [1, 512] zero tensors. Text has no
//     <|image|> or <|audio|> tokens, so the merge never reads them.
//
//     go run ./cmd/gemmafix models/embeddinggemma-2/model.onnx models/embeddinggemma-2/model_go.onnx
package main

import (
	"fmt"
	"os"

	"github.com/gomlx/compute-onnx/support/protos"
	"google.golang.org/protobuf/proto"
)

func main() {
	in, out := os.Args[1], os.Args[2]
	b, err := os.ReadFile(in)
	check(err)
	var m protos.ModelProto
	check(proto.Unmarshal(b, &m))
	g := m.Graph

	var nodes []*protos.NodeProto
	replaced := 0
	rotary := 0
	for _, n := range g.Node {
		if n.Domain == "com.microsoft" && n.OpType == "RotaryEmbedding" {
			if n.Input[1] != "/model/constants/INT64/[0]" {
				check(fmt.Errorf("%s: position_ids %q is not the constant [0]", n.Name, n.Input[1]))
			}
			n.Input[1] = ""
			rotary++
		}
		if n.Domain != "com.microsoft" || n.OpType != "SkipSimplifiedLayerNormalization" {
			nodes = append(nodes, n)
			continue
		}
		replaced++
		// Inputs: input, skip, gamma, [bias]. Outputs: output, mean, inv_std_var, input_skip_bias_sum.
		sum := n.Name + "/sum"
		if len(n.Output) > 3 && n.Output[3] != "" {
			sum = n.Output[3]
		}
		nodes = append(nodes, &protos.NodeProto{
			Name: n.Name + "/Add", OpType: "Add",
			Input: []string{n.Input[0], n.Input[1]}, Output: []string{sum},
		})
		if len(n.Input) > 3 && n.Input[3] != "" {
			biased := sum + "/bias"
			nodes[len(nodes)-1].Output = []string{biased}
			nodes = append(nodes, &protos.NodeProto{
				Name: n.Name + "/AddBias", OpType: "Add",
				Input: []string{biased, n.Input[3]}, Output: []string{sum},
			})
		}
		var attrs []*protos.AttributeProto
		for _, a := range n.Attribute {
			if a.Name == "epsilon" {
				attrs = append(attrs, a)
			}
		}
		attrs = append(attrs, &protos.AttributeProto{Name: "axis", Type: protos.AttributeProto_INT, I: -1})
		nodes = append(nodes, &protos.NodeProto{
			Name: n.Name + "/RMSNorm", OpType: "SimplifiedLayerNormalization",
			Input: []string{sum, n.Input[2]}, Output: []string{n.Output[0]}, Attribute: attrs,
		})
	}
	g.Node = nodes

	var inputs []*protos.ValueInfoProto
	for _, v := range g.Input {
		switch v.Name {
		case "image_features", "video_features", "audio_features":
			g.Initializer = append(g.Initializer, &protos.TensorProto{
				Name: v.Name, Dims: []int64{1, 512},
				DataType:  int32(protos.TensorProto_FLOAT),
				FloatData: make([]float32, 512),
			})
		default:
			inputs = append(inputs, v)
		}
	}
	g.Input = inputs

	empty := 0
	for _, n := range g.Node {
		for _, o := range n.Output {
			if o == "" {
				empty++
			}
		}
	}
	b, err = proto.MarshalOptions{Deterministic: true}.Marshal(&m)
	check(err)
	check(os.WriteFile(out, b, 0o644))
	fmt.Printf("replaced %d SkipSimplifiedLayerNormalization nodes; dropped position_ids from %d RotaryEmbedding nodes; %d inputs left; %d empty output names left\n",
		replaced, rotary, len(g.Input), empty)
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
