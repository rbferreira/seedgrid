package messages

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const (
	moduleRoot = "../.."                                   // control-plane/
	exportPath = "../../../shared/messages.json"           // read by the Portal contract test
	rulesPath  = "../../../deploy/observability/rules.yml" // Alertmanager alert codes
)

func TestConsistentRecord(t *testing.T) {
	seen := map[string]bool{}
	for _, m := range Registry {
		id := string(m.Kind) + "/" + m.Code
		if seen[id] {
			t.Errorf("%s appears twice in the registry", id)
		}
		seen[id] = true
		if m.Payload == nil {
			t.Errorf("%s: Payload nil — no keys, use []string{}", id)
		}
		if (m.Variant == "") != (len(m.Variants) == 0) {
			t.Errorf("%s: Variant and Variants must be set together", id)
		}
		if m.Variant != "" && !contains(m.Payload, m.Variant) {
			t.Errorf("%s: variant key (%s) must be in Payload", id, m.Variant)
		}
		if m.Generic && m.Kind != Error {
			t.Errorf("%s: Generic is only valid for the error track", id)
		}
	}
}

// TestExportedRecord: shared/messages.json is the registry, for the Portal to check the catalog.
// To regenerate after changing the registry: UPDATE_MESSAGES=1 go test ./internal/messages
func TestExportedRecord(t *testing.T) {
	want, err := json.MarshalIndent(Registry, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	want = append(want, '\n')
	if os.Getenv("UPDATE_MESSAGES") == "1" {
		if err := os.WriteFile(exportPath, want, 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatalf("read %s: %v (generate with UPDATE_MESSAGES=1 go test ./internal/messages)", exportPath, err)
	}
	if !bytes.Equal(bytes.ReplaceAll(got, []byte("\r\n"), []byte("\n")), want) {
		t.Errorf("%s is out of date with the registry: run UPDATE_MESSAGES=1 go test ./internal/messages", exportPath)
	}
}

// emission is a point in the code that records a code with a payload.
type emission struct {
	kind  Kind
	code  string
	keys  map[string]bool // payload keys that the point records; nil = could not read
	where string
}

// TestAllCodeEmitted reads the Control Plane source code and verifies, in both directions, that the
// registry is what is actually recorded: every emitted code has an entry; every entry has someone
// emitting it; and every emission point records in the payload the keys that the registry promises.
func TestAllCodeEmitted(t *testing.T) {
	ems := scanEmissions(t)
	for _, c := range alertmanagerCodes(t) {
		ems = append(ems, emission{kind: Alert, code: c, keys: map[string]bool{"instance": true, "source": true}, where: rulesPath})
	}

	byID := map[string]Message{}
	for _, m := range Registry {
		byID[string(m.Kind)+"/"+m.Code] = m
	}
	emitted := map[string]bool{}
	for _, e := range ems {
		id := string(e.kind) + "/" + e.code
		emitted[id] = true
		m, ok := byID[id]
		if !ok {
			t.Errorf("%s: emits %s, which is not in messages.Registry — add the entry (and the phrase in the Portal catalogs)", e.where, id)
			continue
		}
		if e.kind == Audit && e.keys == nil {
			// params built by a handler function (the Node's own value, the
			// alias): the test does not read them; the reader falls back to the recorded text if data is missing.
			continue
		}
		if e.kind == Error {
			// error.params are optional (the error can come without the numbers): the reader falls back to
			// error.message when one is missing; the test here only checks the set of codes.
			continue
		}
		if e.keys == nil {
			t.Errorf("%s: %s — the test could not read the payload keys; store it as a map literal or local variable", e.where, id)
			continue
		}
		for _, k := range m.Payload {
			if !e.keys[k] {
				t.Errorf("%s: %s does not store %q in the payload, which the registry promises (and the phrase uses)", e.where, id, k)
			}
		}
	}
	for id := range byID {
		if !emitted[id] {
			t.Errorf("%s is in the registry, but nothing in the Control Plane emits it", id)
		}
	}
}

var codeLit = regexp.MustCompile(`^[A-Z][A-Z0-9_]{3,}$`)

func scanEmissions(t *testing.T) []emission {
	t.Helper()
	fset := token.NewFileSet()
	kinds := activityKinds(t, fset)
	var out []emission
	for _, dir := range []string{"internal", "cmd"} {
		err := filepath.Walk(filepath.Join(moduleRoot, dir), func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			f, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return err
			}
			for _, d := range f.Decls {
				if g, ok := d.(*ast.GenDecl); ok {
					out = append(out, scanRouteActions(fset, g)...)
					continue
				}
				fn, ok := d.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				out = append(out, scanFunc(fset, fn, kinds)...)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return out
}

// payloadArg: position of the payload in each method that records an alert or event.
var payloadArg = map[string]int{"Emit": 5, "Open": 5, "OpenEpisode": 6, "UpdatePayload": 6}

func scanFunc(fset *token.FileSet, fn *ast.FuncDecl, kinds map[string]string) []emission {
	var out []emission
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CallExpr:
			// WriteError from within the httpx package itself is called without a selector.
			if id, ok := x.Fun.(*ast.Ident); ok {
				x.Fun = &ast.SelectorExpr{X: ast.NewIdent("httpx"), Sel: id}
			}
			sel, ok := x.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if sel.Sel.Name == "SetAudit" && len(x.Args) >= 4 {
				if b, ok := x.Args[1].(*ast.BasicLit); ok {
					if code, err := strconv.Unquote(b.Value); err == nil {
						var keys map[string]bool
						if id, ok := x.Args[2].(*ast.Ident); ok && id.Name == "nil" {
							keys = map[string]bool{}
						} else {
							keys = mapKeys(fn, x.Args[2])
						}
						out = append(out, emission{Audit, code, keys, fset.Position(x.Pos()).String()})
					}
				}
				return true
			}
			if sel.Sel.Name == "RecordAction" && len(x.Args) >= 5 {
				if b, ok := x.Args[3].(*ast.BasicLit); ok {
					if code, err := strconv.Unquote(b.Value); err == nil && code != "" {
						out = append(out, emission{Audit, code, map[string]bool{}, fset.Position(x.Pos()).String()})
					}
				}
				return true
			}
			if (sel.Sel.Name == "WriteError" || sel.Sel.Name == "WriteErrorParams") && len(x.Args) >= 4 {
				if b, ok := x.Args[2].(*ast.BasicLit); ok {
					if code, err := strconv.Unquote(b.Value); err == nil {
						out = append(out, emission{Error, code, nil, fset.Position(x.Pos()).String()})
					}
				}
				return true
			}
			idx, ok := payloadArg[sel.Sel.Name]
			if !ok || len(x.Args) <= idx {
				return true
			}
			kind := Event
			if sel.Sel.Name != "Emit" {
				kind = Alert
			}
			keys := mapKeys(fn, x.Args[idx])
			where := fset.Position(x.Pos()).String()
			switch c := x.Args[1].(type) {
			case *ast.BasicLit:
				if code, err := strconv.Unquote(c.Value); err == nil && codeLit.MatchString(code) {
					out = append(out, emission{kind, code, keys, where})
				}
			case *ast.SelectorExpr:
				// Code coming from a local table (e.g.: spec.code in torrent.Command): every
				// code literal in the function is valid.
				for _, code := range codeLiterals(fn) {
					out = append(out, emission{kind, code, keys, where})
				}
			}
		case *ast.CompositeLit:
			if !isActivityEntry(x.Type) {
				return true
			}
			var code string
			var keys map[string]bool
			for _, el := range x.Elts {
				kv, ok := el.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				switch ident(kv.Key) {
				case "Kind":
					code = kinds[ident(kv.Value)]
				case "Detail":
					keys = mapKeys(fn, kv.Value)
				}
			}
			if keys == nil {
				keys = map[string]bool{} // entry without Detail: empty payload
			}
			if code != "" {
				out = append(out, emission{Activity, code, keys, fset.Position(x.Pos()).String()})
			}
		}
		return true
	})
	return out
}

// scanRouteActions: the codes from the routeActions table (httpx), the Audit action derived from the
// route — `{code: "route_…", text: "…", match: …}`.
func scanRouteActions(fset *token.FileSet, g *ast.GenDecl) []emission {
	var out []emission
	for _, sp := range g.Specs {
		vs, ok := sp.(*ast.ValueSpec)
		if !ok || len(vs.Names) == 0 || vs.Names[0].Name != "routeActions" || len(vs.Values) == 0 {
			continue
		}
		ast.Inspect(vs.Values[0], func(n ast.Node) bool {
			kv, ok := n.(*ast.KeyValueExpr)
			if !ok || ident(kv.Key) != "code" {
				return true
			}
			if b, ok := kv.Value.(*ast.BasicLit); ok {
				if code, err := strconv.Unquote(b.Value); err == nil {
					out = append(out, emission{Audit, code, map[string]bool{}, fset.Position(kv.Pos()).String()})
				}
			}
			return true
		})
	}
	return out
}

// mapKeys reads the keys of a payload: map literal, or local variable built in the function
// (`p := map[string]any{…}` plus `p["k"] = …`). nil when it cannot be read.
func mapKeys(fn *ast.FuncDecl, e ast.Expr) map[string]bool {
	if lit, ok := e.(*ast.CompositeLit); ok {
		return litKeys(lit)
	}
	id, ok := e.(*ast.Ident)
	if !ok {
		return nil
	}
	var keys map[string]bool
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		as, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for i, lhs := range as.Lhs {
			if l, ok := lhs.(*ast.Ident); ok && l.Name == id.Name && i < len(as.Rhs) {
				if lit, ok := as.Rhs[i].(*ast.CompositeLit); ok {
					if keys == nil {
						keys = map[string]bool{}
					}
					for k := range litKeys(lit) {
						keys[k] = true
					}
				}
			}
			if ix, ok := lhs.(*ast.IndexExpr); ok && ident(ix.X) == id.Name {
				if b, ok := ix.Index.(*ast.BasicLit); ok {
					if k, err := strconv.Unquote(b.Value); err == nil && keys != nil {
						keys[k] = true
					}
				}
			}
		}
		return true
	})
	return keys
}

func litKeys(lit *ast.CompositeLit) map[string]bool {
	keys := map[string]bool{}
	for _, el := range lit.Elts {
		if kv, ok := el.(*ast.KeyValueExpr); ok {
			if b, ok := kv.Key.(*ast.BasicLit); ok {
				if k, err := strconv.Unquote(b.Value); err == nil {
					keys[k] = true
				}
			}
		}
	}
	return keys
}

// codeLiterals: the code literals that are elements of a table (composite literal) in the
// function — `{"pause_torrent", "TORRENT_PAUSED", "Torrent paused"}` yields TORRENT_PAUSED. Only
// table elements: a loose "PAUSED" in the function is not an event code.
func codeLiterals(fn *ast.FuncDecl) []string {
	var out []string
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		for _, el := range lit.Elts {
			if b, ok := el.(*ast.BasicLit); ok && b.Kind == token.STRING {
				if s, err := strconv.Unquote(b.Value); err == nil && codeLit.MatchString(s) {
					out = append(out, s)
				}
			}
		}
		return true
	})
	return out
}

func isActivityEntry(e ast.Expr) bool {
	switch x := e.(type) {
	case *ast.SelectorExpr:
		return x.Sel.Name == "Entry" && ident(x.X) == "activity"
	case *ast.Ident:
		return false // within the activity package itself there is no emission
	}
	return false
}

// activityKinds reads the Kind* constants from the activity package: name → value.
func activityKinds(t *testing.T, fset *token.FileSet) map[string]string {
	t.Helper()
	f, err := parser.ParseFile(fset, filepath.Join(moduleRoot, "internal/activity/activity.go"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, d := range f.Decls {
		g, ok := d.(*ast.GenDecl)
		if !ok || g.Tok != token.CONST {
			continue
		}
		for _, s := range g.Specs {
			vs := s.(*ast.ValueSpec)
			for i, n := range vs.Names {
				if strings.HasPrefix(n.Name, "Kind") && i < len(vs.Values) {
					if b, ok := vs.Values[i].(*ast.BasicLit); ok {
						v, _ := strconv.Unquote(b.Value)
						out[n.Name] = v
					}
				}
			}
		}
	}
	return out
}

// alertmanagerCodes: the names of the Prometheus rules, which the Alertmanager webhook records
// as the alert code.
func alertmanagerCodes(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile(rulesPath)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, m := range regexp.MustCompile(`(?m)^\s*-\s*alert:\s*(\S+)\s*$`).FindAllStringSubmatch(string(b), -1) {
		out = append(out, m[1])
	}
	sort.Strings(out)
	return out
}

func ident(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		return x.Sel.Name
	}
	return ""
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
