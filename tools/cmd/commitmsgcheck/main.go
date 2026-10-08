// commitmsgcheck checks the SeedGrid commit message policy.
//
// Usage:
//
//	go run ./cmd/commitmsgcheck -file <path>           (hook commit-msg)
//	go run ./cmd/commitmsgcheck -range <a..b>          (CI check)
//
// Exit codes: 0 = ok, 1 = violation, 2 = usage/git error.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/rbferreira/seedgrid/tools/internal/commitmsg"
)

// rangePattern validates the range before passing it to git: refs, SHAs, `..`/`...` and the
// suffixes `~N`/`^` (CI uses `<sha>~1..<sha>` on a new branch and force-push). Starting with an
// alphanumeric prevents the value from being read as a git option (e.g.: `--output=...`).
var rangePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_./^~-]*$`)

type commit struct {
	sha string
	msg string
}

// parseLog splits the git log output into commits.
// git log is called with --format=%H%x1f%B%x1e:
// SHA (40 hex), separator \x1f, message body, separator \x1e.
func parseLog(out string) []commit {
	var commits []commit
	for _, entry := range strings.Split(out, "\x1e") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.SplitN(entry, "\x1f", 2)
		if len(parts) != 2 {
			continue
		}
		commits = append(commits, commit{sha: parts[0], msg: parts[1]})
	}
	return commits
}

func main() {
	fileFlag := flag.String("file", "", "commit message file path (commit-msg hook)")
	rangeFlag := flag.String("range", "", "git log range (e.g.: BASE..HEAD)")
	flag.Parse()

	if (*fileFlag == "" && *rangeFlag == "") || (*fileFlag != "" && *rangeFlag != "") {
		fmt.Fprintln(os.Stderr, "usage: commitmsgcheck -file <path> OR -range <a..b>")
		os.Exit(2)
	}

	if *fileFlag != "" {
		os.Exit(checkFile(*fileFlag))
	}
	os.Exit(checkRange(*rangeFlag))
}

// checkFile reads the file, removes git comments and checks the message.
func checkFile(path string) int {
	//nolint:gosec // G304: path is the $1 argument of the commit-msg hook, controlled by git itself.
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading %s: %v\n", path, err)
		return 2
	}
	msg := commitmsg.StripComments(string(data))
	violations := commitmsg.Check(msg)
	if len(violations) == 0 {
		return 0
	}
	for _, v := range violations {
		fmt.Fprintln(os.Stderr, v)
	}
	return 1
}

// checkRange runs git log on the range and checks each commit.
func checkRange(r string) int {
	if !rangePattern.MatchString(r) {
		fmt.Fprintf(os.Stderr, "invalid range: %s\n", r)
		return 2
	}
	// The final "--" closes the revision list: the range is never read as a path.
	//nolint:gosec // G204: range validated by strict regex above, no shell in between.
	cmd := exec.Command("git", "log", "--format=%H%x1f%B%x1e", r, "--")
	out, err := cmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error running git log: %v\n", err)
		return 2
	}
	commits := parseLog(string(out))
	exitCode := 0
	for _, c := range commits {
		vs := commitmsg.Check(c.msg)
		for _, v := range vs {
			short := c.sha
			if len(short) > 7 {
				short = short[:7]
			}
			fmt.Fprintf(os.Stderr, "%s: %s\n", short, v)
			exitCode = 1
		}
	}
	return exitCode
}
