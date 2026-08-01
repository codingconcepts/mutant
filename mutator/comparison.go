package mutator

import "go/token"

// NewComparison creates a mutator that nudges comparison boundary operators:
//
// < <-> <=
// > <-> >=
//
// Tests off-by-one coverage in boundary conditions.
func NewComparison() *tokenSwapMutator {
	return &tokenSwapMutator{
		name: "comparison",
		mutations: map[token.Token]token.Token{
			token.LSS: token.LEQ,
			token.LEQ: token.LSS,
			token.GTR: token.GEQ,
			token.GEQ: token.GTR,
		},
		extract: extractBinaryExpr,
	}
}
