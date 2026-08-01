package mutator

import (
	"go/ast"
	"go/token"

	"github.com/codingconcepts/mutant"
)

type loopCondition struct{}

// NewLoopCondition creates a mutator that replaces for-loop conditions with
// a false equivalent (0 != 0), causing the loop body to never execute. Only
// targets for-loops with binary expression conditions; skips infinite loops
// (no condition) and range loops.
func NewLoopCondition() *loopCondition { return &loopCondition{} }

func (m *loopCondition) Name() string { return "loop_condition" }

func (m *loopCondition) Mutate(fset *token.FileSet, file *ast.File, filePath string, original []byte) []mutant.Mutation {
	var out []mutant.Mutation

	ast.Inspect(file, func(n ast.Node) bool {
		forStmt, ok := n.(*ast.ForStmt)
		if !ok {
			return true
		}

		if forStmt.Cond == nil {
			return true
		}

		if _, ok := forStmt.Cond.(*ast.BinaryExpr); !ok {
			return true
		}

		originalCond := forStmt.Cond
		falseCond := &ast.BinaryExpr{
			X:  &ast.BasicLit{Kind: token.INT, Value: "0"},
			Op: token.NEQ,
			Y:  &ast.BasicLit{Kind: token.INT, Value: "0"},
		}

		out = append(out, newMutation(filePath, "loop_condition", fset.Position(forStmt.For).Line,
			"replaced loop condition with false",
			func() { forStmt.Cond = falseCond },
			func() { forStmt.Cond = originalCond },
		))

		return true
	})

	return out
}
