package modelguard

import (
	"go/ast"
	"go/types"
	"regexp"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// defaultModelPackagePattern matches domain model packages such as
// ".../internal/donate/internal/model".
const defaultModelPackagePattern = `(^|/)internal/[^/]+(/internal)?/model$`

func newAnalyzer(s Settings) (*analysis.Analyzer, error) {
	modelPattern := s.ModelPackagePattern
	if modelPattern == "" {
		modelPattern = defaultModelPackagePattern
	}
	modelRe, err := regexp.Compile(modelPattern)
	if err != nil {
		return nil, err
	}
	return &analysis.Analyzer{
		Name: "modelguard",
		Doc: "enforces that domain model structs are mutated via methods (never direct field assignment) outside their " +
			"own package, and that aggregates with a New* constructor are built via it (never a raw struct literal)",
		Requires: []*analysis.Analyzer{inspect.Analyzer},
		Run: func(pass *analysis.Pass) (any, error) {
			run(pass, modelRe)
			return nil, nil
		},
	}, nil
}

// classification records what kind of model type a named type is.
type classification struct{ isModel, hasCtor bool }

func run(pass *analysis.Pass, modelRe *regexp.Regexp) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	cache := map[*types.Named]classification{}
	classify := func(n *types.Named) classification {
		if n == nil {
			return classification{}
		}
		if v, ok := cache[n]; ok {
			return v
		}
		v := classification{}
		if pkg := n.Obj().Pkg(); pkg != nil && modelRe.MatchString(pkg.Path()) {
			if _, ok := n.Underlying().(*types.Struct); ok {
				v.isModel = true
				v.hasCtor = hasConstructor(pkg, n)
			}
		}
		cache[n] = v
		return v
	}

	filter := []ast.Node{
		(*ast.CompositeLit)(nil),
		(*ast.AssignStmt)(nil),
		(*ast.IncDecStmt)(nil),
	}
	insp.Preorder(filter, func(n ast.Node) {
		switch node := n.(type) {
		case *ast.CompositeLit:
			named := namedStruct(pass.TypesInfo.TypeOf(node))
			// Only constructor-guarded aggregates can be required to use New*;
			// types without a constructor have no other way to be built.
			if c := classify(named); !c.isModel || !c.hasCtor || pass.Pkg == named.Obj().Pkg() {
				return
			}
			pass.Reportf(node.Lbrace, "construct %s via its New%s constructor, not a struct literal",
				named.Obj().Name(), named.Obj().Name())
		case *ast.AssignStmt:
			for _, lhs := range node.Lhs {
				reportFieldWrite(pass, lhs, classify)
			}
		case *ast.IncDecStmt:
			reportFieldWrite(pass, node.X, classify)
		}
	})
}

func reportFieldWrite(pass *analysis.Pass, lhs ast.Expr, classify func(*types.Named) classification) {
	sel, ok := lhs.(*ast.SelectorExpr)
	if !ok {
		return
	}
	named := namedStruct(deref(pass.TypesInfo.TypeOf(sel.X)))
	if c := classify(named); !c.isModel || pass.Pkg == named.Obj().Pkg() {
		return
	}
	if selem, ok := pass.TypesInfo.Selections[sel]; !ok || selem.Kind() != types.FieldVal {
		return
	}
	pass.Reportf(sel.Sel.Pos(), "mutate %s via its methods, not direct field assignment", named.Obj().Name())
}

// namedStruct returns t as a named struct type, or nil if t is not one.
func namedStruct(t types.Type) *types.Named {
	if t == nil {
		return nil
	}
	n, ok := types.Unalias(t).(*types.Named)
	if !ok {
		return nil
	}
	if _, ok := n.Underlying().(*types.Struct); !ok {
		return nil
	}
	return n
}

func deref(t types.Type) types.Type {
	if t == nil {
		return nil
	}
	if p, ok := types.Unalias(t).(*types.Pointer); ok {
		return p.Elem()
	}
	return t
}

// hasConstructor reports whether pkg declares a func named New* whose result is
// named or *named — the signal that named is a constructor-guarded aggregate.
func hasConstructor(pkg *types.Package, named *types.Named) bool {
	scope := pkg.Scope()
	for _, name := range scope.Names() {
		if !strings.HasPrefix(name, "New") {
			continue
		}
		fn, ok := scope.Lookup(name).(*types.Func)
		if !ok {
			continue
		}
		sig, ok := fn.Type().(*types.Signature)
		if !ok {
			continue
		}
		results := sig.Results()
		for i := range results.Len() {
			if returnsNamed(results.At(i).Type(), named) {
				return true
			}
		}
	}
	return false
}

func returnsNamed(t types.Type, named *types.Named) bool {
	t = types.Unalias(t)
	if p, ok := t.(*types.Pointer); ok {
		t = types.Unalias(p.Elem())
	}
	n, ok := t.(*types.Named)
	return ok && n.Obj() == named.Obj()
}
