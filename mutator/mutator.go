package mutator

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"

	"github.com/codingconcepts/mutant"
)

// Registry contains all built-in mutation strategies ("viruses"). The CLI
// uses all of them by default; --viruses filters this list by name.
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

// ByName filters the Registry to only include mutators matching the given
// names (case-insensitive). Returns all mutators if names is empty.
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

// tokenSwapMutator is a generic mutator that replaces one Go token with
// another (e.g. + with -, < with <=). It handles the bulk of the mutation
// strategies: arithmetic, bitwise, comparison, loop_break, etc.
//
// mutations maps original tokens to their replacements.
// extract pulls the relevant token/position from an AST node.
// skip optionally excludes certain nodes (e.g. string concatenation for arithmetic).
type tokenSwapMutator struct {
	mutations map[token.Token]token.Token
	extract   func(ast.Node) (original token.Token, pos token.Pos, apply func(token.Token), ok bool)
	skip      func(ast.Node) bool
	name      string
}

// Name returns the mutator's identifier (e.g. "arithmetic", "comparison").
func (m *tokenSwapMutator) Name() string { return m.name }

// Mutate walks the AST, extracts matching tokens via the extract function,
// looks up their replacement in the mutations map, and produces a Mutation
// for each match.
func (m *tokenSwapMutator) Mutate(fset *token.FileSet, file *ast.File, filePath string, original []byte) []mutant.Mutation {
	var out []mutant.Mutation

	ast.Inspect(file, func(n ast.Node) bool {
		tok, pos, apply, ok := m.extract(n)
		if !ok {
			return true
		}

		if m.skip != nil && m.skip(n) {
			return true
		}

		mutated, ok := m.mutations[tok]
		if !ok {
			return true
		}

		out = append(out, newMutation(filePath, m.name, fset.Position(pos).Line,
			fmt.Sprintf("replaced %s with %s", tok, mutated),
			func() { apply(mutated) },
			func() { apply(tok) },
		))

		return true
	})

	return out
}

// extractBinaryExpr extracts the operator from a BinaryExpr node (e.g. +, -, <, &&).
func extractBinaryExpr(n ast.Node) (token.Token, token.Pos, func(token.Token), bool) {
	expr, ok := n.(*ast.BinaryExpr)
	if !ok {
		return 0, 0, nil, false
	}

	return expr.Op, expr.OpPos, func(t token.Token) { expr.Op = t }, true
}

// extractAssignStmt extracts the assignment operator from an AssignStmt (e.g. +=, -=).
func extractAssignStmt(n ast.Node) (token.Token, token.Pos, func(token.Token), bool) {
	stmt, ok := n.(*ast.AssignStmt)
	if !ok {
		return 0, 0, nil, false
	}

	return stmt.Tok, stmt.TokPos, func(t token.Token) { stmt.Tok = t }, true
}

// extractBranchStmt extracts the keyword from a BranchStmt (break or continue).
func extractBranchStmt(n ast.Node) (token.Token, token.Pos, func(token.Token), bool) {
	stmt, ok := n.(*ast.BranchStmt)
	if !ok {
		return 0, 0, nil, false
	}

	return stmt.Tok, stmt.TokPos, func(t token.Token) { stmt.Tok = t }, true
}

func newMutation(filePath, mutatorName string, line int, description string, apply, revert func()) mutant.Mutation {
	return mutant.Mutation{
		File:        filePath,
		Line:        line,
		Mutator:     mutatorName,
		Description: description,
		Apply:       apply,
		Revert:      revert,
	}
}
