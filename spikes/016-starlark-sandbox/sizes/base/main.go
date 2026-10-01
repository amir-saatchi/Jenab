// Size baseline: the same program without an engine.
package main

import (
	"fmt"
	"os"
)

func main() { fmt.Println(len(os.Args) + 1) }
