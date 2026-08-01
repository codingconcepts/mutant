package mutator

import (
	"go/ast"
	"go/token"

	"github.com/codingconcepts/mutant"
)

type rangeBreak struct{}

// NewRangeBreak creates a mutator that inserts a break statement at the
// beginning of range loop bodies, causing only the first iteration to
// execute. Tests whether iteration behavior is covered.
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
		mutatedBody := withLeadingBreak(originalBody)

		out = append(out, newMutation(filePath, "range_break", fset.Position(rangeStmt.For).Line,
			"added early break to range loop",
			func() { rangeStmt.Body = mutatedBody },
			func() { rangeStmt.Body = originalBody },
		))

		return true
	})

	return out
}

func withLeadingBreak(body *ast.BlockStmt) *ast.BlockStmt {
	stmts := make([]ast.Stmt, 0, len(body.List)+1)
	stmts = append(stmts, &ast.BranchStmt{Tok: token.BREAK})
	stmts = append(stmts, body.List...)

	return &ast.BlockStmt{
		Lbrace: body.Lbrace,
		List:   stmts,
		Rbrace: body.Rbrace,
	}
}
