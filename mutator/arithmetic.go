package mutator

import (
	"go/ast"
	"go/token"

	"github.com/codingconcepts/mutant"
)

type arithmetic struct {
	tokenSwapMutator
}

func NewArithmetic() *arithmetic {
	return &arithmetic{
		tokenSwapMutator{
			name: "arithmetic",
			mutations: map[token.Token]token.Token{
				token.ADD: token.SUB,
				token.SUB: token.ADD,
				token.MUL: token.QUO,
				token.QUO: token.MUL,
				token.REM: token.MUL,
			},
			extract: extractBinaryExpr,
		},
	}
}

func (m *arithmetic) Mutate(fset *token.FileSet, file *ast.File, filePath string, original []byte) []mutant.Mutation {
	var out []mutant.Mutation

	ast.Inspect(file, func(n ast.Node) bool {
		expr, ok := n.(*ast.BinaryExpr)
		if !ok {
			return true
		}

		if isStringExpr(expr.X) || isStringExpr(expr.Y) {
			return true
		}

		mutated, ok := m.mutations[expr.Op]
		if !ok {
			return true
		}

		originalOp := expr.Op
		pos := fset.Position(expr.OpPos)
		out = append(out, mutant.Mutation{
			File:        filePath,
			Line:        pos.Line,
			Mutator:     m.name,
			Description: "replaced " + originalOp.String() + " with " + mutated.String(),
			Apply:       func() { expr.Op = mutated },
			Revert:      func() { expr.Op = originalOp },
		})

		return true
	})

	return out
}

func isStringExpr(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.BasicLit:
		return e.Kind == token.STRING
	case *ast.CallExpr:
		if ident, ok := e.Fun.(*ast.Ident); ok {
			return ident.Name == "string" && ident.Obj == nil
		}
	}

	return false
}
