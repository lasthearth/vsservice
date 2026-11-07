package main

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// natsPrefix is the import path prefix every NATS package lives under: the
// client (nats.go), its JetStream package, and the two libraries those pull in
// (nkeys, nuid).
const natsPrefix = "github.com/nats-io/"

// goModName is the module file the second guard reads. It is named rather than
// inlined so the failure message can point at it.
const goModName = "go.mod"

// TestNoSourceImportsNATS walks the module's own sources and fails on any import
// of a NATS package.
//
// This is the check that the broker is gone rather than merely unused. A deleted
// call site leaves the dependency in go.mod, an unused import fails the build
// only in the file that carries it, and a package nobody composes into main.go
// still compiles and still links the client. Reading the import graph covers all
// three at once, and it does not depend on the go tool being reachable.
func TestNoSourceImportsNATS(t *testing.T) {
	fset := token.NewFileSet()
	walked := 0

	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			if skipDir(path, d.Name()) {
				return fs.SkipDir
			}

			return nil
		}

		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		walked++

		f, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			// A file this guard cannot parse is not a file it can clear, so it
			// is reported instead of being walked past in silence.
			t.Errorf("%s: cannot parse: %v", path, parseErr)

			return nil
		}

		for _, imp := range f.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			if strings.HasPrefix(importPath, natsPrefix) {
				t.Errorf("%s imports %s: NATS was removed, presence and serverinfo go through MongoDB", path, importPath)
			}
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Without this the test passes on a walk that found nothing — an empty tree,
	// a wrong working directory, a skip rule that ate the module.
	if walked == 0 {
		t.Fatal("the guard walked no .go files, so it proved nothing")
	}
}

// TestGoModRequiresNoNATS pins the dependency itself. A source walk cannot see a
// module that is only required and not yet imported, and it cannot see the
// indirect libraries (nkeys, nuid) the client drags in.
func TestGoModRequiresNoNATS(t *testing.T) {
	raw, err := os.ReadFile(goModName)
	if err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(string(raw), "\n")
	for i, line := range lines {
		if strings.Contains(line, "nats-io") {
			t.Errorf("%s:%d still requires a NATS module: %s", goModName, i+1, strings.TrimSpace(line))
		}
	}
}

// skipDir reports whether a directory is outside this module's sources.
//
// Dot-directories hold checkouts, worktrees and tool state rather than code this
// module builds, and a nested go.mod is a module of its own — tools/modelguard is
// one, and its testdata is deliberately not compilable.
func skipDir(path, name string) bool {
	if path == "." {
		return false
	}

	if strings.HasPrefix(name, ".") || name == "vendor" || name == "node_modules" {
		return true
	}

	if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
		return true
	}

	return false
}
