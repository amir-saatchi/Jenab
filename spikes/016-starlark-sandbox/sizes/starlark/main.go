package main

import (
	"fmt"

	"go.starlark.net/starlark"
)

func main() {
	g, err := starlark.ExecFile(&starlark.Thread{}, "x.star", "result = 1 + 1", nil)
	fmt.Println(g["result"], err)
}
