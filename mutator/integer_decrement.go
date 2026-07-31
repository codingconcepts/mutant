package mutator

import (
	"go/ast"
	"go/token"
	"strconv"

	"github.com/codingconcepts/mutant"
)

type integerDecrement struct{}

func NewIntegerDecrement() *integerDecrement { return &integerDecrement{} }

func (m *integerDecrement) Name() string { return "integer_decrement" }

func (m *integerDecrement) Mutate(fset *token.FileSet, file *ast.File, filePath string, original []byte) []mutant.Mutation {
	var out []mutant.Mutation

	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.INT {
			return true
		}

		originalValue := lit.Value

		v, err := strconv.Atoi(originalValue)
		if err != nil {
			return true
		}

		mutatedValue := strconv.Itoa(v - 1)
		pos := fset.Position(lit.ValuePos)
		out = append(out, mutant.Mutation{
			File:        filePath,
			Line:        pos.Line,
			Mutator:     "integer_decrement",
			Description: "decremented " + originalValue + " to " + mutatedValue,
			Apply:       func() { lit.Value = mutatedValue },
			Revert:      func() { lit.Value = originalValue },
		})

		return true
	})

	return out
}
