// SPDX-License-Identifier: BUSL-1.1

package parser

import (
	"github.com/hazyhaar/js55/pkg/js55/lexer"
)

// PreParser performs fast, allocation-free syntax pre-validation before full parsing.
// It verifies balanced braces/parentheses and determines variable scope escapes.
type PreParser struct {
	tokens   []lexer.Token
	pos      int
	hasError bool
}

// NewPreParser initializes a lightweight preparser over a token stream.
func NewPreParser(tokens []lexer.Token) *PreParser {
	return &PreParser{
		tokens:   tokens,
		pos:      0,
		hasError: false,
	}
}

// Validate performs syntax pre-validation without allocating an AST.
func (p *PreParser) Validate() bool {
	parenDepth := 0
	braceDepth := 0

	for p.pos < len(p.tokens) {
		tok := p.tokens[p.pos]
		p.pos++

		switch tok.Kind {
		case lexer.LParen:
			parenDepth++
		case lexer.RParen:
			parenDepth--
			if parenDepth < 0 {
				return false
			}
		case lexer.LBrace:
			braceDepth++
		case lexer.RBrace:
			braceDepth--
			if braceDepth < 0 {
				return false
			}
		}
	}

	return parenDepth == 0 && braceDepth == 0
}
