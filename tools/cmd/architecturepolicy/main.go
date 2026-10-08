// architecturepolicy checks the architectural boundaries of SeedGrid.
//
// Usage:
//
//	go run ./cmd/architecturepolicy -root ..   (from the tools/ directory)
//
// Exit codes: 0 = ok, 1 = violation, 2 = error (invalid root, parse).
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/rbferreira/seedgrid/tools/internal/archpolicy"
)

func main() {
	rootFlag := flag.String("root", "..", "repository root directory")
	flag.Parse()

	info, err := os.Stat(*rootFlag)
	if err != nil || !info.IsDir() {
		fmt.Fprintf(os.Stderr, "invalid root: %s\n", *rootFlag)
		os.Exit(2)
	}

	violations, err := archpolicy.Check(os.DirFS(*rootFlag))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}

	if len(violations) == 0 {
		fmt.Println("architecturepolicy: ok")
		os.Exit(0)
	}

	for _, v := range violations {
		fmt.Fprintln(os.Stderr, v)
	}
	os.Exit(1)
}
