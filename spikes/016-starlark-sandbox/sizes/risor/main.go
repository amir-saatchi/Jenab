package main

import (
	"context"
	"fmt"

	"github.com/deepnoodle-ai/risor/v2"
)

func main() {
	v, err := risor.Eval(context.Background(), "1 + 1", risor.WithEnv(risor.Builtins()))
	fmt.Println(v, err)
}
