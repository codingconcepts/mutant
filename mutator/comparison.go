package mutator

import "go/token"

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
