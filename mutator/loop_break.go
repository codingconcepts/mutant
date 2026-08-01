package mutator

import "go/token"

// NewLoopBreak creates a mutator that swaps break <-> continue in loop bodies.
// Tests whether loop exit/continuation logic is properly covered.
func NewLoopBreak() *tokenSwapMutator {
	return &tokenSwapMutator{
		name: "loop_break",
		mutations: map[token.Token]token.Token{
			token.BREAK:    token.CONTINUE,
			token.CONTINUE: token.BREAK,
		},
		extract: extractBranchStmt,
	}
}
