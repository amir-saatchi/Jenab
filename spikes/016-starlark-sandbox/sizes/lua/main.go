package main

import (
	"fmt"

	lua "github.com/yuin/gopher-lua"
)

func main() {
	L := lua.NewState()
	defer L.Close()
	err := L.DoString("return 1 + 1")
	fmt.Println(L.Get(-1), err)
}
