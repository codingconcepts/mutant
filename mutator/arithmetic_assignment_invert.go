package mutator

import "go/token"

// NewArithmeticAssignmentInvert creates a mutator that swaps compound
// assignment operators with their inverse: += <-> -=, *= <-> /=, %= -> *=.
func NewArithmeticAssignmentInvert() *tokenSwapMutator {
	return &tokenSwapMutator{
		name: "arithmetic_assignment_invert",
		mutations: map[token.Token]token.Token{
			token.ADD_ASSIGN: token.SUB_ASSIGN,
			token.SUB_ASSIGN: token.ADD_ASSIGN,
			token.MUL_ASSIGN: token.QUO_ASSIGN,
			token.QUO_ASSIGN: token.MUL_ASSIGN,
			token.REM_ASSIGN: token.MUL_ASSIGN,
		},
		extract: extractAssignStmt,
	}
}
