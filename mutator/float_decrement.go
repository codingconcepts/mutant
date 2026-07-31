package mutator

import (
	"fmt"
	"go/ast"
	"go/token"
	"strconv"

	"github.com/codingconcepts/mutant"
)

type floatDecrement struct{}

func NewFloatDecrement() *floatDecrement { return &floatDecrement{} }

func (m *floatDecrement) Name() string { return "float_decrement" }

func (m *floatDecrement) Mutate(fset *token.FileSet, file *ast.File, filePath string, original []byte) []mutant.Mutation {
	var out []mutant.Mutation

	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.FLOAT {
			return true
		}

		originalValue := lit.Value

		f, err := strconv.ParseFloat(originalValue, 64)
		if err != nil {
			return true
		}

		mutatedValue := fmt.Sprintf("%v", f-1.0)
		pos := fset.Position(lit.ValuePos)
		out = append(out, mutant.Mutation{
			File:        filePath,
			Line:        pos.Line,
			Mutator:     "float_decrement",
			Description: "decremented " + originalValue + " to " + mutatedValue,
			Apply:       func() { lit.Value = mutatedValue },
			Revert:      func() { lit.Value = originalValue },
		})

		return true
	})

	return out
}
