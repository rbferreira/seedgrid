// languagepolicy checks that SeedGrid's code stays in English (#64).
//
// Usage:
//
//	go run ./cmd/languagepolicy -root ..   (from the tools/ directory)
//
// Exceptions live in tools/language-allow.txt, one per line, each with its reason.
// Exit codes: 0 = ok, 1 = Portuguese found, 2 = error.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rbferreira/seedgrid/tools/internal/langpolicy"
)

func main() {
	rootFlag := flag.String("root", "..", "repository root directory")
	allowFlag := flag.String("allow", "language-allow.txt", "exceptions file")
	flag.Parse()

	info, err := os.Stat(*rootFlag)
	if err != nil || !info.IsDir() {
		fmt.Fprintf(os.Stderr, "invalid root: %s\n", *rootFlag)
		os.Exit(2)
	}
	data, err := os.ReadFile(filepath.Clean(*allowFlag))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}
	allow, err := langpolicy.ParseAllow(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}
	findings, err := langpolicy.Check(os.DirFS(*rootFlag), allow)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}
	if len(findings) == 0 {
		fmt.Println("languagepolicy: ok")
		return
	}
	for _, f := range findings {
		fmt.Fprintln(os.Stderr, f)
	}
	fmt.Fprintf(os.Stderr, "languagepolicy: %d finding(s) — translate them, or add a justified exception to tools/language-allow.txt\n", len(findings))
	os.Exit(1)
}
