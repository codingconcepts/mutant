package mutator

import "go/token"

func NewComparisonInvert() *binaryExprMutator {
	return &binaryExprMutator{
		name: "comparison_invert",
		mutations: map[token.Token]token.Token{
			token.GTR: token.LEQ,
			token.LEQ: token.GTR,
			token.LSS: token.GEQ,
			token.GEQ: token.LSS,
			token.EQL: token.NEQ,
			token.NEQ: token.EQL,
		},
	}
}
