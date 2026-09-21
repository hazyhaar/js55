package parser

import (
	"github.com/hazyhaar/js55/pkg/js55/ast"
	"github.com/hazyhaar/js55/pkg/js55/lexer"
)

// parseStatementListItem lit une instruction ou une déclaration.
func (p *parser) parseStatementListItem() ast.Stmt {
	p.enter()
	defer p.leave()

	if p.tok.Kind == lexer.Keyword {
		switch p.tok.Value {
		case "function":
			start := p.tok.Pos
			return &ast.FunctionDecl{Base: ast.Base{P: start}, Fn: p.parseFunctionDecl(start, false)}
		case "class":
			start := p.tok.Pos
			cl := p.parseClass(true).(*ast.ClassExpr)
			return &ast.ClassDecl{Base: ast.Base{P: start}, Class: cl}
		case "const":
			return p.parseVarDecl("const")
		case "let":
			// « let » n'introduit une déclaration que suivi d'un nom, d'un
			// crochet ou d'une accolade ; ailleurs c'est un identifiant.
			if p.letStartsDeclaration() {
				return p.parseVarDecl("let")
			}
		case "import":
			if p.module && p.importIsDeclaration() {
				return p.parseImportDecl()
			}
		case "export":
			if p.module {
				return p.parseExportDecl()
			}
			p.fail("« export » n'est admis que dans un module")
		}
	}
	// « async function » ouvre une déclaration de fonction asynchrone.
	// « async » est lexé en identifiant, non en mot réservé : ce cas ne peut
	// donc pas vivre dans le commutateur ci-dessus.
	if p.tok.Kind == lexer.Ident && p.tok.Value == "async" {
		save := p.lx.Save()
		tk := p.tok
		start := tk.Pos
		p.advance()
		if p.isKeyword("function") && !p.tok.NewlineBefore {
			return &ast.FunctionDecl{Base: ast.Base{P: start}, Fn: p.parseFunctionDecl(start, true)}
		}
		p.lx.Restore(save)
		p.tok = tk
	}

	return p.parseStatement()
}

// letStartsDeclaration distingue « let x = 1 » de « let » employé comme nom.
func (p *parser) letStartsDeclaration() bool {
	save := p.lx.Save()
	tk := p.tok
	p.advance()
	ok := p.tok.Kind == lexer.Ident || p.is(lexer.LBracket) || p.is(lexer.LBrace) ||
		(p.tok.Kind == lexer.Keyword && !p.strict && isStrictOnlyReserved(p.tok.Value))
	p.lx.Restore(save)
	p.tok = tk
	return ok
}

// importIsDeclaration distingue la déclaration d'import de l'import dynamique
// et de import.meta, qui sont des expressions.
func (p *parser) importIsDeclaration() bool {
	save := p.lx.Save()
	tk := p.tok
	p.advance()
	ok := !p.is(lexer.LParen) && !p.is(lexer.Dot)
	p.lx.Restore(save)
	p.tok = tk
	return ok
}

func (p *parser) parseStatement() ast.Stmt {
	p.enter()
	defer p.leave()

	start := p.tok.Pos

	if p.inIteration > 0 {
		if p.tok.Kind == lexer.Keyword {
			switch p.tok.Value {
			case "function", "class", "const":
				p.fail("déclaration interdite en position d'instruction")
			case "let":
				if p.letStartsDeclaration() {
					saveLet := p.lx.Save()
					tkLet := p.tok
					p.advance()
					nextBracket := p.is(lexer.LBracket)
					nl := p.tok.NewlineBefore
					p.lx.Restore(saveLet)
					p.tok = tkLet
					if nextBracket || !nl {
						p.fail("déclaration interdite en position d'instruction")
					}
				}
			}
		}
		if p.tok.Kind == lexer.Ident && p.tok.Value == "async" {
			save := p.lx.Save()
			tk := p.tok
			p.advance()
			if p.isKeyword("function") && !p.tok.NewlineBefore {
				p.fail("déclaration interdite en position d'instruction")
			}
			p.lx.Restore(save)
			p.tok = tk
		}
	}

	switch p.tok.Kind {
	case lexer.LBrace:
		return p.parseBlock()
	case lexer.Semicolon:
		p.advance()
		return &ast.EmptyStmt{Base: ast.Base{P: start}}
	case lexer.Keyword:
		switch p.tok.Value {
		case "var":
			return p.parseVarDecl("var")
		case "if":
			return p.parseIf()
		case "for":
			return p.parseFor()
		case "while":
			return p.parseWhile()
		case "do":
			return p.parseDoWhile()
		case "return":
			return p.parseReturn()
		case "break", "continue":
			return p.parseBreakContinue()
		case "throw":
			return p.parseThrow()
		case "try":
			return p.parseTry()
		case "switch":
			return p.parseSwitch()
		case "debugger":
			p.advance()
			p.semicolon()
			return &ast.DebuggerStmt{Base: ast.Base{P: start}}
		case "with":
			if p.strict {
				p.fail("« with » est interdit en mode strict")
			}
			return p.parseWith()
		}
	}

	// Instruction étiquetée : un identifiant suivi de deux-points.
	if p.tok.Kind == lexer.Ident {
		save := p.lx.Save()
		tk := p.tok
		p.advance()
		if p.is(lexer.Colon) {
			p.advance()
			label := &ast.Ident{Base: ast.Base{P: tk.Pos}, Name: tk.Value}
			return &ast.LabeledStmt{Base: ast.Base{P: start}, Label: label, Body: p.parseStatement()}
		}
		p.lx.Restore(save)
		p.tok = tk
	}

	p.regexpHere()
	expr := p.parseExpression()
	p.semicolon()
	return &ast.ExpressionStmt{Base: ast.Base{P: start}, Expression: expr}
}

func (p *parser) parseBlock() *ast.BlockStmt {
	start := p.tok.Pos
	p.expect(lexer.LBrace)
	b := &ast.BlockStmt{Base: ast.Base{P: start}, Body: []ast.Stmt{}}
	for !p.is(lexer.RBrace) {
		if p.is(lexer.EOF) {
			p.fail("« } » attendu pour clore le bloc")
		}
		b.Body = append(b.Body, p.parseStatementListItem())
	}
	p.expect(lexer.RBrace)
	return b
}

func (p *parser) parseVarDecl(kind string) ast.Stmt {
	start := p.tok.Pos
	p.advance() // var / let / const
	d := p.parseVarDeclList(kind, start)
	p.semicolon()
	return d
}

// parseVarDeclList lit la liste de déclarateurs, sans consommer le
// point-virgule : l'en-tête d'un for en a besoin sans terminaison.
func (p *parser) parseVarDeclList(kind string, start lexer.Position) *ast.VarDecl {
	d := &ast.VarDecl{Base: ast.Base{P: start}, DeclKind: kind}
	for {
		dStart := p.tok.Pos
		target := p.parseBindingTarget()
		decl := &ast.VarDeclarator{Base: ast.Base{P: dStart}, Target: target}
		if p.eat(lexer.Assign) {
			p.regexpHere()
			decl.Init = p.parseAssign()
		}
		d.Decls = append(d.Decls, decl)
		if !p.eat(lexer.Comma) {
			break
		}
	}
	return d
}

// parseBindingTarget lit un nom lié ou un motif de décomposition.
func (p *parser) parseBindingTarget() ast.Expr {
	switch {
	case p.is(lexer.LBracket), p.is(lexer.LBrace):
		return p.toPattern(p.parsePrimary())
	case p.tok.Kind == lexer.Ident:
		t := p.tok
		p.advance()
		return &ast.Ident{Base: ast.Base{P: t.Pos}, Name: identName(t.Value)}
	case p.tok.Kind == lexer.Keyword && !p.strict && isStrictOnlyReserved(p.tok.Value):
		t := p.tok
		p.advance()
		return &ast.Ident{Base: ast.Base{P: t.Pos}, Name: t.Value}
	case p.isKeyword("yield") && !p.inGenerator && !p.strict:
		t := p.tok
		p.advance()
		return &ast.Ident{Base: ast.Base{P: t.Pos}, Name: t.Value}
	case p.isKeyword("await") && !p.inAsync && !p.module:
		t := p.tok
		p.advance()
		return &ast.Ident{Base: ast.Base{P: t.Pos}, Name: t.Value}
	}
	p.fail("nom lié attendu")
	return nil
}

func (p *parser) parseIf() ast.Stmt {
	start := p.tok.Pos
	p.advance()
	p.expect(lexer.LParen)
	p.regexpHere()
	test := p.parseExpression()
	p.expect(lexer.RParen)
	p.regexpHere()
	s := &ast.IfStmt{Base: ast.Base{P: start}, Test: test, Consequent: p.parseStatement()}
	if p.eatKeyword("else") {
		p.regexpHere()
		s.Alternate = p.parseStatement()
	}
	return s
}

func (p *parser) parseWhile() ast.Stmt {
	start := p.tok.Pos
	p.advance()
	p.expect(lexer.LParen)
	p.regexpHere()
	test := p.parseExpression()
	p.expect(lexer.RParen)
	p.inIteration++
	defer func() { p.inIteration-- }()
	p.regexpHere()
	return &ast.WhileStmt{Base: ast.Base{P: start}, Test: test, Body: p.parseStatement()}
}

func (p *parser) parseDoWhile() ast.Stmt {
	start := p.tok.Pos
	p.advance()
	p.inIteration++
	p.regexpHere()
	body := p.parseStatement()
	p.inIteration--
	if !p.eatKeyword("while") {
		p.fail("« while » attendu après le corps d'un do")
	}
	p.expect(lexer.LParen)
	p.regexpHere()
	test := p.parseExpression()
	p.expect(lexer.RParen)
	p.eat(lexer.Semicolon) // point-virgule facultatif après do-while
	return &ast.DoWhileStmt{Base: ast.Base{P: start}, Body: body, Test: test}
}

func (p *parser) parseFor() ast.Stmt {
	start := p.tok.Pos
	p.advance()

	await := false
	if p.isKeyword("await") && (p.inAsync || p.module) {
		saveAw := p.lx.Save()
		tkAw := p.tok
		p.advance()
		if p.tok.Kind == lexer.Ident && p.tok.Value == "using" {
			p.lx.Restore(saveAw)
			p.tok = tkAw
		} else {
			await = true
		}
	}
	p.expect(lexer.LParen)

	p.inIteration++
	defer func() { p.inIteration-- }()

	// En-tête vide : for (;;)
	if p.is(lexer.Semicolon) {
		p.advance()
		return p.parseForTail(start, nil)
	}

	var init ast.Node

	awaitUsing := p.isKeyword("await")
	if awaitUsing {
		saveAU := p.lx.Save()
		tkAU := p.tok
		p.advance()
		if !(p.tok.Kind == lexer.Ident && p.tok.Value == "using") {
			p.lx.Restore(saveAU)
			p.tok = tkAU
			awaitUsing = false
		} else {
			p.lx.Restore(saveAU)
			p.tok = tkAU
		}
	}
	usingDecl := (p.tok.Kind == lexer.Ident && p.tok.Value == "using") || awaitUsing
	if usingDecl {
		saveU := p.lx.Save()
		tkU := p.tok
		p.advance()
		okU := p.tok.Kind == lexer.Ident || p.is(lexer.LBracket) || p.is(lexer.LBrace)
		p.lx.Restore(saveU)
		p.tok = tkU
		if !okU {
			usingDecl = false
		}
	}
	if (p.tok.Kind == lexer.Keyword &&
		(p.tok.Value == "var" || p.tok.Value == "const" ||
			(p.tok.Value == "let" && p.letStartsDeclaration()))) || usingDecl {
		kind := p.tok.Value
		if usingDecl {
			kind = "const"
		}
		declStart := p.tok.Pos
		if awaitUsing {
			p.advance()
			p.advance()
		} else {
			p.advance()
		}

		// Un seul nom lié sans initialiseur peut ouvrir un for-in ou un for-of.
		target := p.parseBindingTarget()
		if kind == "let" || kind == "const" {
			seen := map[string]int{}
			if dup := boundNameDup(target, seen); dup != "" {
				p.fail("nom lié en double : " + dup)
			}
		}
		if p.isKeyword("in") || (p.isName() && p.tok.Value == "of") {
			of := p.tok.Value == "of"
			p.advance()
			p.regexpHere()
			right := p.parseAssign()
			p.expect(lexer.RParen)
			p.regexpHere()
			left := &ast.VarDecl{Base: ast.Base{P: declStart}, DeclKind: kind,
				Decls: []*ast.VarDeclarator{{Base: ast.Base{P: target.Pos()}, Target: target}}}
			body := p.parseStatement()
			if kind == "let" || kind == "const" {
				checkForDeclEarlyErrors(p, target, body)
			}
			return &ast.ForInStmt{Base: ast.Base{P: start}, Left: left, Right: right,
				Body: body, Of: of, Await: await}
		}

		decl := &ast.VarDecl{Base: ast.Base{P: declStart}, DeclKind: kind}
		first := &ast.VarDeclarator{Base: ast.Base{P: target.Pos()}, Target: target}
		if p.eat(lexer.Assign) {
			p.regexpHere()
			first.Init = p.parseAssign()
		}
		decl.Decls = append(decl.Decls, first)
		for p.eat(lexer.Comma) {
			dStart := p.tok.Pos
			t := p.parseBindingTarget()
			d := &ast.VarDeclarator{Base: ast.Base{P: dStart}, Target: t}
			if p.eat(lexer.Assign) {
				p.regexpHere()
				d.Init = p.parseAssign()
			}
			decl.Decls = append(decl.Decls, d)
		}
		init = decl
	} else {
		// En-tête d'expression : « in » y est suspendu, sans quoi
		// « for (a in b; ...) » serait ambigu.
		p.noIn = true
		p.regexpHere()
		forbidAsyncOf := p.tok.Kind == lexer.Ident && p.tok.Value == "async"
		expr := p.parseExpression()
		p.noIn = false

		if p.isKeyword("in") || (p.isName() && p.tok.Value == "of") {
			of := p.tok.Value == "of"
			if of && forbidAsyncOf {
				if _, ok := expr.(*ast.Ident); ok {
					p.fail("« async of » est interdit")
				}
			}
			p.advance()
			p.regexpHere()
			right := p.parseAssign()
			p.expect(lexer.RParen)
			p.regexpHere()
			return &ast.ForInStmt{Base: ast.Base{P: start}, Left: p.toPattern(expr),
				Right: right, Body: p.parseStatement(), Of: of, Await: await}
		}
		init = expr
	}

	p.expect(lexer.Semicolon)
	return p.parseForTail(start, init)
}

func (p *parser) parseForTail(start lexer.Position, init ast.Node) ast.Stmt {
	s := &ast.ForStmt{Base: ast.Base{P: start}, Init: init}
	if !p.is(lexer.Semicolon) {
		p.regexpHere()
		s.Test = p.parseExpression()
	}
	p.expect(lexer.Semicolon)
	if !p.is(lexer.RParen) {
		p.regexpHere()
		s.Update = p.parseExpression()
	}
	p.expect(lexer.RParen)
	p.regexpHere()
	s.Body = p.parseStatement()
	return s
}

func (p *parser) parseReturn() ast.Stmt {
	start := p.tok.Pos
	p.advance()
	s := &ast.ReturnStmt{Base: ast.Base{P: start}}
	if !p.is(lexer.Semicolon) && !p.is(lexer.RBrace) && !p.is(lexer.EOF) && !p.tok.NewlineBefore {
		p.regexpHere()
		s.Argument = p.parseExpression()
	}
	p.semicolon()
	return s
}

func (p *parser) parseBreakContinue() ast.Stmt {
	start := p.tok.Pos
	isBreak := p.tok.Value == "break"
	p.advance()

	var label *ast.Ident
	if p.tok.Kind == lexer.Ident && !p.tok.NewlineBefore {
		label = &ast.Ident{Base: ast.Base{P: p.tok.Pos}, Name: p.tok.Value}
		p.advance()
	}
	p.semicolon()
	if isBreak {
		return &ast.BreakStmt{Base: ast.Base{P: start}, Label: label}
	}
	return &ast.ContinueStmt{Base: ast.Base{P: start}, Label: label}
}

func (p *parser) parseThrow() ast.Stmt {
	start := p.tok.Pos
	p.advance()
	if p.tok.NewlineBefore {
		p.fail("« throw » ne peut être suivi d'un saut de ligne")
	}
	p.regexpHere()
	arg := p.parseExpression()
	p.semicolon()
	return &ast.ThrowStmt{Base: ast.Base{P: start}, Argument: arg}
}

func (p *parser) parseTry() ast.Stmt {
	start := p.tok.Pos
	p.advance()
	s := &ast.TryStmt{Base: ast.Base{P: start}, Block: p.parseBlock()}

	if p.isKeyword("catch") {
		cStart := p.tok.Pos
		p.advance()
		c := &ast.CatchClause{Base: ast.Base{P: cStart}}
		if p.eat(lexer.LParen) {
			c.Param = p.parseBindingTarget()
			p.expect(lexer.RParen)
		}
		c.Body = p.parseBlock()
		s.Handler = c
	}
	if p.eatKeyword("finally") {
		s.Finalizer = p.parseBlock()
	}
	if s.Handler == nil && s.Finalizer == nil {
		p.failAt("« try » exige « catch » ou « finally »", start)
	}
	return s
}

func (p *parser) parseSwitch() ast.Stmt {
	start := p.tok.Pos
	p.advance()
	p.expect(lexer.LParen)
	p.regexpHere()
	disc := p.parseExpression()
	p.expect(lexer.RParen)
	p.expect(lexer.LBrace)

	p.inSwitch++
	defer func() { p.inSwitch-- }()

	s := &ast.SwitchStmt{Base: ast.Base{P: start}, Discriminant: disc}
	seenDefault := false
	for !p.is(lexer.RBrace) {
		if p.is(lexer.EOF) {
			p.fail("« } » attendu pour clore le switch")
		}
		cStart := p.tok.Pos
		c := &ast.SwitchCase{Base: ast.Base{P: cStart}}
		switch {
		case p.eatKeyword("case"):
			p.regexpHere()
			c.Test = p.parseExpression()
		case p.eatKeyword("default"):
			if seenDefault {
				p.failAt("un switch n'admet qu'un seul « default »", cStart)
			}
			seenDefault = true
		default:
			p.fail("« case » ou « default » attendu")
		}
		p.expect(lexer.Colon)
		for !p.is(lexer.RBrace) && !p.isKeyword("case") && !p.isKeyword("default") {
			if p.is(lexer.EOF) {
				p.fail("« } » attendu pour clore le switch")
			}
			c.Body = append(c.Body, p.parseStatementListItem())
		}
		s.Cases = append(s.Cases, c)
	}
	p.expect(lexer.RBrace)
	return s
}

func (p *parser) parseWith() ast.Stmt {
	start := p.tok.Pos
	p.advance()
	p.expect(lexer.LParen)
	p.regexpHere()
	obj := p.parseExpression()
	p.expect(lexer.RParen)
	p.regexpHere()
	return &ast.WithStmt{Base: ast.Base{P: start}, Object: obj, Body: p.parseStatement()}
}

// ─── Modules ────────────────────────────────────────────────────────────────

func (p *parser) parseImportDecl() ast.Stmt {
	start := p.tok.Pos
	p.advance() // import

	d := &ast.ImportDecl{Base: ast.Base{P: start}}

	// import "module"
	if p.is(lexer.String) {
		d.Source = &ast.StringLit{Base: ast.Base{P: p.tok.Pos}, Raw: p.tok.Value}
		p.advance()
		p.semicolon()
		return d
	}

	if p.tok.Kind == lexer.Ident {
		local := &ast.Ident{Base: ast.Base{P: p.tok.Pos}, Name: p.tok.Value}
		d.Specifiers = append(d.Specifiers,
			&ast.ImportSpecifier{Base: ast.Base{P: p.tok.Pos}, Local: local, Kind: "default"})
		p.advance()
		p.eat(lexer.Comma)
	}

	switch {
	case p.is(lexer.Star):
		sp := p.tok.Pos
		p.advance()
		if !p.isName() || p.tok.Value != "as" {
			p.fail("« as » attendu après « * »")
		}
		p.advance()
		local := &ast.Ident{Base: ast.Base{P: p.tok.Pos}, Name: p.tok.Value}
		p.advance()
		d.Specifiers = append(d.Specifiers,
			&ast.ImportSpecifier{Base: ast.Base{P: sp}, Local: local, Kind: "namespace"})

	case p.is(lexer.LBrace):
		p.advance()
		for !p.is(lexer.RBrace) {
			sp := p.tok.Pos
			if !p.isName() && !p.is(lexer.String) {
				p.fail("nom importé attendu")
			}
			imported := &ast.Ident{Base: ast.Base{P: sp}, Name: p.tok.Value}
			p.advance()
			local := imported
			if p.isName() && p.tok.Value == "as" {
				p.advance()
				local = &ast.Ident{Base: ast.Base{P: p.tok.Pos}, Name: p.tok.Value}
				p.advance()
			}
			d.Specifiers = append(d.Specifiers, &ast.ImportSpecifier{
				Base: ast.Base{P: sp}, Imported: imported, Local: local, Kind: "named"})
			if !p.eat(lexer.Comma) {
				break
			}
		}
		p.expect(lexer.RBrace)
	}

	if !p.isName() || p.tok.Value != "from" {
		p.fail("« from » attendu")
	}
	p.advance()
	if !p.is(lexer.String) {
		p.fail("chaîne de module attendue")
	}
	d.Source = &ast.StringLit{Base: ast.Base{P: p.tok.Pos}, Raw: p.tok.Value}
	p.advance()
	p.semicolon()
	return d
}

func (p *parser) parseExportDecl() ast.Stmt {
	start := p.tok.Pos
	p.advance() // export

	d := &ast.ExportDecl{Base: ast.Base{P: start}}

	switch {
	case p.eatKeyword("default"):
		d.Default = true
		switch {
		case p.isKeyword("function"):
			fnStart := p.tok.Pos
			fn := p.parseFunctionAt(fnStart, false).(*ast.FunctionExpr)
			d.Declaration = &ast.FunctionDecl{Base: ast.Base{P: fnStart}, Fn: fn}
			// « export default function(){} » admet l'anonymat, contrairement
			// à une déclaration ordinaire.
		case p.isKeyword("class"):
			clStart := p.tok.Pos
			cl := p.parseClass(true).(*ast.ClassExpr)
			d.Declaration = &ast.ClassDecl{Base: ast.Base{P: clStart}, Class: cl}
		default:
			p.regexpHere()
			d.DefaultExpr = p.parseAssign()
			p.semicolon()
		}
		return d

	case p.is(lexer.Star):
		p.advance()
		d.Star = true
		if p.isName() && p.tok.Value == "as" {
			p.advance()
			exported := &ast.Ident{Base: ast.Base{P: p.tok.Pos}, Name: p.tok.Value}
			p.advance()
			d.Specifiers = append(d.Specifiers,
				&ast.ExportSpecifier{Base: ast.Base{P: exported.P}, Exported: exported})
		}
		if !p.isName() || p.tok.Value != "from" {
			p.fail("« from » attendu après « export * »")
		}
		p.advance()
		if !p.is(lexer.String) {
			p.fail("chaîne de module attendue")
		}
		d.Source = &ast.StringLit{Base: ast.Base{P: p.tok.Pos}, Raw: p.tok.Value}
		p.advance()
		p.semicolon()
		return d

	case p.is(lexer.LBrace):
		p.advance()
		for !p.is(lexer.RBrace) {
			sp := p.tok.Pos
			if !p.isName() {
				p.fail("nom exporté attendu")
			}
			local := &ast.Ident{Base: ast.Base{P: sp}, Name: p.tok.Value}
			p.advance()
			exported := local
			if p.isName() && p.tok.Value == "as" {
				p.advance()
				exported = &ast.Ident{Base: ast.Base{P: p.tok.Pos}, Name: p.tok.Value}
				p.advance()
			}
			d.Specifiers = append(d.Specifiers,
				&ast.ExportSpecifier{Base: ast.Base{P: sp}, Local: local, Exported: exported})
			if !p.eat(lexer.Comma) {
				break
			}
		}
		p.expect(lexer.RBrace)
		if p.isName() && p.tok.Value == "from" {
			p.advance()
			if !p.is(lexer.String) {
				p.fail("chaîne de module attendue")
			}
			d.Source = &ast.StringLit{Base: ast.Base{P: p.tok.Pos}, Raw: p.tok.Value}
			p.advance()
		}
		p.semicolon()
		return d

	default:
		d.Declaration = p.parseStatementListItem()
		return d
	}
}
