package mutator

import (
	"fmt"
	"go/ast"
	"go/token"

	"github.com/codingconcepts/mutant"
)

func appendSideReplace(out []mutant.Mutation, side string, current ast.Expr, set func(ast.Expr), replacement *ast.Ident, op token.Token, filePath string, line int) []mutant.Mutation {
	if fmt.Sprint(current) == replacement.Name {
		return out
	}

	original := current
	repl := &ast.Ident{Name: replacement.Name, NamePos: original.Pos()}

	return append(out, newMutation(filePath, "comparison_replace", line,
		fmt.Sprintf("replaced %s side of %s with %s", side, op, replacement.Name),
		func() { set(repl) },
		func() { set(original) },
	))
}

type comparisonReplace struct {
	replacements map[token.Token]*ast.Ident
}

// NewComparisonReplace creates a mutator that replaces operands of logical
// operators with constants: for &&, each side is replaced with true; for ||,
// each side is replaced with false. Produces two mutations per expression
// (one for each side).
func NewComparisonReplace() *comparisonReplace {
	return &comparisonReplace{
		replacements: map[token.Token]*ast.Ident{
			token.LAND: ast.NewIdent("true"),
			token.LOR:  ast.NewIdent("false"),
		},
	}
}

func (m *comparisonReplace) Name() string { return "comparison_replace" }

// Mutate walks the AST for && and || expressions, producing mutations that
// replace the left or right operand with the corresponding constant.
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

		line := fset.Position(expr.OpPos).Line

		out = appendSideReplace(out, "left", expr.X, func(e ast.Expr) { expr.X = e }, replacement, expr.Op, filePath, line)
		out = appendSideReplace(out, "right", expr.Y, func(e ast.Expr) { expr.Y = e }, replacement, expr.Op, filePath, line)

		return true
	})

	return out
}
