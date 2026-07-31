package mutator

import (
	"go/ast"
	"go/token"

	"github.com/codingconcepts/mutant"
)

type rangeBreak struct{}

func NewRangeBreak() *rangeBreak { return &rangeBreak{} }

func (m *rangeBreak) Name() string { return "range_break" }

func (m *rangeBreak) Mutate(fset *token.FileSet, file *ast.File, filePath string, original []byte) []mutant.Mutation {
	var out []mutant.Mutation

	ast.Inspect(file, func(n ast.Node) bool {
		rangeStmt, ok := n.(*ast.RangeStmt)
		if !ok {
			return true
		}

		originalBody := rangeStmt.Body
		mutatedStmts := make([]ast.Stmt, 0, len(originalBody.List)+1)
		mutatedStmts = append(mutatedStmts, &ast.BranchStmt{Tok: token.BREAK})
		mutatedStmts = append(mutatedStmts, originalBody.List...)
		mutatedBody := &ast.BlockStmt{
			Lbrace: originalBody.Lbrace,
			List:   mutatedStmts,
			Rbrace: originalBody.Rbrace,
		}
		pos := fset.Position(rangeStmt.For)
		out = append(out, mutant.Mutation{
			File:        filePath,
			Line:        pos.Line,
			Mutator:     "range_break",
			Description: "added early break to range loop",
			Apply:       func() { rangeStmt.Body = mutatedBody },
			Revert:      func() { rangeStmt.Body = originalBody },
		})

		return true
	})

	return out
}
