package mutator

import "go/token"

func NewComparison() *binaryExprMutator {
	return &binaryExprMutator{
		name: "comparison",
		mutations: map[token.Token]token.Token{
			token.LSS: token.LEQ,
			token.LEQ: token.LSS,
			token.GTR: token.GEQ,
			token.GEQ: token.GTR,
		},
	}
}
