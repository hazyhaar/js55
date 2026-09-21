package parser

import (
	"github.com/hazyhaar/js55/pkg/js55/ast"
	"github.com/hazyhaar/js55/pkg/js55/lexer"
)

func (p *parser) parsePrimary() ast.Expr {
	p.enter()
	defer p.leave()

	p.regexpHere()
	t := p.tok
	start := t.Pos

	switch t.Kind {
	case lexer.Number:
		p.advance()
		return &ast.NumberLit{Base: ast.Base{P: start}, Raw: t.Value}

	case lexer.BigInt:
		p.advance()
		return &ast.BigIntLit{Base: ast.Base{P: start}, Raw: t.Value}

	case lexer.String:
		p.advance()
		return &ast.StringLit{Base: ast.Base{P: start}, Raw: t.Value}

	case lexer.Regexp:
		p.advance()
		return &ast.RegExpLit{Base: ast.Base{P: start}, Raw: t.Value}

	case lexer.NoSubTemplate, lexer.TemplateHead:
		return p.parseTemplate()

	case lexer.PrivateIdent:
		p.advance()
		return &ast.PrivateName{Base: ast.Base{P: start}, Name: t.Value}

	case lexer.Ident:
		// « async function » et « async function* » ouvrent une expression de
		// fonction asynchrone. « async » étant lexé en identifiant et non en
		// mot réservé, ce cas ne peut pas vivre dans le commutateur des mots
		// réservés plus bas — c'était la cause dominante des refus à tort
		// mesurés sur test262 (motif « point-virgule attendu, * trouvé »).
		if t.Value == "async" {
			save := p.lx.Save()
			p.advance()
			if p.isKeyword("function") && !p.tok.NewlineBefore {
				return p.parseFunctionAt(start, true)
			}
			p.lx.Restore(save)
			p.tok = t
		}
		p.advance()
		return &ast.Ident{Base: ast.Base{P: start}, Name: identName(t.Value)}

	case lexer.LBracket:
		return p.parseArrayLit()

	case lexer.LBrace:
		return p.parseObjectLit()

	case lexer.LParen:
		return p.parseParenOrArrow()

	case lexer.Keyword:
		switch t.Value {
		case "this":
			p.advance()
			return &ast.ThisExpr{Base: ast.Base{P: start}}
		case "super":
			p.advance()
			return &ast.SuperExpr{Base: ast.Base{P: start}}
		case "true", "false":
			p.advance()
			return &ast.BoolLit{Base: ast.Base{P: start}, Value: t.Value == "true"}
		case "null":
			p.advance()
			return &ast.NullLit{Base: ast.Base{P: start}}
		case "function":
			return p.parseFunction(false)
		case "class":
			return p.parseClass(false)
		case "async":
			// « async function » ouvre une fonction asynchrone ; ailleurs
			// « async » est un identifiant ordinaire.
			save := p.lx.Save()
			asyncTok := p.tok
			p.advance()
			if p.isKeyword("function") && !p.tok.NewlineBefore {
				return p.parseFunctionAt(start, true)
			}
			p.lx.Restore(save)
			p.tok = asyncTok
			p.advance()
			return &ast.Ident{Base: ast.Base{P: start}, Name: "async"}
		case "import":
			p.advance()
			if p.is(lexer.Dot) {
				p.advance()
				if !p.isName() || p.tok.Value != "meta" {
					p.fail("« import.meta » attendu")
				}
				p.advance()
				return &ast.MetaProperty{Base: ast.Base{P: start}, Meta: "import", Property: "meta"}
			}
			if p.is(lexer.LParen) {
				args := p.parseArguments()
				callee := &ast.Ident{Base: ast.Base{P: start}, Name: "import"}
				return &ast.CallExpr{Base: ast.Base{P: start}, Callee: callee, Args: args}
			}
			p.fail("« import » inattendu en position d'expression")
		default:
			// Les mots réservés en mode strict seulement restent des
			// identifiants hors de ce mode.
			if !p.strict && isStrictOnlyReserved(t.Value) {
				p.advance()
				return &ast.Ident{Base: ast.Base{P: start}, Name: t.Value}
			}
			if t.Value == "yield" && !p.inGenerator {
				if p.strict {
					p.fail("« yield » est réservé en mode strict")
				}
				p.advance()
				return &ast.Ident{Base: ast.Base{P: start}, Name: "yield"}
			}
			if t.Value == "await" && !p.inAsync && !p.module {
				p.advance()
				return &ast.Ident{Base: ast.Base{P: start}, Name: "await"}
			}
		}
	}

	p.fail("expression attendue, « " + describeToken(t) + " » trouvé")
	return nil
}

func describeToken(t lexer.Token) string {
	if t.Value != "" {
		return t.Value
	}
	return t.Kind.String()
}

func isStrictOnlyReserved(w string) bool {
	switch w {
	case "let", "static", "implements", "interface", "package",
		"private", "protected", "public":
		return true
	}
	return false
}

// parseTemplate lit un gabarit complet, substitutions comprises.
func (p *parser) parseTemplate() *ast.TemplateLit {
	start := p.tok.Pos
	lit := &ast.TemplateLit{Base: ast.Base{P: start}}

	if p.is(lexer.NoSubTemplate) {
		lit.Quasis = append(lit.Quasis, p.tok.Value)
		p.advance()
		return lit
	}

	lit.Quasis = append(lit.Quasis, p.tok.Value) // tête, jusqu'à ${
	p.advance()
	for {
		p.regexpHere()
		lit.Exprs = append(lit.Exprs, p.parseExpression())

		if !p.is(lexer.RBrace) {
			p.fail("« } » attendu pour clore la substitution du gabarit")
		}
		// Le lexer doit relire depuis l'accolade en mode gabarit.
		p.lx.SeekTo(p.tok.Pos)
		cont := p.lx.NextTemplateContinuation()
		if cont.Kind == lexer.Illegal {
			p.failAt(cont.Message, cont.Pos)
		}
		lit.Quasis = append(lit.Quasis, cont.Value)
		if cont.Kind == lexer.TemplateTail {
			p.advance()
			return lit
		}
		p.advance()
	}
}

func (p *parser) parseArrayLit() ast.Expr {
	start := p.tok.Pos
	p.expect(lexer.LBracket)
	savedNoIn := p.noIn
	p.noIn = false
	defer func() { p.noIn = savedNoIn }()
	lit := &ast.ArrayLit{Base: ast.Base{P: start}}
	for {
		p.regexpHere()
		if p.is(lexer.RBracket) {
			break
		}
		if p.is(lexer.Comma) {
			p.advance()
			lit.Elements = append(lit.Elements, nil) // case élidée
			continue
		}
		if p.is(lexer.Ellipsis) {
			sp := p.tok.Pos
			p.advance()
			p.regexpHere()
			lit.Elements = append(lit.Elements,
				&ast.SpreadElement{Base: ast.Base{P: sp}, Argument: p.parseAssign()})
			if p.eat(lexer.Comma) {
				if p.is(lexer.RBracket) {
					lit.RestTrailingComma = true
					break
				}
			} else {
				break
			}
			continue
		} else {
			lit.Elements = append(lit.Elements, p.parseAssign())
		}
		if !p.eat(lexer.Comma) {
			break
		}
	}
	p.expect(lexer.RBracket)
	return lit
}

func (p *parser) parseObjectLit() ast.Expr {
	start := p.tok.Pos
	p.expect(lexer.LBrace)
	savedNoIn := p.noIn
	p.noIn = false
	defer func() { p.noIn = savedNoIn }()
	lit := &ast.ObjectLit{Base: ast.Base{P: start}}
	for !p.is(lexer.RBrace) {
		prop := p.parseObjectMember()
		lit.Properties = append(lit.Properties, prop)
		if !p.eat(lexer.Comma) {
			break
		}
		if p.is(lexer.RBrace) {
			if prop.Kind == "spread" {
				lit.RestTrailingComma = true
			}
			break
		}
	}
	p.expect(lexer.RBrace)
	return lit
}

func (p *parser) parseObjectMember() *ast.Property {
	start := p.tok.Pos

	if p.is(lexer.Ellipsis) {
		p.advance()
		p.regexpHere()
		return &ast.Property{Base: ast.Base{P: start}, Kind: "spread", Value: p.parseAssign()}
	}

	async, generator := false, false

	// « async » et « * » précèdent une méthode ; « async » seul reste un nom.
	// L'interdiction de terminateur de ligne porte sur ce qui SUIT « async »,
	// jamais sur ce qui le précède : un membre écrit sur sa propre ligne est le
	// cas ordinaire. Tester NewlineBefore sur « async » lui-même écartait
	// silencieusement toutes les méthodes asynchrones d'un corps multiligne.
	if p.isName() && p.tok.Value == "async" {
		save := p.lx.Save()
		tk := p.tok
		p.advance()
		if p.tok.NewlineBefore || p.is(lexer.Colon) || p.is(lexer.Comma) ||
			p.is(lexer.RBrace) || p.is(lexer.LParen) || p.is(lexer.Assign) {
			p.lx.Restore(save)
			p.tok = tk
		} else {
			async = true
		}
	}
	if p.eat(lexer.Star) {
		generator = true
	}

	// Accesseurs : get / set suivis d'un nom de propriété.
	if !async && !generator && p.isName() && (p.tok.Value == "get" || p.tok.Value == "set") {
		kind := p.tok.Value
		save := p.lx.Save()
		tk := p.tok
		p.advance()
		if p.is(lexer.Colon) || p.is(lexer.Comma) || p.is(lexer.RBrace) ||
			p.is(lexer.LParen) || p.is(lexer.Assign) {
			p.lx.Restore(save)
			p.tok = tk
		} else {
			key, computed := p.parsePropertyKey()
			fn := p.parseMethodTail(start, false, false)
			return &ast.Property{Base: ast.Base{P: start}, Key: key, Value: fn,
				Computed: computed, Kind: kind, Method: true}
		}
	}

	key, computed := p.parsePropertyKey()

	switch {
	case p.is(lexer.LParen):
		fn := p.parseMethodTail(start, generator, async)
		return &ast.Property{Base: ast.Base{P: start}, Key: key, Value: fn,
			Computed: computed, Kind: "init", Method: true}

	case p.eat(lexer.Colon):
		p.regexpHere()
		return &ast.Property{Base: ast.Base{P: start}, Key: key, Value: p.parseAssign(),
			Computed: computed, Kind: "init"}

	case p.is(lexer.Assign):
		// Forme abrégée avec valeur par défaut : licite seulement dans un motif
		// de décomposition, ce que toPattern vérifiera.
		p.advance()
		p.regexpHere()
		def := p.parseAssign()
		id, ok := key.(*ast.Ident)
		if !ok {
			p.failAt("valeur par défaut sur une clé non identifiante", start)
		}
		return &ast.Property{Base: ast.Base{P: start}, Key: id, Kind: "init", Shorthand: true,
			Value: &ast.AssignPattern{Base: ast.Base{P: start}, Target: id, Default: def}}

	default:
		id, ok := key.(*ast.Ident)
		if !ok || computed {
			p.fail("« : » attendu après le nom de propriété")
		}
		return &ast.Property{Base: ast.Base{P: start}, Key: id, Value: id,
			Kind: "init", Shorthand: true}
	}
}

// parsePropertyKey lit une clé de propriété et indique si elle est calculée.
func (p *parser) parsePropertyKey() (ast.Expr, bool) {
	t := p.tok
	switch {
	case p.is(lexer.LBracket):
		p.advance()
		p.regexpHere()
		k := p.parseAssign()
		p.expect(lexer.RBracket)
		return k, true
	case p.is(lexer.String):
		p.advance()
		return &ast.StringLit{Base: ast.Base{P: t.Pos}, Raw: t.Value}, false
	case p.is(lexer.Number):
		p.advance()
		return &ast.NumberLit{Base: ast.Base{P: t.Pos}, Raw: t.Value}, false
	case p.is(lexer.BigInt):
		p.advance()
		return &ast.BigIntLit{Base: ast.Base{P: t.Pos}, Raw: t.Value}, false
	case p.is(lexer.PrivateIdent):
		p.advance()
		return &ast.PrivateName{Base: ast.Base{P: t.Pos}, Name: t.Value}, false
	case p.isName():
		p.advance()
		return &ast.Ident{Base: ast.Base{P: t.Pos}, Name: identName(t.Value)}, false
	}
	p.fail("nom de propriété attendu")
	return nil, false
}

// parseMethodTail lit la liste de paramètres et le corps d'une méthode.
func (p *parser) parseMethodTail(start lexer.Position, generator, async bool) *ast.FunctionExpr {
	fn := &ast.FunctionExpr{Base: ast.Base{P: start}, Generator: generator, Async: async}
	savedGen, savedAsync, savedFn := p.inGenerator, p.inAsync, p.inFunction
	p.inGenerator, p.inAsync, p.inFunction = generator, async, true
	defer func() { p.inGenerator, p.inAsync, p.inFunction = savedGen, savedAsync, savedFn }()

	fn.Params = p.parseParams()
	fn.Body = p.parseBlock()
	return fn
}

// parseParams lit une liste de paramètres formels.
func (p *parser) parseParams() []ast.Expr {
	p.expect(lexer.LParen)
	params := []ast.Expr{}
	for !p.is(lexer.RParen) {
		if p.is(lexer.Ellipsis) {
			start := p.tok.Pos
			p.advance()
			p.regexpHere()
			params = append(params,
				&ast.RestElement{Base: ast.Base{P: start}, Argument: p.toPattern(p.parseAssign())})
			break
		}
		p.regexpHere()
		params = append(params, p.toPattern(p.parseAssign()))
		if !p.eat(lexer.Comma) {
			break
		}
	}
	p.expect(lexer.RParen)
	return params
}

// parseParenOrArrow applique la grammaire de couverture : la liste entre
// parenthèses se lit d'abord comme une suite d'expressions, puis se réinterprète
// en paramètres si une flèche suit.
func (p *parser) parseParenOrArrow() ast.Expr {
	start := p.tok.Pos
	p.expect(lexer.LParen)
	savedNoIn := p.noIn
	p.noIn = false
	defer func() { p.noIn = savedNoIn }()

	items := []ast.Expr{}
	sawSpread := false
	trailingComma := false

	for !p.is(lexer.RParen) {
		if p.is(lexer.Ellipsis) {
			sp := p.tok.Pos
			p.advance()
			p.regexpHere()
			items = append(items, &ast.RestElement{Base: ast.Base{P: sp}, Argument: p.parseAssign()})
			sawSpread = true
			break
		}
		p.regexpHere()
		items = append(items, p.parseAssign())
		if !p.eat(lexer.Comma) {
			break
		}
		if p.is(lexer.RParen) {
			trailingComma = true
		}
	}
	p.expect(lexer.RParen)

	if p.is(lexer.Arrow) && !p.tok.NewlineBefore {
		p.advance()
		return p.parseArrowBody(items, start, false)
	}

	// Sans flèche, ni élément de reste ni liste vide ni virgule finale ne sont
	// des expressions valides.
	if sawSpread {
		p.failAt("élément de reste hors liste de paramètres", start)
	}
	if len(items) == 0 {
		p.failAt("« () » n'est une expression que suivi de « => »", start)
	}
	if trailingComma {
		p.failAt("virgule finale hors liste de paramètres", start)
	}
	if len(items) == 1 {
		p.parens[items[0]] = true
		return items[0]
	}
	seq := &ast.SequenceExpr{Base: ast.Base{P: start}, Exprs: items}
	p.parens[seq] = true
	return seq
}

// ─── Fonctions et classes ───────────────────────────────────────────────────

func (p *parser) parseFunction(declaration bool) ast.Expr {
	return p.parseFunctionAt(p.tok.Pos, false)
}

// parseFunctionDecl lit une déclaration de fonction, qui exige un nom là où
// l'expression de fonction l'admet facultatif.
func (p *parser) parseFunctionDecl(start lexer.Position, async bool) *ast.FunctionExpr {
	fn := p.parseFunctionAt(start, async).(*ast.FunctionExpr)
	if fn.Name == nil {
		p.failAt("une déclaration de fonction exige un nom", start)
	}
	return fn
}

func (p *parser) parseFunctionAt(start lexer.Position, async bool) ast.Expr {
	if !p.eatKeyword("function") {
		p.fail("« function » attendu")
	}
	generator := p.eat(lexer.Star)

	fn := &ast.FunctionExpr{Base: ast.Base{P: start}, Generator: generator, Async: async}
	if p.tok.Kind == lexer.Ident || (p.tok.Kind == lexer.Keyword && !p.strict && isStrictOnlyReserved(p.tok.Value)) {
		fn.Name = &ast.Ident{Base: ast.Base{P: p.tok.Pos}, Name: p.tok.Value}
		p.advance()
	}

	savedGen, savedAsync, savedFn := p.inGenerator, p.inAsync, p.inFunction
	p.inGenerator, p.inAsync, p.inFunction = generator, async, true
	defer func() { p.inGenerator, p.inAsync, p.inFunction = savedGen, savedAsync, savedFn }()

	fn.Params = p.parseParams()
	fn.Body = p.parseBlock()
	return fn
}

func (p *parser) parseClass(declaration bool) ast.Expr {
	start := p.tok.Pos
	p.expect(lexer.Keyword) // class

	// Le corps d'une classe est toujours en mode strict.
	savedStrict := p.strict
	p.strict = true
	defer func() { p.strict = savedStrict }()

	cl := &ast.ClassExpr{Base: ast.Base{P: start}}
	if p.tok.Kind == lexer.Ident {
		cl.Name = &ast.Ident{Base: ast.Base{P: p.tok.Pos}, Name: p.tok.Value}
		p.advance()
	}
	if p.eatKeyword("extends") {
		cl.SuperClass = p.parseCallOrMember(true)
	}

	p.expect(lexer.LBrace)
	for !p.is(lexer.RBrace) {
		if p.eat(lexer.Semicolon) {
			continue
		}
		cl.Body = append(cl.Body, p.parseClassMember())
	}
	p.expect(lexer.RBrace)
	return cl
}

func (p *parser) parseClassMember() *ast.ClassMember {
	start := p.tok.Pos
	static := false

	if p.isName() && p.tok.Value == "static" {
		save := p.lx.Save()
		tk := p.tok
		p.advance()
		if p.is(lexer.LParen) || p.is(lexer.Assign) || p.is(lexer.Semicolon) || p.is(lexer.RBrace) {
			p.lx.Restore(save)
			p.tok = tk
		} else if p.is(lexer.LBrace) {
			body := p.parseBlock()
			return &ast.ClassMember{Base: ast.Base{P: start}, Kind: "static-block",
				Static: true, Value: &ast.FunctionExpr{Base: ast.Base{P: start}, Body: body}}
		} else {
			static = true
		}
	}

	async, generator := false, false
	// Même règle que pour les littéraux objet : le terminateur de ligne
	// interdit est celui qui suivrait « async », pas celui qui le précède.
	if p.isName() && p.tok.Value == "async" {
		save := p.lx.Save()
		tk := p.tok
		p.advance()
		if p.tok.NewlineBefore || p.is(lexer.LParen) || p.is(lexer.Assign) ||
			p.is(lexer.Semicolon) || p.is(lexer.RBrace) {
			p.lx.Restore(save)
			p.tok = tk
		} else {
			async = true
		}
	}
	if p.eat(lexer.Star) {
		generator = true
	}

	if !async && !generator && p.isName() && (p.tok.Value == "get" || p.tok.Value == "set") {
		kind := p.tok.Value
		save := p.lx.Save()
		tk := p.tok
		p.advance()
		if p.is(lexer.LParen) || p.is(lexer.Assign) || p.is(lexer.Semicolon) || p.is(lexer.RBrace) {
			p.lx.Restore(save)
			p.tok = tk
		} else {
			key, computed := p.parsePropertyKey()
			fn := p.parseMethodTail(start, false, false)
			return &ast.ClassMember{Base: ast.Base{P: start}, Key: key, Value: fn,
				Kind: kind, Static: static, Computed: computed}
		}
	}

	key, computed := p.parsePropertyKey()

	if p.is(lexer.LParen) {
		fn := p.parseMethodTail(start, generator, async)
		kind := "method"
		if id, ok := key.(*ast.Ident); ok && !static && !computed && id.Name == "constructor" {
			kind = "constructor"
		}
		return &ast.ClassMember{Base: ast.Base{P: start}, Key: key, Value: fn,
			Kind: kind, Static: static, Computed: computed}
	}

	m := &ast.ClassMember{Base: ast.Base{P: start}, Key: key, Kind: "field",
		Static: static, Computed: computed}
	if p.eat(lexer.Assign) {
		p.regexpHere()
		m.Value = p.parseAssign()
	}
	p.semicolon()
	return m
}
