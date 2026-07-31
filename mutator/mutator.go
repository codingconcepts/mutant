package mutator

import (
	"go/ast"
	"go/token"
	"strings"

	"github.com/codingconcepts/mutant"
)

var Registry = []mutant.Mutator{
	NewArithmetic(),
	NewArithmeticAssignment(),
	NewArithmeticAssignmentInvert(),
	NewBitwise(),
	NewComparison(),
	NewComparisonInvert(),
	NewComparisonReplace(),
	NewFloatDecrement(),
	NewFloatIncrement(),
	NewIntegerDecrement(),
	NewIntegerIncrement(),
	NewLoopBreak(),
	NewLoopCondition(),
	NewRangeBreak(),
	NewCancelNil(),
}

func ByName(names []string) []mutant.Mutator {
	if len(names) == 0 {
		return Registry
	}

	want := make(map[string]bool)
	for _, n := range names {
		want[strings.ToLower(strings.TrimSpace(n))] = true
	}

	var out []mutant.Mutator

	for _, m := range Registry {
		if want[strings.ToLower(m.Name())] {
			out = append(out, m)
		}
	}

	return out
}

type binaryExprMutator struct {
	name      string
	mutations map[token.Token]token.Token
}

func (m *binaryExprMutator) Name() string { return m.name }

func (m *binaryExprMutator) Mutate(fset *token.FileSet, file *ast.File, filePath string, original []byte) []mutant.Mutation {
	var out []mutant.Mutation

	ast.Inspect(file, func(n ast.Node) bool {
		expr, ok := n.(*ast.BinaryExpr)
		if !ok {
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

type assignStmtMutator struct {
	name      string
	mutations map[token.Token]token.Token
}

func (m *assignStmtMutator) Name() string { return m.name }

func (m *assignStmtMutator) Mutate(fset *token.FileSet, file *ast.File, filePath string, original []byte) []mutant.Mutation {
	var out []mutant.Mutation

	ast.Inspect(file, func(n ast.Node) bool {
		stmt, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}

		mutated, ok := m.mutations[stmt.Tok]
		if !ok {
			return true
		}

		originalTok := stmt.Tok
		pos := fset.Position(stmt.TokPos)
		out = append(out, mutant.Mutation{
			File:        filePath,
			Line:        pos.Line,
			Mutator:     m.name,
			Description: "replaced " + originalTok.String() + " with " + mutated.String(),
			Apply:       func() { stmt.Tok = mutated },
			Revert:      func() { stmt.Tok = originalTok },
		})

		return true
	})

	return out
}
