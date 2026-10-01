package main

import (
	"fmt"

	"github.com/dop251/goja"
)

func main() {
	v, err := goja.New().RunString("1 + 1")
	fmt.Println(v, err)
}
