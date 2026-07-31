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

type tokenSwapMutator struct {
	mutations map[token.Token]token.Token
	extract   func(ast.Node) (original token.Token, pos token.Pos, apply func(token.Token), ok bool)
	name      string
}

func (m *tokenSwapMutator) Name() string { return m.name }

func (m *tokenSwapMutator) Mutate(fset *token.FileSet, file *ast.File, filePath string, original []byte) []mutant.Mutation {
	var out []mutant.Mutation

	ast.Inspect(file, func(n ast.Node) bool {
		tok, pos, apply, ok := m.extract(n)
		if !ok {
			return true
		}

		mutated, ok := m.mutations[tok]
		if !ok {
			return true
		}

		originalTok := tok
		position := fset.Position(pos)
		out = append(out, mutant.Mutation{
			File:        filePath,
			Line:        position.Line,
			Mutator:     m.name,
			Description: "replaced " + originalTok.String() + " with " + mutated.String(),
			Apply:       func() { apply(mutated) },
			Revert:      func() { apply(originalTok) },
		})

		return true
	})

	return out
}

func extractBinaryExpr(n ast.Node) (token.Token, token.Pos, func(token.Token), bool) {
	expr, ok := n.(*ast.BinaryExpr)
	if !ok {
		return 0, 0, nil, false
	}

	return expr.Op, expr.OpPos, func(t token.Token) { expr.Op = t }, true
}

func extractAssignStmt(n ast.Node) (token.Token, token.Pos, func(token.Token), bool) {
	stmt, ok := n.(*ast.AssignStmt)
	if !ok {
		return 0, 0, nil, false
	}

	return stmt.Tok, stmt.TokPos, func(t token.Token) { stmt.Tok = t }, true
}
