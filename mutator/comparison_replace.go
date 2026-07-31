package mutator

import (
	"fmt"
	"go/ast"
	"go/token"

	"github.com/codingconcepts/mutant"
)

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

		if fmt.Sprint(expr.X) != replacement.Name {
			originalX := expr.X
			replX := &ast.Ident{Name: replacement.Name, NamePos: originalX.Pos()}
			out = append(out, mutant.Mutation{
				File:        filePath,
				Line:        pos.Line,
				Mutator:     "comparison_replace",
				Description: "replaced left side of " + expr.Op.String() + " with " + replacement.Name,
				Apply:       func() { expr.X = replX },
				Revert:      func() { expr.X = originalX },
			})
		}

		if fmt.Sprint(expr.Y) != replacement.Name {
			originalY := expr.Y
			replY := &ast.Ident{Name: replacement.Name, NamePos: originalY.Pos()}
			out = append(out, mutant.Mutation{
				File:        filePath,
				Line:        pos.Line,
				Mutator:     "comparison_replace",
				Description: "replaced right side of " + expr.Op.String() + " with " + replacement.Name,
				Apply:       func() { expr.Y = replY },
				Revert:      func() { expr.Y = originalY },
			})
		}

		return true
	})

	return out
}
