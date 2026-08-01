package mutator

import "go/token"

// NewBitwise creates a mutator that swaps bitwise operators:
//
// & <-> |
// ^ -> &
// &^ -> &
// << <-> >>
func NewBitwise() *tokenSwapMutator {
	return &tokenSwapMutator{
		name: "bitwise",
		mutations: map[token.Token]token.Token{
			token.AND:     token.OR,
			token.OR:      token.AND,
			token.XOR:     token.AND,
			token.AND_NOT: token.AND,
			token.SHL:     token.SHR,
			token.SHR:     token.SHL,
		},
		extract: extractBinaryExpr,
	}
}
