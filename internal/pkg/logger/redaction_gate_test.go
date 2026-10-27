package logger_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// sensitiveKeys are the key fragments whose value must never be logged as an
// opaque structure. The redaction core masks by key for plain values, but
// zap.Any and zap.Reflect hand the encoder an interface it cannot look inside,
// so a whole payload or a credential struct would pass through untouched. The
// match is a substring on the normalised key, so logto_body and user_token are
// caught as well.
var sensitiveKeys = []string{
	"token",
	"secret",
	"password",
	"authorization",
	"credential",
	"payload",
	"body",
	"claims",
	"cookie",
	"header",
	"jwks",
	"privatekey",
	"apikey",
}

func keyIsSensitiveKey(key string) bool {
	normalised := normaliseKey(key)
	for _, fragment := range sensitiveKeys {
		if strings.Contains(normalised, fragment) {
			return true
		}
	}
	return false
}

// TestNoOpaqueSensitiveLogFields is the CI gate from the logging plan: it fails
// the build when a log call puts a payload, a body or a credential under a key
// the redaction core cannot see into.
func TestNoOpaqueSensitiveLogFields(t *testing.T) {
	root := repoRoot(t)
	fset := token.NewFileSet()

	var hits []string
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		src, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		hits = append(hits, opaqueSensitiveHits(fset, path, src)...)
		return nil
	})
	if err != nil {
		t.Fatalf("walk internal/: %v", err)
	}

	if len(hits) > 0 {
		t.Errorf("log calls must not put an opaque value under a sensitive key; the redaction core cannot mask it:\n  %s",
			strings.Join(hits, "\n  "))
	}
}

// The gate must actually fire: a scanner that reports nothing no matter what it
// reads would keep the suite green and the leak in place.
func TestGateDetectsOpaqueSensitiveField(t *testing.T) {
	src := []byte(`package sample

import "go.uber.org/zap"

func f(payload map[string]any, token string, id string) []zap.Field {
	return []zap.Field{
		zap.Any("payload", payload),
		zap.Reflect("claims", token),
		zap.Any("logto_body", payload),
		zap.String("user_id", id),
	}
}
`)

	hits := opaqueSensitiveHits(token.NewFileSet(), "sample.go", src)

	if len(hits) != 3 {
		t.Fatalf("want 3 hits for payload/claims/logto_body, got %d: %v", len(hits), hits)
	}
	for _, want := range []string{"payload", "claims", "logto_body"} {
		if !strings.Contains(strings.Join(hits, "\n"), want) {
			t.Errorf("want a hit naming %q, got %v", want, hits)
		}
	}
}

// A file that does not parse is skipped: the build step already reports it.
func TestGateSkipsUnparsableFile(t *testing.T) {
	if hits := opaqueSensitiveHits(token.NewFileSet(), "broken.go", []byte("package (")); len(hits) != 0 {
		t.Errorf("want no hits for an unparsable file, got %v", hits)
	}
}

// opaqueSensitiveHits returns one "file:line:col: reason" entry per offending
// call in src.
func opaqueSensitiveHits(fset *token.FileSet, path string, src []byte) []string {
	file, err := parser.ParseFile(fset, path, src, 0)
	if err != nil {
		return nil
	}

	var hits []string
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || !isOpaqueFieldConstructor(call) {
			return true
		}
		key, ok := stringLiteral(call.Args[0])
		if !ok {
			return true
		}
		if keyIsSensitiveKey(key) {
			hits = append(hits, fset.Position(call.Pos()).String()+": zap."+opaqueName(call)+" with sensitive key "+key)
		}
		return true
	})
	return hits
}

// opaqueName returns the zap constructor name, e.g. "Any".
func opaqueName(call *ast.CallExpr) string {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	return sel.Sel.Name
}

func isOpaqueFieldConstructor(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok || pkg.Name != "zap" {
		return false
	}
	if len(call.Args) < 1 {
		return false
	}
	switch sel.Sel.Name {
	case "Any", "Reflect":
		return true
	default:
		return false
	}
}

func stringLiteral(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	// The key is a constant in every call site this gate targets; a computed
	// key is out of scope for a syntax check.
	unquoted, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return unquoted, true
}

func normaliseKey(key string) string {
	key = strings.ToLower(key)
	key = strings.ReplaceAll(key, "_", "")
	key = strings.ReplaceAll(key, "-", "")
	return key
}

// repoRoot walks up from this file until it finds go.mod, so the gate works
// from any working directory.
func repoRoot(t *testing.T) string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate this test file")
	}

	dir := filepath.Dir(thisFile)
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatalf("go.mod not found above %s", filepath.Dir(thisFile))
	return ""
}
