package mutator

import (
	"go/ast"
	"go/token"
)

// NewArithmetic creates a mutator that swaps arithmetic operators in binary
// expressions:
//
// + <-> -
// * <-> /
// % -> *
//
// Skips string concatenation.
func NewArithmetic() *tokenSwapMutator {
	return &tokenSwapMutator{
		name: "arithmetic",
		mutations: map[token.Token]token.Token{
			token.ADD: token.SUB,
			token.SUB: token.ADD,
			token.MUL: token.QUO,
			token.QUO: token.MUL,
			token.REM: token.MUL,
		},
		extract: extractBinaryExpr,
		skip: func(n ast.Node) bool {
			expr, ok := n.(*ast.BinaryExpr)
			if !ok {
				return true
			}

			return isStringExpr(expr.X) || isStringExpr(expr.Y)
		},
	}
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
