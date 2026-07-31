package mutator

import (
	"fmt"
	"go/ast"
	"go/token"
	"strconv"

	"github.com/codingconcepts/mutant"
)

type literalMutator struct {
	transform   func(string) (string, error)
	mutatorName string
	verb        string
	kind        token.Token
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

		pos := fset.Position(lit.ValuePos)
		out = append(out, mutant.Mutation{
			File:        filePath,
			Line:        pos.Line,
			Mutator:     m.mutatorName,
			Description: m.verb + " " + originalValue + " to " + mutatedValue,
			Apply:       func() { lit.Value = mutatedValue },
			Revert:      func() { lit.Value = originalValue },
		})

		return true
	})

	return out
}

func NewFloatDecrement() *literalMutator {
	return &literalMutator{
		mutatorName: "float_decrement",
		kind:        token.FLOAT,
		verb:        "decremented",
		transform: func(s string) (string, error) {
			f, err := strconv.ParseFloat(s, 64)
			if err != nil {
				return "", err
			}

			return fmt.Sprintf("%v", f-1.0), nil
		},
	}
}

func NewFloatIncrement() *literalMutator {
	return &literalMutator{
		mutatorName: "float_increment",
		kind:        token.FLOAT,
		verb:        "incremented",
		transform: func(s string) (string, error) {
			f, err := strconv.ParseFloat(s, 64)
			if err != nil {
				return "", err
			}

			return fmt.Sprintf("%v", f+1.0), nil
		},
	}
}

func NewIntegerDecrement() *literalMutator {
	return &literalMutator{
		mutatorName: "integer_decrement",
		kind:        token.INT,
		verb:        "decremented",
		transform: func(s string) (string, error) {
			v, err := strconv.Atoi(s)
			if err != nil {
				return "", err
			}

			return strconv.Itoa(v - 1), nil
		},
	}
}

func NewIntegerIncrement() *literalMutator {
	return &literalMutator{
		mutatorName: "integer_increment",
		kind:        token.INT,
		verb:        "incremented",
		transform: func(s string) (string, error) {
			v, err := strconv.Atoi(s)
			if err != nil {
				return "", err
			}

			return strconv.Itoa(v + 1), nil
		},
	}
}
