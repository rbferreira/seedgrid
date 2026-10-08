package langpolicy

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestIsPortuguese(t *testing.T) {
	pt := []string{
		"// o env do Node",
		"falha ao gravar o arquivo",
		"Não foi possível conectar",
		"deveria ser 3",
		"host = %q, quer host2",
		"apelido do Node (#145)",
		"configuração",
	}
	for _, s := range pt {
		if !IsPortuguese(s) {
			t.Errorf("IsPortuguese(%q) = false, want true", s)
		}
	}
	en := []string{
		"// so that the same Node reuses its identity",
		"see https://example.com for details",
		"Reset runs in the same goroutine as the select",
		"ate the whole queue", // "até" needs the accent
		"ja-JP locale",        // "já" needs the accent
		"want 3, got 4",
	}
	for _, s := range en {
		if IsPortuguese(s) {
			t.Errorf("IsPortuguese(%q) = true, want false", s)
		}
	}
}

func TestParseAllow(t *testing.T) {
	allow, err := ParseAllow([]byte("# comment\n\na/b.go\tfoo\twhy it stays\n"))
	if err != nil || len(allow) != 1 || allow[0].Path != "a/b.go" || allow[0].Text != "foo" {
		t.Fatalf("ParseAllow = %+v, %v", allow, err)
	}
	if _, err := ParseAllow([]byte("a/b.go\tfoo\n")); err == nil {
		t.Error("an exception without a reason was accepted")
	}
}

func TestCheck(t *testing.T) {
	fsys := fstest.MapFS{
		"x/code.go": {Data: []byte(`package x

// Run fica parado até o fim.
func Run() error { return errLog("falha ao gravar o arquivo") }

func errLog(s string) error { return nil }
`)},
		"x/code_test.go": {Data: []byte(`package x

import "testing"

func TestRun(t *testing.T) {
	data := "título de exemplo, não é mensagem" // test data is not checked
	_ = data
	t.Errorf("deveria ser 3")
}

func TestSemNome(t *testing.T) {}
`)},
		"web/a.test.ts":     {Data: []byte("describe(\"faz a coisa quando pode\", () => {});\n")},
		"run.sh":            {Data: []byte("#!/bin/sh\necho \"falha ao subir\"  # comentário em português\n")},
		"docs/pt-BR/x.md":   {Data: []byte("Documentação em português\n")},
		"profiles/u/a.json": {Data: []byte(`{"name": "Séries"}`)},
		"shared/c.json":     {Data: []byte(`{"$comment": "Contrato entre os dois lados", "name": "Séries"}`)},
		"web/index.html":    {Data: []byte("<script>\n  // Aplica o tema salvo\n</script>\n<!-- página inicial -->\n")},
		"d/a.service":       {Data: []byte("[Unit]\nDescription=SeedGrid — reanexar o storage\n")},
		"d/a.rules":         {Data: []byte("# recupera o storage quando o disco volta\n")},
		".gitignore":        {Data: []byte("# Pasta de teste local — não versionar\n/teste/\n")},
	}
	got, err := Check(fsys, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"x/code.go:3":       "comment",
		"x/code.go:4":       "string",
		"x/code_test.go:8":  "test message",
		"x/code_test.go:11": "function name",
		"web/a.test.ts:1":   "test name",
		"run.sh:2":          "comment",
		"shared/c.json:1":   "comment",
		"web/index.html:2":  "comment",
		"web/index.html:4":  "comment",
		"d/a.service:2":     "unit description",
		"d/a.rules:1":       "comment",
		".gitignore:1":      "comment",
	}
	seen := map[string]bool{}
	for _, f := range got {
		key := f.Path + ":" + itoa(f.Line)
		seen[key+"/"+f.Kind] = true
	}
	for key, kind := range want {
		if !seen[key+"/"+kind] {
			t.Errorf("missing finding %s (%s); got %v", key, kind, got)
		}
	}
	if !seen["run.sh:2/string"] {
		t.Errorf("missing the shell string on run.sh:2; got %v", got)
	}
	for _, f := range got {
		if strings.HasPrefix(f.Path, "docs/") || strings.HasPrefix(f.Path, "profiles/") || strings.Contains(f.Text, "título de exemplo") {
			t.Errorf("flagged something that is allowed to be Portuguese: %v", f)
		}
	}

	// An exception covers its line, matched against the whole text.
	got, _ = Check(fsys, []Allow{{Path: "x/code.go", Text: "gravar o arquivo", Reason: "test"}})
	for _, f := range got {
		if f.Path == "x/code.go" && f.Line == 4 {
			t.Errorf("the exception did not cover %v", f)
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}
