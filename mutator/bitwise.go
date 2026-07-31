package mutator

import "go/token"

func NewBitwise() *binaryExprMutator {
	return &binaryExprMutator{
		name: "bitwise",
		mutations: map[token.Token]token.Token{
			token.AND:     token.OR,
			token.OR:      token.AND,
			token.XOR:     token.AND,
			token.AND_NOT: token.AND,
			token.SHL:     token.SHR,
			token.SHR:     token.SHL,
		},
	}
}
