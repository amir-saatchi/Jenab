// SPIKE-016: can go.starlark.net run script.starlark safely, or is another
// embedded language better?
//
// Usage: go run . > results.md
//
// Hostile scripts run in child processes (this binary with "child ...") inside
// a Windows Job Object with a memory cap and a hard timeout.
package main

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "child":
			childMain(os.Args[2:])
			return
		case "one": // one <engine> <case> <mode>: debug a single hostile run
			o := runChild(os.Args[2], os.Args[3], os.Args[4])
			fmt.Printf("%+v\n", o)
			return
		case "section": // section <name>: print one part of the report
			map[string]func(){"hostile": hostileTables, "probe": probeTable, "db": dbTable, "transform": transformTable,
				"det": determinismTable, "cost": costTables}[os.Args[2]]()
			return
		case "eval": // eval <engine> <file>: run a script in-process with the DB
			db, err := openDB()
			if err != nil {
				panic(err)
			}
			src, _ := os.ReadFile(os.Args[3])
			ctx, cancel := context.WithTimeoutCause(context.Background(), runTimeout, errTimeout)
			defer cancel()
			v, err := engineByName(os.Args[2]).Run(ctx, string(src), RunOpts{DB: db})
			fmt.Printf("%v\nerr=%v\n", v, err)
			return
		}
	}
	report()
}

func report() {
	fmt.Printf("# SPIKE-016 results\n\n")
	fmt.Printf("Go %s, %s/%s, %d CPUs, run on %s.\n\n", runtime.Version(), runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), time.Now().Format("2006-01-02"))
	printVersions()
	hostileTables()
	probeTable()
	dbTable()
	transformTable()
	determinismTable()
	costTables()
}
