// Package langpolicy keeps SeedGrid's code in English (#64).
//
// The product speaks several languages through its catalogs; the code a contributor reads does
// not. This checker flags Portuguese in:
//   - Go: comments, string literals of non-test code, the messages and names that tests print
//     (t.Errorf, t.Run…) and function names;
//   - TypeScript: comments and test names (describe/it/test). The Portal's own guard
//     (no-literals.test.ts) already covers interface text;
//   - shell, YAML, SQL, CSS and config files: comments, and quoted strings in shell.
//
// What is Portuguese on purpose stays out: the pt-BR catalogs and docs, and site profiles. A legitimate exception elsewhere (a Brazilian site's category name that the code
// must recognize, a sample title) goes in language-allow.txt with its reason.
package langpolicy

import (
	"bufio"
	"bytes"
	_ "embed"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Finding is one piece of Portuguese outside the allowed places.
type Finding struct {
	Path string
	Line int
	Kind string
	Text string // clipped, for display
	full string // the whole text, for matching exceptions
}

func (f Finding) String() string {
	return fmt.Sprintf("%s:%d: %s in Portuguese: %s", f.Path, f.Line, f.Kind, f.Text)
}

// Allow is one accepted exception: Text found in a line of Path (a file or a directory prefix).
type Allow struct {
	Path, Text, Reason string
}

// skipped are the places that are Portuguese on purpose.
var skipped = []string{
	"docs/", // docs/ is checked for structure parity; docs/pt-BR/ is Portuguese by design
	"portal/src/i18n/pt-BR/",
	"control-plane/internal/notify/catalog.go",
	"profiles/",
	"node_modules/", "dist/", "bin/", ".git/", ".claude/", ".graphify/", "graphify-out/",
}

// ptWord matches letters only Portuguese uses, or common Portuguese words that English never
// uses. Boundaries are explicit because RE2's \b is ASCII-only.
var ptWord = regexp.MustCompile(`(?i)(?:^|[^\p{L}])(n[ãa]o|voc[êe]|tamb[ée]m|ent[ãa]o|est[áa]|s[ãa]o|até|ap[óo]s|nenhuma?|deveria[m]?|quer|falhou|falha|obrigat[óo]ri[oa]|quando|porque|pelo|pela|isso|isto|ainda|sempre|nunca|já|só|uma|dos|das|seu|sua|est[ãa]o|tem|t[êe]m|foi|ser[áa]|aqui|agora|depois|antes|sem|com o|com a|para o|para a|do Node|da fila|ao lado)(?:$|[^\p{L}])|[ãõçÃÕÇ]|\p{L}(?:ári[oa]s?|óri[oa]s?|ênci[ao]s?|ânci[ao]s?|ível|áve(?:l|is)|ês)(?:$|[^\p{L}])`)

//go:embed ptwords.txt
var ptWordList string

// ptWords is the vocabulary of Portuguese words found in the code (#64): a word of the list,
// alone or inside an identifier (camelCase, snake_case), reads as Portuguese — which is what
// catches the identifiers and short phrases the pattern above cannot (no accent, no common word).
var ptWords = func() map[string]bool {
	m := map[string]bool{}
	for _, line := range strings.Split(ptWordList, "\n") {
		if w := strings.TrimSpace(line); w != "" && !strings.HasPrefix(w, "#") {
			m[w] = true
		}
	}
	return m
}()

var wordRe = regexp.MustCompile(`\p{L}+`)

// IsPortuguese reports whether text reads as Portuguese.
func IsPortuguese(text string) bool {
	if ptWord.MatchString(text) {
		return true
	}
	for _, w := range wordRe.FindAllString(splitCamel(text), -1) {
		if ptWords[strings.ToLower(w)] {
			return true
		}
	}
	return false
}

// ParseAllow reads language-allow.txt: "path<TAB>text<TAB>reason" per line; # comments.
func ParseAllow(data []byte) ([]Allow, error) {
	var out []Allow
	sc := bufio.NewScanner(bytes.NewReader(data))
	n := 0
	for sc.Scan() {
		n++
		line := sc.Text()
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != 3 || parts[0] == "" || parts[1] == "" || strings.TrimSpace(parts[2]) == "" {
			return nil, fmt.Errorf("language-allow.txt:%d: expected path<TAB>text<TAB>reason", n)
		}
		out = append(out, Allow{Path: parts[0], Text: parts[1], Reason: parts[2]})
	}
	return out, sc.Err()
}

// Check walks fsys and returns the findings that no Allow covers.
func Check(fsys fs.FS, allow []Allow) ([]Finding, error) {
	var out []Finding
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := p
		for _, s := range skipped {
			if rel == strings.TrimSuffix(s, "/") || strings.HasPrefix(rel, s) || strings.Contains(rel, "/"+s) {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
		}
		if d.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		found, err := checkFile(rel, data)
		if err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
		for _, f := range found {
			if !allowed(f, data, allow) {
				out = append(out, f)
			}
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Line < out[j].Line
	})
	return out, err
}

func allowed(f Finding, data []byte, allow []Allow) bool {
	lines := strings.Split(string(data), "\n")
	line := ""
	if f.Line >= 1 && f.Line <= len(lines) {
		line = lines[f.Line-1]
	}
	for _, a := range allow {
		if (f.Path == a.Path || strings.HasPrefix(f.Path, strings.TrimSuffix(a.Path, "/")+"/")) &&
			(strings.Contains(line, a.Text) || strings.Contains(f.full, a.Text)) {
			return true
		}
	}
	return false
}

func checkFile(rel string, data []byte) ([]Finding, error) {
	base := path.Base(rel)
	switch ext := path.Ext(rel); {
	case ext == ".go":
		return checkGo(rel, data)
	case ext == ".ts" || ext == ".tsx":
		return checkTS(rel, data), nil
	case ext == ".css":
		return blockComments(rel, data, "/*", "*/"), nil
	case ext == ".sql":
		return lineComments(rel, data, "--"), nil
	case ext == ".sh":
		return append(lineComments(rel, data, "#"), shellStrings(rel, data)...), nil
	case ext == ".service" || ext == ".timer":
		return append(lineComments(rel, data, "#"), matches(rel, data, unitDescription, "unit description")...), nil
	case ext == ".yml" || ext == ".yaml" || ext == ".example" || ext == ".conf" || ext == ".rules" || ext == ".txt" ||
		strings.HasPrefix(base, "Dockerfile") || base == ".gitignore" || base == ".dockerignore":
		return lineComments(rel, data, "#"), nil
	case ext == ".html":
		return append(lineComments(rel, data, "//"), blockComments(rel, data, "<!--", "-->")...), nil
	case ext == ".json":
		return matches(rel, data, jsonComment, "comment"), nil
	}
	return nil, nil
}

var (
	unitDescription = regexp.MustCompile(`(?m)^Description=(.*)$`)
	// jsonComment is the "$comment" a contract file (shared/*.json) carries for whoever opens it.
	jsonComment = regexp.MustCompile(`"\$comment"\s*:\s*"((?:\\.|[^"\\])*)"`)
)

// matches reports the first group of each match of re that reads as Portuguese.
func matches(rel string, data []byte, re *regexp.Regexp, kind string) []Finding {
	var out []Finding
	s := string(data)
	for _, m := range re.FindAllStringSubmatchIndex(s, -1) {
		if text := s[m[2]:m[3]]; IsPortuguese(text) {
			out = append(out, Finding{rel, strings.Count(s[:m[2]], "\n") + 1, kind, clip(text), text})
		}
	}
	return out
}

func clip(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 120 {
		s = string(r[:120]) + "…"
	}
	return s
}

// ---- Go

var testReport = map[string]bool{"Errorf": true, "Fatalf": true, "Error": true, "Fatal": true, "Logf": true, "Log": true,
	"Skipf": true, "Skip": true, "Run": true}

func checkGo(rel string, data []byte) ([]Finding, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, rel, data, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	var out []Finding
	for _, cg := range f.Comments {
		for _, c := range cg.List {
			if IsPortuguese(c.Text) {
				out = append(out, Finding{rel, fset.Position(c.Pos()).Line, "comment", clip(c.Text), c.Text})
			}
		}
	}
	isTest := strings.HasSuffix(rel, "_test.go")
	reported := map[token.Pos]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncDecl:
			if IsPortuguese(splitCamel(x.Name.Name)) {
				out = append(out, Finding{rel, fset.Position(x.Pos()).Line, "function name", x.Name.Name, x.Name.Name})
			}
		case *ast.ValueSpec:
			out = append(out, idents(rel, fset, x.Names)...)
		case *ast.TypeSpec:
			out = append(out, idents(rel, fset, []*ast.Ident{x.Name})...)
		case *ast.Field:
			out = append(out, idents(rel, fset, x.Names)...)
		case *ast.AssignStmt:
			if x.Tok == token.DEFINE {
				for _, e := range x.Lhs {
					if id, ok := e.(*ast.Ident); ok {
						out = append(out, idents(rel, fset, []*ast.Ident{id})...)
					}
				}
			}
		case *ast.RangeStmt:
			if x.Tok == token.DEFINE {
				for _, e := range []ast.Expr{x.Key, x.Value} {
					if id, ok := e.(*ast.Ident); ok {
						out = append(out, idents(rel, fset, []*ast.Ident{id})...)
					}
				}
			}
		case *ast.CallExpr:
			if !isTest || len(x.Args) == 0 {
				return true
			}
			if sel, ok := x.Fun.(*ast.SelectorExpr); ok && testReport[sel.Sel.Name] {
				if lit, ok := x.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
					reported[lit.Pos()] = true
					if s, err := strconv.Unquote(lit.Value); err == nil && IsPortuguese(s) {
						out = append(out, Finding{rel, fset.Position(lit.Pos()).Line, "test message", clip(s), s})
					}
				}
			}
		case *ast.BasicLit:
			if isTest || x.Kind != token.STRING || reported[x.Pos()] {
				return true
			}
			if s, err := strconv.Unquote(x.Value); err == nil && IsPortuguese(s) {
				out = append(out, Finding{rel, fset.Position(x.Pos()).Line, "string", clip(s), s})
			}
		}
		return true
	})
	return out, nil
}

// idents reports the declared names that read as Portuguese.
func idents(rel string, fset *token.FileSet, ids []*ast.Ident) []Finding {
	var out []Finding
	for _, id := range ids {
		if id == nil || id.Name == "_" {
			continue
		}
		if IsPortuguese(splitCamel(id.Name)) {
			out = append(out, Finding{rel, fset.Position(id.Pos()).Line, "identifier", id.Name, id.Name})
		}
	}
	return out
}

var camel = regexp.MustCompile(`([a-z0-9])([A-Z])`)

func splitCamel(name string) string {
	return strings.ReplaceAll(camel.ReplaceAllString(name, "$1 $2"), "_", " ")
}

// ---- TypeScript

// tsTestName captures the first argument of describe/it/test, in any of the three quotes (RE2
// has no backreferences: one alternative per quote).
var tsTestName = regexp.MustCompile(`\b(?:describe|it|test)(?:\.(?:skip|only|each\([^)]*\)))?\(\s*(?:"((?:\\.|[^"\\])*)"|'((?:\\.|[^'\\])*)'|` + "`((?:\\\\.|[^`\\\\])*)`" + `)`)

// tsDecl captures declared names: const/let/var (also destructured), function, class, type,
// interface and enum.
var tsDecl = regexp.MustCompile(`\b(?:const|let|var)\s+(?:([A-Za-z_$][\w$]*)|\{([^}]*)\}|\[([^\]]*)\])|\b(?:function|class|type|interface|enum)\s+([A-Za-z_$][\w$]*)`)
var tsName = regexp.MustCompile(`[A-Za-z_$][\w$]*`)

func checkTS(rel string, data []byte) []Finding {
	out := append(lineComments(rel, data, "//"), blockComments(rel, data, "/*", "*/")...)
	for _, m := range tsDecl.FindAllSubmatchIndex(data, -1) {
		for g := 1; g <= 4; g++ {
			if m[2*g] < 0 {
				continue
			}
			for _, name := range tsName.FindAllString(string(data[m[2*g]:m[2*g+1]]), -1) {
				if IsPortuguese(splitCamel(name)) {
					out = append(out, Finding{rel, lineOf(data, m[0]), "identifier", name, name})
				}
			}
		}
	}
	if strings.Contains(rel, ".test.") {
		for _, m := range tsTestName.FindAllSubmatchIndex(data, -1) {
			for g := 1; g <= 3; g++ {
				if m[2*g] < 0 {
					continue
				}
				if name := string(data[m[2*g]:m[2*g+1]]); IsPortuguese(name) {
					out = append(out, Finding{rel, lineOf(data, m[0]), "test name", clip(name), name})
				}
			}
		}
	}
	return out
}

// ---- generic comment scanners (line based: good enough for these file types, and the
// guard reports — a false positive is solved in the allow list with a reason)

func lineComments(rel string, data []byte, marker string) []Finding {
	var out []Finding
	for i, line := range strings.Split(string(data), "\n") {
		k := commentStart(line, marker)
		if k < 0 {
			continue
		}
		if text := line[k:]; IsPortuguese(text) {
			out = append(out, Finding{rel, i + 1, "comment", clip(text), text})
		}
	}
	return out
}

// commentStart finds the marker outside quotes, at line start or after whitespace.
func commentStart(line, marker string) int {
	var q byte
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case q != 0:
			switch c {
			case '\\':
				i++
			case q:
				q = 0
			}
		case c == '"' || c == '\'' || c == '`':
			q = c
		case strings.HasPrefix(line[i:], marker) && (i == 0 || line[i-1] == ' ' || line[i-1] == '\t'):
			if marker == "#" && strings.HasPrefix(line[i:], "#!") {
				return -1
			}
			return i
		}
	}
	return -1
}

func blockComments(rel string, data []byte, open, close string) []Finding {
	var out []Finding
	s := string(data)
	for i := strings.Index(s, open); i >= 0; {
		j := strings.Index(s[i+len(open):], close)
		end := len(s)
		if j >= 0 {
			end = i + len(open) + j + len(close)
		}
		if text := s[i:end]; IsPortuguese(text) {
			out = append(out, Finding{rel, strings.Count(s[:i], "\n") + 1, "comment", clip(text), text})
		}
		k := strings.Index(s[end:], open)
		if k < 0 {
			break
		}
		i = end + k
	}
	return out
}

var shString = regexp.MustCompile(`"((?:\\.|[^"\\])*)"`)

func shellStrings(rel string, data []byte) []Finding {
	var out []Finding
	for i, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		code := line
		if k := commentStart(line, "#"); k >= 0 {
			code = line[:k]
		}
		for _, m := range shString.FindAllStringSubmatch(code, -1) {
			if IsPortuguese(m[1]) {
				out = append(out, Finding{rel, i + 1, "string", clip(m[1]), m[1]})
			}
		}
	}
	return out
}

func lineOf(data []byte, off int) int { return bytes.Count(data[:off], []byte("\n")) + 1 }
