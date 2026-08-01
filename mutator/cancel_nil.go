package mutator

import (
	"go/ast"
	"go/token"

	"github.com/codingconcepts/mutant"
)

type cancelNil struct{}

func NewCancelNil() *cancelNil { return &cancelNil{} }

func (m *cancelNil) Name() string { return "cancel_nil" }

func (m *cancelNil) Mutate(fset *token.FileSet, file *ast.File, filePath string, original []byte) []mutant.Mutation {
	cancelFuncs := findCancelCauseFuncs(file)
	if len(cancelFuncs) == 0 {
		return nil
	}

	var out []mutant.Mutation

	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		ident, ok := call.Fun.(*ast.Ident)
		if !ok {
			return true
		}

		if !cancelFuncs[ident.Name] {
			return true
		}

		if len(call.Args) != 1 {
			return true
		}

		if isNilIdent(call.Args[0]) {
			return true
		}

		originalArg := call.Args[0]
		nilIdent := &ast.Ident{Name: "nil", NamePos: originalArg.Pos()}
		pos := fset.Position(call.Lparen)
		out = append(out, mutant.Mutation{
			File:        filePath,
			Line:        pos.Line,
			Mutator:     "cancel_nil",
			Description: "replaced cancel cause argument with nil",
			Apply:       func() { call.Args[0] = nilIdent },
			Revert:      func() { call.Args[0] = originalArg },
		})

		return true
	})

	return out
}

func findCancelCauseFuncs(file *ast.File) map[string]bool {
	funcs := make(map[string]bool)

	ast.Inspect(file, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}

		if len(assign.Rhs) != 1 {
			return true
		}

		call, ok := assign.Rhs[0].(*ast.CallExpr)
		if !ok {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		pkgIdent, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}

		if pkgIdent.Name != "context" || sel.Sel.Name != "WithCancelCause" {
			return true
		}
		// context.WithCancelCause returns (ctx, cancel) - cancel is the second LHS
		if len(assign.Lhs) >= 2 {
			if ident, ok := assign.Lhs[1].(*ast.Ident); ok {
				funcs[ident.Name] = true
			}
		}

		return true
	})

	return funcs
}

func isNilIdent(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == "nil"
}
