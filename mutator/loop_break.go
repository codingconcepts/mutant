package mutator

import (
	"go/ast"
	"go/token"

	"github.com/codingconcepts/mutant"
)

type loopBreak struct {
	mutations map[token.Token]token.Token
}

func NewLoopBreak() *loopBreak {
	return &loopBreak{
		mutations: map[token.Token]token.Token{
			token.BREAK:    token.CONTINUE,
			token.CONTINUE: token.BREAK,
		},
	}
}

func (m *loopBreak) Name() string { return "loop_break" }

func (m *loopBreak) Mutate(fset *token.FileSet, file *ast.File, filePath string, original []byte) []mutant.Mutation {
	var out []mutant.Mutation

	ast.Inspect(file, func(n ast.Node) bool {
		stmt, ok := n.(*ast.BranchStmt)
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
			Mutator:     "loop_break",
			Description: "replaced " + originalTok.String() + " with " + mutated.String(),
			Apply:       func() { stmt.Tok = mutated },
			Revert:      func() { stmt.Tok = originalTok },
		})

		return true
	})

	return out
}
