package mutator

import "go/token"

// NewComparisonInvert creates a mutator that negates comparison operators:
//
// > <-> <=
// < <-> >=
// == <-> !=
//
// Tests that comparisons are meaningfully covered.
func NewComparisonInvert() *tokenSwapMutator {
	return &tokenSwapMutator{
		name: "comparison_invert",
		mutations: map[token.Token]token.Token{
			token.GTR: token.LEQ,
			token.LEQ: token.GTR,
			token.LSS: token.GEQ,
			token.GEQ: token.LSS,
			token.EQL: token.NEQ,
			token.NEQ: token.EQL,
		},
		extract: extractBinaryExpr,
	}
}
