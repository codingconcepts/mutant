package mutator

import (
	"fmt"
	"go/ast"
	"go/token"
	"strconv"

	"github.com/codingconcepts/mutant"
)

// literalMutator applies a numeric transformation to literal values in the
// AST. Used to implement integer/float increment and decrement mutations.
type literalMutator struct {
	transform   func(string) (string, error) // transforms the literal string value
	mutatorName string
	verb        string      // past-tense verb for the description (e.g. "decremented")
	kind        token.Token // token.INT or token.FLOAT
}

func (m *literalMutator) Name() string { return m.mutatorName }

func (m *literalMutator) Mutate(fset *token.FileSet, file *ast.File, filePath string, original []byte) []mutant.Mutation {
	var out []mutant.Mutation

	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != m.kind {
			return true
		}

		originalValue := lit.Value

		mutatedValue, err := m.transform(originalValue)
		if err != nil {
			return true
		}

		out = append(out, newMutation(filePath, m.mutatorName, fset.Position(lit.ValuePos).Line,
			fmt.Sprintf("%s %s to %s", m.verb, originalValue, mutatedValue),
			func() { lit.Value = mutatedValue },
			func() { lit.Value = originalValue },
		))

		return true
	})

	return out
}

func newNumericLiteralMutator[T int | float64](mutatorName, verb string, kind token.Token, delta T, parse func(string) (T, error), format func(T) string) *literalMutator {
	return &literalMutator{
		mutatorName: mutatorName,
		kind:        kind,
		verb:        verb,
		transform: func(s string) (string, error) {
			v, err := parse(s)
			if err != nil {
				return "", err
			}

			return format(v + delta), nil
		},
	}
}

func formatFloat(f float64) string { return fmt.Sprintf("%v", f) }

func parseFloat(s string) (float64, error) { return strconv.ParseFloat(s, 64) }

// NewFloatDecrement creates a mutator that subtracts 1.0 from float literals.
func NewFloatDecrement() *literalMutator {
	return newNumericLiteralMutator("float_decrement", "decremented", token.FLOAT, -1.0, parseFloat, formatFloat)
}

// NewFloatIncrement creates a mutator that adds 1.0 to float literals.
func NewFloatIncrement() *literalMutator {
	return newNumericLiteralMutator("float_increment", "incremented", token.FLOAT, 1.0, parseFloat, formatFloat)
}

// NewIntegerDecrement creates a mutator that subtracts 1 from integer literals.
func NewIntegerDecrement() *literalMutator {
	return newNumericLiteralMutator("integer_decrement", "decremented", token.INT, -1, strconv.Atoi, strconv.Itoa)
}

// NewIntegerIncrement creates a mutator that adds 1 to integer literals.
func NewIntegerIncrement() *literalMutator {
	return newNumericLiteralMutator("integer_increment", "incremented", token.INT, 1, strconv.Atoi, strconv.Itoa)
}
