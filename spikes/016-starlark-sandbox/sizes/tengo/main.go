package main

import (
	"fmt"

	"github.com/d5/tengo/v2"
)

func main() {
	c, err := tengo.NewScript([]byte("result := 1 + 1")).Run()
	fmt.Println(c.Get("result"), err)
}
