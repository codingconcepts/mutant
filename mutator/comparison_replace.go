package mutator

import (
	"fmt"
	"go/ast"
	"go/token"

	"github.com/codingconcepts/mutant"
)

func appendSideReplace(out []mutant.Mutation, side string, current ast.Expr, set func(ast.Expr), replacement *ast.Ident, op token.Token, filePath string, pos token.Position) []mutant.Mutation {
	if fmt.Sprint(current) == replacement.Name {
		return out
	}

	original := current
	repl := &ast.Ident{Name: replacement.Name, NamePos: original.Pos()}

	return append(out, mutant.Mutation{
		File:        filePath,
		Line:        pos.Line,
		Mutator:     "comparison_replace",
		Description: "replaced " + side + " side of " + op.String() + " with " + replacement.Name,
		Apply:       func() { set(repl) },
		Revert:      func() { set(original) },
	})
}

type comparisonReplace struct {
	replacements map[token.Token]*ast.Ident
}

func NewComparisonReplace() *comparisonReplace {
	return &comparisonReplace{
		replacements: map[token.Token]*ast.Ident{
			token.LAND: ast.NewIdent("true"),
			token.LOR:  ast.NewIdent("false"),
		},
	}
}

func (m *comparisonReplace) Name() string { return "comparison_replace" }

func (m *comparisonReplace) Mutate(fset *token.FileSet, file *ast.File, filePath string, original []byte) []mutant.Mutation {
	var out []mutant.Mutation

	ast.Inspect(file, func(n ast.Node) bool {
		expr, ok := n.(*ast.BinaryExpr)
		if !ok {
			return true
		}

		replacement, ok := m.replacements[expr.Op]
		if !ok {
			return true
		}

		pos := fset.Position(expr.OpPos)

		out = appendSideReplace(out, "left", expr.X, func(e ast.Expr) { expr.X = e }, replacement, expr.Op, filePath, pos)
		out = appendSideReplace(out, "right", expr.Y, func(e ast.Expr) { expr.Y = e }, replacement, expr.Op, filePath, pos)

		return true
	})

	return out
}
