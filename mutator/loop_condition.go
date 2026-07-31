package mutator

import (
	"go/ast"
	"go/token"

	"github.com/codingconcepts/mutant"
)

type loopCondition struct{}

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
		pos := fset.Position(forStmt.For)
		out = append(out, mutant.Mutation{
			File:        filePath,
			Line:        pos.Line,
			Mutator:     "loop_condition",
			Description: "replaced loop condition with false",
			Apply:       func() { forStmt.Cond = falseCond },
			Revert:      func() { forStmt.Cond = originalCond },
		})

		return true
	})

	return out
}
