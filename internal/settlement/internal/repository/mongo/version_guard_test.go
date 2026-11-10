package repository

// The version guard for the settlements collection.
//
// UpdateSettlement replaces a WHOLE document under a {version: N} guard (mongox.UpdateDoc), so a
// field-level write that leaves version alone is invisible to that guard: a concurrent guarded edit
// still matches, writes back its own older snapshot and silently undoes the field-level change. A
// kicked member reappears with the role_ids of the snapshot, a tag returns, an approval loses the
// tier it granted.
//
// These tests read the package source, so they fail on a write statement that omits the increment
// no matter which function it sits in, and a new writer cannot reintroduce the lost update without
// answering here.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// guardedMethods are the collection calls whose update document this guard reads, mapped to the
// position of that document in the argument list.
var guardedMethods = map[string]int{
	"UpdateOne":        2,
	"UpdateMany":       2,
	"FindOneAndUpdate": 2,
}

// unguardedMethods change a settlements document in a shape this guard cannot read. They must go
// through mongox.UpdateDoc, which carries the version guard itself. InsertOne is not listed: a
// create has no earlier version to bump.
var unguardedMethods = []string{"ReplaceOne", "FindOneAndReplace", "BulkWrite"}

// settlementWrite is one write statement against the settlements collection.
type settlementWrite struct {
	where  string // file:line, for the failure message
	fn     string // enclosing function
	op     string // collection method
	fields map[string][]string
}

func TestEverySettlementsWriteBumpsTheVersion(t *testing.T) {
	writes := scanSettlementWrites(t)

	if len(writes) == 0 {
		t.Fatal("the scan found no settlements write at all, so it proves nothing")
	}

	// The four writers the guard was missing on, named so a move or a rename shows up as a lost
	// site instead of a silently smaller scan.
	for _, want := range []string{"RemoveMember", "AddTag", "RemoveTag", "Update"} {
		if !slices.ContainsFunc(writes, func(w settlementWrite) bool { return w.fn == want }) {
			t.Errorf("no settlements write was found in %s; the scan missed a known site", want)
		}
	}

	for _, w := range writes {
		inc := w.fields["$inc"]
		if !slices.Contains(inc, "version") {
			t.Errorf("%s: %s updates %v but increments %v; a concurrent UpdateSettlement would write back a stale snapshot over it and silently undo the change",
				w.where, w.op, operatedFields(w.fields), inc)
		}
	}
}

// operatedFields lists every field path a write touches, for the failure message.
func operatedFields(fields map[string][]string) []string {
	var out []string
	for _, paths := range fields {
		out = append(out, paths...)
	}
	sort.Strings(out)
	return slices.Compact(out)
}

// scanSettlementWrites parses every non-test file of this package and collects the writes it makes
// against the settlements collection.
func scanSettlementWrites(t *testing.T) []settlementWrite {
	t.Helper()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read the package directory: %v", err)
	}

	var writes []settlementWrite
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			locals := localCompositeLiterals(fn.Body)
			writes = append(writes, writesInFunc(fset, fn, locals)...)
		}
	}

	return writes
}

// localCompositeLiterals maps a local name to the literal it was assigned, so a write that builds
// its update document in a variable is still read.
func localCompositeLiterals(body *ast.BlockStmt) map[string]ast.Expr {
	locals := make(map[string]ast.Expr)

	ast.Inspect(body, func(n ast.Node) bool {
		switch stmt := n.(type) {
		case *ast.AssignStmt:
			if len(stmt.Lhs) == 1 && len(stmt.Rhs) == 1 {
				if id, ok := stmt.Lhs[0].(*ast.Ident); ok {
					if lit, ok := stmt.Rhs[0].(*ast.CompositeLit); ok {
						locals[id.Name] = lit
					}
				}
			}
		case *ast.DeclStmt:
			decl, ok := stmt.Decl.(*ast.GenDecl)
			if !ok {
				return true
			}
			for _, spec := range decl.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok || len(vs.Names) != 1 || len(vs.Values) != 1 {
					continue
				}
				if lit, ok := vs.Values[0].(*ast.CompositeLit); ok {
					locals[vs.Names[0].Name] = lit
				}
			}
		}
		return true
	})

	return locals
}

func writesInFunc(fset *token.FileSet, fn *ast.FuncDecl, locals map[string]ast.Expr) []settlementWrite {
	var writes []settlementWrite

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || !isSettlementsCollection(sel.X) {
			return true
		}

		method := sel.Sel.Name
		if slices.Contains(unguardedMethods, method) {
			writes = append(writes, settlementWrite{
				where: pos(fset, call),
				fn:    fn.Name.Name,
				op:    method + " (this guard cannot read it; route the write through mongox.UpdateDoc)",
			})
			return true
		}

		argPos, guarded := guardedMethods[method]
		if !guarded {
			return true
		}
		if len(call.Args) <= argPos {
			return true
		}

		fields := updateFields(call.Args[argPos], locals)
		if fields == nil {
			writes = append(writes, settlementWrite{
				where:  pos(fset, call),
				fn:     fn.Name.Name,
				op:     method,
				fields: map[string][]string{},
			})
			return true
		}

		writes = append(writes, settlementWrite{
			where:  pos(fset, call),
			fn:     fn.Name.Name,
			op:     method,
			fields: fields,
		})
		return true
	})

	return writes
}

// isSettlementsCollection reports whether the receiver of a call is the settlements collection,
// r.setColl.
func isSettlementsCollection(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "setColl"
}

func pos(fset *token.FileSet, n ast.Node) string {
	p := fset.Position(n.Pos())
	return filepath.Base(p.Filename) + ":" + strconv.Itoa(p.Line)
}

// updateFields reads an update document and maps each operator to the field paths it touches. It
// answers nil when the document is not a literal this guard can read.
func updateFields(expr ast.Expr, locals map[string]ast.Expr) map[string][]string {
	if id, ok := expr.(*ast.Ident); ok {
		if resolved, found := locals[id.Name]; found {
			expr = resolved
		}
	}

	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil
	}

	fields := make(map[string][]string)

	// A pipeline update ([]bson.D) carries its operators one stage per element; a document update
	// carries them at the top level. Both flatten into the same map.
	if _, isPipeline := lit.Type.(*ast.ArrayType); isPipeline {
		for _, stage := range lit.Elts {
			for op, paths := range documentOperators(stage) {
				fields[op] = append(fields[op], paths...)
			}
		}
		return fields
	}

	return documentOperators(lit)
}

// documentOperators reads one update document: a bson.D of operators, or a bson.M of them.
func documentOperators(expr ast.Expr) map[string][]string {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil
	}

	ops := make(map[string][]string)
	for _, elt := range lit.Elts {
		switch e := elt.(type) {
		case *ast.KeyValueExpr: // bson.M{"$set": ...}
			op := stringKey(e.Key)
			if op == "" {
				continue
			}
			ops[op] = append(ops[op], literalFieldNames(e.Value)...)
		case *ast.CompositeLit: // bson.D{{Key: "$set", Value: ...}}
			op, value := keyedElement(e)
			if op == "" {
				continue
			}
			ops[op] = append(ops[op], literalFieldNames(value)...)
		}
	}
	return ops
}

// keyedElement reads one bson.E literal into its operator name and its value expression.
func keyedElement(lit *ast.CompositeLit) (string, ast.Expr) {
	var op string
	var value ast.Expr
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		switch key.Name {
		case "Key":
			op = stringKey(kv.Value)
		case "Value":
			value = kv.Value
		}
	}
	return op, value
}

// literalFieldNames lists the keys of a bson.D or bson.M value.
func literalFieldNames(expr ast.Expr) []string {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil
	}

	var names []string
	for _, elt := range lit.Elts {
		switch e := elt.(type) {
		case *ast.KeyValueExpr: // bson.M{"version": 1}
			if name := stringKey(e.Key); name != "" {
				names = append(names, name)
			}
		case *ast.CompositeLit: // bson.D{{Key: "version", Value: 1}}
			if name, _ := keyedElement(e); name != "" {
				names = append(names, name)
			}
		}
	}
	return names
}

func stringKey(expr ast.Expr) string {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return ""
	}
	value, err := strconv.Unquote(lit.Value)
	if err != nil {
		return ""
	}
	return value
}
