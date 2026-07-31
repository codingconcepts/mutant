package mutator

import "go/token"

func NewArithmeticAssignment() *assignStmtMutator {
	return &assignStmtMutator{
		name: "arithmetic_assignment",
		mutations: map[token.Token]token.Token{
			token.ADD_ASSIGN:     token.ASSIGN,
			token.SUB_ASSIGN:     token.ASSIGN,
			token.MUL_ASSIGN:     token.ASSIGN,
			token.QUO_ASSIGN:     token.ASSIGN,
			token.REM_ASSIGN:     token.ASSIGN,
			token.AND_ASSIGN:     token.ASSIGN,
			token.OR_ASSIGN:      token.ASSIGN,
			token.XOR_ASSIGN:     token.ASSIGN,
			token.SHL_ASSIGN:     token.ASSIGN,
			token.SHR_ASSIGN:     token.ASSIGN,
			token.AND_NOT_ASSIGN: token.ASSIGN,
		},
	}
}
