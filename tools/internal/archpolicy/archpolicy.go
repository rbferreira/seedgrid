// Package archpolicy verifies the architectural boundaries of SeedGrid.
//
// Rules:
//   - agent-no-database: the Agent cannot import database drivers
//   - agent-no-control-plane: the Agent cannot import the Control Plane
//   - control-plane-no-agent: the Control Plane cannot import the Agent
//   - agent-no-worker: the Agent cannot declare identifiers with "worker"
//   - portal-network-only-via-api: all Portal networking goes through api.ts
package archpolicy

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"
)

// Violation represents an architectural boundary violation.
type Violation struct {
	Path string
	Line int
	Rule string
	Msg  string
}

// String formats the violation as <path>:<line>: [<rule>] <message>.
func (v Violation) String() string {
	return fmt.Sprintf("%s:%d: [%s] %s", v.Path, v.Line, v.Rule, v.Msg)
}

// skipDirs are directories ignored during the scan.
var skipDirs = map[string]bool{
	"node_modules": true,
	"dist":         true,
	"bin":          true,
	".git":         true,
	"vendor":       true,
}

// forbiddenAgentImports are the imports forbidden in the Agent (rules 1 and 2).
var forbiddenAgentImports = []struct {
	prefix string
	rule   string
	msg    string
}{
	{"database/sql", "agent-no-db", "Agent cannot import database driver; only Control Plane accesses PostgreSQL"},
	{"github.com/jackc/", "agent-no-db", "Agent cannot import database driver; only Control Plane accesses PostgreSQL"},
	{"github.com/lib/pq", "agent-no-db", "Agent cannot import database driver; only Control Plane accesses PostgreSQL"},
	{"github.com/rbferreira/seedgrid/control-plane", "agent-no-control-plane", "Agent cannot import Control Plane; they communicate only via API"},
}

// Check scans the fs.FS (repository root) and applies the 5 boundary rules.
// The output order is deterministic: by path, then by line.
// A parse error for a .go is returned as an error (not a silent violation).
func Check(fsys fs.FS) ([]Violation, error) {
	var violations []Violation

	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			base := path.Base(p)
			if skipDirs[base] {
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(p, ".go") {
			vs, err := checkGoFile(fsys, p)
			if err != nil {
				return fmt.Errorf("parse de %s: %w", p, err)
			}
			violations = append(violations, vs...)
		}
		if isPortalSource(p) {
			vs, err := checkPortalFile(fsys, p)
			if err != nil {
				return fmt.Errorf("read %s: %w", p, err)
			}
			violations = append(violations, vs...)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(violations, func(i, j int) bool {
		if violations[i].Path != violations[j].Path {
			return violations[i].Path < violations[j].Path
		}
		if violations[i].Line != violations[j].Line {
			return violations[i].Line < violations[j].Line
		}
		return violations[i].Rule < violations[j].Rule
	})
	return violations, nil
}

// hasImportPrefix tells whether importPath is the prefix package or a subpackage of it. The slash is the
// boundary: "x/agent" does not match "x/agents". A prefix that already ends in "/" matches
// anything below it (e.g., "github.com/jackc/").
func hasImportPrefix(importPath, prefix string) bool {
	if strings.HasSuffix(prefix, "/") {
		return strings.HasPrefix(importPath, prefix)
	}
	return importPath == prefix || strings.HasPrefix(importPath, prefix+"/")
}

const (
	agentModule       = "github.com/rbferreira/seedgrid/agent"
	workerMsg         = "Agent cannot declare identifier with 'worker'; Worker is an internal Control Plane component"
	cpImportsAgentMsg = "Control Plane cannot import Agent; Control Plane does not communicate with Engine"
)

// checkGoFile applies the Go rules (1-4) to a single file.
func checkGoFile(fsys fs.FS, p string) ([]Violation, error) {
	data, err := fs.ReadFile(fsys, p)
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, p, data, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}

	var vs []Violation
	inAgent := strings.HasPrefix(p, "agent/")
	inCP := strings.HasPrefix(p, "control-plane/")

	// Rules 1-3: forbidden imports.
	for _, imp := range f.Imports {
		importPath := strings.Trim(imp.Path.Value, "`\"")
		line := fset.Position(imp.Pos()).Line
		if inAgent {
			for _, fb := range forbiddenAgentImports {
				if hasImportPrefix(importPath, fb.prefix) {
					vs = append(vs, Violation{Path: p, Line: line, Rule: fb.rule, Msg: fb.msg})
				}
			}
		}
		if inCP && hasImportPrefix(importPath, agentModule) {
			vs = append(vs, Violation{Path: p, Line: line, Rule: "control-plane-no-agent", Msg: cpImportsAgentMsg})
		}
	}

	// Rule 4: no DECLARED identifier with "worker" in the Agent. Only declarations — using
	// a name that came from outside (a field from another package, an assignment with "=") does not count.
	if inAgent {
		flag := func(id *ast.Ident) {
			if id != nil && containsWorker(id.Name) {
				vs = append(vs, Violation{Path: p, Line: fset.Position(id.Pos()).Line, Rule: "agent-no-worker", Msg: workerMsg})
			}
		}
		flagFields := func(fl *ast.FieldList) {
			if fl == nil {
				return
			}
			for _, fld := range fl.List {
				for _, name := range fld.Names {
					flag(name)
				}
			}
		}
		flagDefined := func(tok token.Token, exprs ...ast.Expr) {
			if tok != token.DEFINE {
				return
			}
			for _, e := range exprs {
				if id, ok := e.(*ast.Ident); ok {
					flag(id)
				}
			}
		}
		flag(f.Name) // package name
		ast.Inspect(f, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.FuncDecl: // function, method and receiver name
				flag(x.Name)
				flagFields(x.Recv)
			case *ast.FuncType: // parameters and named results (includes func literal)
				flagFields(x.Params)
				flagFields(x.Results)
			case *ast.TypeSpec:
				flag(x.Name)
			case *ast.StructType: // fields
				flagFields(x.Fields)
			case *ast.InterfaceType: // interface methods
				flagFields(x.Methods)
			case *ast.ValueSpec: // var e const
				for _, name := range x.Names {
					flag(name)
				}
			case *ast.AssignStmt: // only ":="
				flagDefined(x.Tok, x.Lhs...)
			case *ast.RangeStmt: // for k, v := range
				flagDefined(x.Tok, x.Key, x.Value)
			}
			return true
		})
	}

	return vs, nil
}

// containsWorker checks if the identifier contains "worker", ignoring case.
func containsWorker(s string) bool {
	return strings.Contains(strings.ToLower(s), "worker")
}

// isPortalSource checks if the path is a .ts/.tsx file inside portal/src/.
func isPortalSource(p string) bool {
	if !strings.HasPrefix(p, "portal/src/") {
		return false
	}
	return strings.HasSuffix(p, ".ts") || strings.HasSuffix(p, ".tsx")
}

// networkCallPattern catches forbidden network calls in the Portal outside api.ts.
// \b + optional spaces before the parenthesis, as specified.
var networkCallPattern = regexp.MustCompile(
	`\b(?:fetch|sendBeacon)\s*\(`,
)

var networkCallPatternsNew = []*regexp.Regexp{
	regexp.MustCompile(`\bnew\s+XMLHttpRequest\s*\(`),
	regexp.MustCompile(`\bnew\s+WebSocket\s*\(`),
	regexp.MustCompile(`\bnew\s+EventSource\s*\(`),
}

// isCommentLine checks if the line (without leading spaces) is a comment: starts with
// "//", with "/*" or with "*" (middle of a JSDoc/multiline block).
func isCommentLine(line string) bool {
	trimmed := strings.TrimLeft(line, " \t")
	if strings.HasPrefix(trimmed, "//") {
		return true
	}
	return strings.HasPrefix(trimmed, "*") || strings.HasPrefix(trimmed, "/*")
}

// checkPortalFile applies rule 5 to a Portal file.
func checkPortalFile(fsys fs.FS, p string) ([]Violation, error) {
	// Exceptions: api.ts and test files.
	if p == "portal/src/api.ts" {
		return nil, nil
	}
	if strings.Contains(path.Base(p), ".test.") {
		return nil, nil
	}

	data, err := fs.ReadFile(fsys, p)
	if err != nil {
		return nil, err
	}

	var vs []Violation
	lines := bytes.Split(data, []byte("\n"))
	for i, line := range lines {
		s := string(line)
		if isCommentLine(s) {
			continue
		}
		lineNum := i + 1
		if networkCallPattern.MatchString(s) {
			// We need to identify which call to give an accurate message;
			// if there is more than one, we report them all as a single violation per line.
			vs = append(vs, portalViolation(p, lineNum))
			continue
		}
		for _, re := range networkCallPatternsNew {
			if re.MatchString(s) {
				vs = append(vs, portalViolation(p, lineNum))
				break
			}
		}
	}
	return vs, nil
}

// portalViolation produces the default violation for rule 5.
func portalViolation(p string, line int) Violation {
	return Violation{
		Path: p,
		Line: line,
		Rule: "portal-network-only-via-api",
		Msg:  "all Portal network calls go through api.ts; centralize API communication there",
	}
}
