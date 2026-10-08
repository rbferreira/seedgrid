package archpolicy

import (
	"strings"
	"testing"
	"testing/fstest"
)

// TestAgentWithoutDatabase verifies that the Agent cannot import database drivers,
// but the Control Plane can and that comments/strings do not count.
func TestAgentWithoutDatabase(t *testing.T) {
	fsys := fstest.MapFS{
		// Positive case: Agent importing database/sql.
		"agent/internal/db/db.go": &fstest.MapFile{
			Data: []byte(`package db

import "database/sql"

func Connect() *sql.DB {
	return nil
}
`),
		},
		// Positive case: Agent importing pgx driver.
		"agent/internal/pgx.go": &fstest.MapFile{
			Data: []byte(`package agent

import "github.com/jackc/pgx/v5"

func UsePgx() pgx.Conn {
	return pgx.Conn{}
}
`),
		},
		// Positive case: Agent importing lib/pq driver.
		"agent/internal/pq.go": &fstest.MapFile{
			Data: []byte(`package agent

import _ "github.com/lib/pq"
`),
		},
		// Negative case: Control Plane importing database/sql is allowed.
		"control-plane/internal/db/db.go": &fstest.MapFile{
			Data: []byte(`package db

import "database/sql"

func Connect() *sql.DB {
	return nil
}
`),
		},
		// Negative case: Agent with database/sql in a comment does not count.
		"agent/internal/comment.go": &fstest.MapFile{
			Data: []byte(`package agent

// database/sql não é importado aqui.
func Foo() {}
`),
		},
		// Negative case: Agent with database/sql in a string does not count.
		"agent/internal/string.go": &fstest.MapFile{
			Data: []byte(`package agent

func Bar() string {
	return "database/sql"
}
`),
		},
	}

	vs, err := Check(fsys)
	if err != nil {
		t.Fatalf("Check() error: %v", err)
	}

	// Should catch 3 violations in the Agent files.
	if len(vs) != 3 {
		t.Errorf("Check() returned %d violations, want 3", len(vs))
		for _, v := range vs {
			t.Logf("  %s", v)
		}
	}

	// Verifies that all of them are from the Agent.
	for _, v := range vs {
		if !strings.HasPrefix(v.Path, "agent/") {
			t.Errorf("violation outside Agent: %s", v)
		}
		if v.Rule != "agent-no-db" {
			t.Errorf("wrong rule: %s", v)
		}
	}
}

// TestAgentWithoutControlPlane verifies that the Agent cannot import the Control Plane.
func TestAgentWithoutControlPlane(t *testing.T) {
	fsys := fstest.MapFS{
		// Positive case: Agent importing Control Plane.
		"agent/internal/client.go": &fstest.MapFile{
			Data: []byte(`package client

import "github.com/rbferreira/seedgrid/control-plane/api"

func Call() {}
`),
		},
		// Negative case: Agent importing net/http is allowed.
		"agent/internal/http.go": &fstest.MapFile{
			Data: []byte(`package agent

import "net/http"

func Serve() {}
`),
		},
	}

	vs, err := Check(fsys)
	if err != nil {
		t.Fatalf("Check() error: %v", err)
	}

	if len(vs) != 1 {
		t.Errorf("Check() returned %d violations, want 1", len(vs))
		for _, v := range vs {
			t.Logf("  %s", v)
		}
	}

	if len(vs) > 0 {
		if vs[0].Path != "agent/internal/client.go" {
			t.Errorf("wrong path: %s", vs[0].Path)
		}
		if vs[0].Rule != "agent-no-control-plane" {
			t.Errorf("wrong rule: %s", vs[0].Rule)
		}
		if vs[0].Line != 3 {
			t.Errorf("wrong line: %d, want 3", vs[0].Line)
		}
	}
}

// TestControlPlaneWithoutAgent verifies that the Control Plane cannot import the Agent.
func TestControlPlaneWithoutAgent(t *testing.T) {
	fsys := fstest.MapFS{
		// Positive case: Control Plane importing Agent.
		"control-plane/internal/engine.go": &fstest.MapFile{
			Data: []byte(`package engine

import "github.com/rbferreira/seedgrid/agent"

func Start() {}
`),
		},
		// Negative case: Control Plane importing shared is allowed.
		"control-plane/internal/shared.go": &fstest.MapFile{
			Data: []byte(`package internal

import "github.com/rbferreira/seedgrid/shared"

func Use() {}
`),
		},
	}

	vs, err := Check(fsys)
	if err != nil {
		t.Fatalf("Check() error: %v", err)
	}

	if len(vs) != 1 {
		t.Errorf("Check() returned %d violations, want 1", len(vs))
		for _, v := range vs {
			t.Logf("  %s", v)
		}
	}

	if len(vs) > 0 {
		if vs[0].Path != "control-plane/internal/engine.go" {
			t.Errorf("wrong path: %s", vs[0].Path)
		}
		if vs[0].Rule != "control-plane-no-agent" {
			t.Errorf("wrong rule: %s", vs[0].Rule)
		}
		if vs[0].Line != 3 {
			t.Errorf("wrong line: %d, want 3", vs[0].Line)
		}
	}
}

// TestAgentWithoutWorker verifies that the Agent cannot declare identifiers with "worker",
// but that comments and strings do not count.
func TestAgentWithoutWorker(t *testing.T) {
	fsys := fstest.MapFS{
		// Positive case: function with Worker in the name.
		"agent/internal/worker.go": &fstest.MapFile{
			Data: []byte(`package agent

func StartWorker() {}
`),
		},
		// Positive case: type with Worker in the name.
		"agent/internal/types.go": &fstest.MapFile{
			Data: []byte(`package agent

type WorkerPool struct{}
`),
		},
		// Positive case: variable with Worker in the name.
		"agent/internal/var.go": &fstest.MapFile{
			Data: []byte(`package agent

var workerCount int
`),
		},
		// Positive case: struct field with Worker in the name.
		"agent/internal/struct.go": &fstest.MapFile{
			Data: []byte(`package agent

type Config struct {
	WorkerTimeout int
}
`),
		},
		// Negative case: Worker in a comment does not count.
		"agent/internal/comment.go": &fstest.MapFile{
			Data: []byte(`package agent

// Worker é um componente do Control Plane.
func Info() {}
`),
		},
		// Negative case: Worker in a string does not count.
		"agent/internal/string.go": &fstest.MapFile{
			Data: []byte(`package agent

func Log() string {
	return "worker started"
}
`),
		},
		// Negative case: Control Plane can have Worker.
		"control-plane/internal/worker.go": &fstest.MapFile{
			Data: []byte(`package internal

func StartWorker() {}
`),
		},
	}

	vs, err := Check(fsys)
	if err != nil {
		t.Fatalf("Check() error: %v", err)
	}

	// Should catch 4 violations (function, type, variable, field).
	if len(vs) != 4 {
		t.Errorf("Check() returned %d violations, want 4", len(vs))
		for _, v := range vs {
			t.Logf("  %s", v)
		}
	}

	for _, v := range vs {
		if !strings.HasPrefix(v.Path, "agent/") {
			t.Errorf("violation outside Agent: %s", v)
		}
		if v.Rule != "agent-no-worker" {
			t.Errorf("wrong rule: %s", v)
		}
	}
}

// TestPortalNetworkOnlyInAPI verifies that the Portal only does network via api.ts,
// but that comments, tests, and api.ts itself are exceptions.
func TestPortalNetworkOnlyInAPI(t *testing.T) {
	fsys := fstest.MapFS{
		// Positive case: fetch outside api.ts.
		"portal/src/hooks/useData.ts": &fstest.MapFile{
			Data: []byte(`export function useData() {
	return fetch("/api/data")
}
`),
		},
		// Positive case: WebSocket outside api.ts.
		"portal/src/hooks/useWS.ts": &fstest.MapFile{
			Data: []byte(`export function useWS() {
	return new WebSocket("ws://localhost")
}
`),
		},
		// Positive case: XMLHttpRequest outside api.ts.
		"portal/src/hooks/useXHR.ts": &fstest.MapFile{
			Data: []byte(`export function useXHR() {
	return new XMLHttpRequest()
}
`),
		},
		// Positive case: EventSource outside api.ts.
		"portal/src/hooks/useSSE.ts": &fstest.MapFile{
			Data: []byte(`export function useSSE() {
	return new EventSource("/events")
}
`),
		},
		// Positive case: sendBeacon outside api.ts.
		"portal/src/hooks/useBeacon.ts": &fstest.MapFile{
			Data: []byte(`export function useBeacon() {
	navigator.sendBeacon("/log")
}
`),
		},
		// Negative case: fetch in a comment does not count.
		"portal/src/hooks/comment.ts": &fstest.MapFile{
			Data: []byte(`// fetch("/api") não é usado aqui
export function foo() {}
`),
		},
		// Negative case: fetch in a test does not count.
		"portal/src/hooks/useData.test.ts": &fstest.MapFile{
			Data: []byte(`test("useData", () => {
	fetch("/api/data")
})
`),
		},
		// Negative case: fetch in api.ts is allowed.
		"portal/src/api.ts": &fstest.MapFile{
			Data: []byte(`export async function getData() {
	return fetch("/api/data")
}
`),
		},
		// Negative case: fetch in a file outside portal/src/ does not count.
		"portal/scripts/build.ts": &fstest.MapFile{
			Data: []byte(`fetch("/build")
`),
		},
	}

	vs, err := Check(fsys)
	if err != nil {
		t.Fatalf("Check() error: %v", err)
	}

	// Should catch 5 violations (fetch, WebSocket, XMLHttpRequest, EventSource, sendBeacon).
	if len(vs) != 5 {
		t.Errorf("Check() returned %d violations, want 5", len(vs))
		for _, v := range vs {
			t.Logf("  %s", v)
		}
	}

	for _, v := range vs {
		if v.Rule != "portal-network-only-via-api" {
			t.Errorf("wrong rule: %s", v)
		}
	}
}

// TestSkipDirs verifies that ignored directories are not scanned.
func TestSkipDirs(t *testing.T) {
	fsys := fstest.MapFS{
		// Positive case: violation outside an ignored directory.
		"agent/internal/worker.go": &fstest.MapFile{
			Data: []byte(`package agent

func StartWorker() {}
`),
		},
		// Negative case: violation in vendor does not count.
		"vendor/agent/internal/worker.go": &fstest.MapFile{
			Data: []byte(`package agent

func StartWorker() {}
`),
		},
		// Negative case: violation in node_modules does not count.
		"portal/node_modules/agent/worker.go": &fstest.MapFile{
			Data: []byte(`package agent

func StartWorker() {}
`),
		},
		// Negative case: violation in .git does not count.
		".git/hooks/worker.go": &fstest.MapFile{
			Data: []byte(`package hooks

func StartWorker() {}
`),
		},
	}

	vs, err := Check(fsys)
	if err != nil {
		t.Fatalf("Check() error: %v", err)
	}

	// Should catch only 1 violation (the one outside the ignored directories).
	if len(vs) != 1 {
		t.Errorf("Check() returned %d violations, want 1", len(vs))
		for _, v := range vs {
			t.Logf("  %s", v)
		}
	}

	if len(vs) > 0 && vs[0].Path != "agent/internal/worker.go" {
		t.Errorf("wrong path: %s", vs[0].Path)
	}
}

// TestDeterministicOrder verifies that violations are output sorted by path and line.
func TestDeterministicOrder(t *testing.T) {
	fsys := fstest.MapFS{
		"agent/b.go": &fstest.MapFile{
			Data: []byte(`package agent

import "database/sql"

func B() {}
`),
		},
		"agent/a.go": &fstest.MapFile{
			Data: []byte(`package agent

import "database/sql"

func A() {}
`),
		},
		"portal/src/z.ts": &fstest.MapFile{
			Data: []byte(`fetch("/z")
`),
		},
		"portal/src/a.ts": &fstest.MapFile{
			Data: []byte(`fetch("/a")
`),
		},
	}

	vs, err := Check(fsys)
	if err != nil {
		t.Fatalf("Check() error: %v", err)
	}

	if len(vs) != 4 {
		t.Fatalf("Check() returned %d violations, want 4", len(vs))
	}

	// Check order: agent/a.go < agent/b.go < portal/src/a.ts < portal/src/z.ts.
	expected := []string{"agent/a.go", "agent/b.go", "portal/src/a.ts", "portal/src/z.ts"}
	for i, exp := range expected {
		if vs[i].Path != exp {
			t.Errorf("vs[%d].Path = %s, want %s", i, vs[i].Path, exp)
		}
	}
}

// TestAgentWithoutWorker_OnlyDeclarations covers what "declaring" includes (parameter, named
// return, range variable, ":=", interface method, receiver) and what it does not include
// (assignment with "=", use of a field from another type).
func TestAgentWithoutWorker_OnlyDeclarations(t *testing.T) {
	get := map[string]string{
		"parameter":        "package a\n\nfunc f(workerID int) {}\n",
		"named result":     "package a\n\nfunc f() (worker int) { return }\n",
		"range variable":   "package a\n\nfunc f(xs []int) {\n\tfor _, worker := range xs {\n\t\t_ = worker\n\t}\n}\n",
		"short assignment": "package a\n\nfunc f() {\n\tworker := 1\n\t_ = worker\n}\n",
		"interface method": "package a\n\ntype T interface {\n\tRunWorker()\n}\n",
		"receiver":         "package a\n\ntype T struct{}\n\nfunc (worker T) F() {}\n",
		"package name":     "package worker\n",
		"constant":         "package a\n\nconst maxWorkers = 2\n",
	}
	for name, src := range get {
		vs, err := Check(fstest.MapFS{"agent/internal/x/x.go": &fstest.MapFile{Data: []byte(src)}})
		if err != nil {
			t.Fatalf("%s: Check() error: %v", name, err)
		}
		if len(vs) == 0 {
			t.Errorf("%s: not caught", name)
		}
		for _, v := range vs {
			if v.Rule != "agent-no-worker" {
				t.Errorf("%s: wrong rule: %s", name, v)
			}
		}
	}

	notCaught := map[string]string{
		"assignment with = to outer name": "package a\n\nimport \"os\"\n\nfunc f() {\n\tvar x struct{ n int }\n\tx.n = len(os.Args)\n}\n",
		"call to external function":       "package a\n\nimport \"strings\"\n\nfunc f() bool { return strings.Contains(\"a\", \"worker\") }\n",
		"similar word":                    "package a\n\nfunc network() {}\n\nvar workload int\n",
	}
	for name, src := range notCaught {
		vs, err := Check(fstest.MapFS{"agent/internal/x/x.go": &fstest.MapFile{Data: []byte(src)}})
		if err != nil {
			t.Fatalf("%s: Check() error: %v", name, err)
		}
		if len(vs) != 0 {
			t.Errorf("%s: should not be a violation: %v", name, vs)
		}
	}
}

// TestImportPrefixHasBoundary: the forbidden prefix matches the package and subpackages, not a
// similarly named neighbor.
func TestImportPrefixHasBoundary(t *testing.T) {
	src := func(imp string) *fstest.MapFile {
		return &fstest.MapFile{Data: []byte("package x\n\nimport _ \"" + imp + "\"\n")}
	}
	vs, err := Check(fstest.MapFS{
		// These are not what the rule forbids: similar name, different boundary.
		"agent/internal/a/a.go":         src("github.com/lib/pqueue"),
		"agent/internal/b/b.go":         src("database/sqlite3extra"),
		"control-plane/internal/c/c.go": src("github.com/rbferreira/seedgrid/agents"),
		// These are.
		"agent/internal/d/d.go":         src("github.com/lib/pq/oid"),
		"control-plane/internal/e/e.go": src("github.com/rbferreira/seedgrid/agent"),
	})
	if err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	var got []string
	for _, v := range vs {
		got = append(got, v.Path)
	}
	want := []string{"agent/internal/d/d.go", "control-plane/internal/e/e.go"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("violations in %v, want %v", got, want)
	}
}

// TestPortalBlockComment: "/* fetch( */" on a line is a comment, not a call.
func TestPortalBlockComment(t *testing.T) {
	vs, err := Check(fstest.MapFS{
		"portal/src/lib/x.ts": &fstest.MapFile{Data: []byte("/* previously used fetch(url) here */\nexport const a = 1;\n")},
	})
	if err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if len(vs) != 0 {
		t.Errorf("block comment counted as network call: %v", vs)
	}
}
