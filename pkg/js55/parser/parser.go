// Package parser construit un arbre syntaxique ECMAScript 2020 à partir d'une
// source, par descente récursive.
//
// Deux points de conception méritent d'être nommés.
//
// La distinction entre division et littéral d'expression régulière n'est pas
// décidable lexicalement. Le parser lit toujours en mode division, puis relit
// le lexème en mode expression régulière lorsqu'il constate se trouver en
// position de préfixe. Cette relecture est exacte, là où l'heuristique usuelle
// sur le lexème précédent se trompe : « if (x) /re/ » est une expression
// régulière, « (a) / b » une division, et le lexème précédent est le même.
//
// Les listes entre parenthèses sont analysées selon une grammaire de
// couverture : « (a, b) » se lit d'abord comme une liste d'expressions, puis se
// réinterprète en liste de paramètres si une flèche suit. Cela évite tout
// retour arrière et traite « (a, ...b) => c » sans cas particulier.
package parser

import (
	"fmt"
	"strings"

	"github.com/hazyhaar/js55/pkg/js55/ast"
	"github.com/hazyhaar/js55/pkg/js55/lexer"
)

// SyntaxError décrit une erreur de syntaxe et sa position.
type SyntaxError struct {
	Msg string
	Pos lexer.Position
}

func (e *SyntaxError) Error() string {
	return fmt.Sprintf("SyntaxError: %s (ligne %d, colonne %d)", e.Msg, e.Pos.Line, e.Pos.Col)
}

// Options règle le mode d'analyse.
type Options struct {
	// Module analyse la source comme un module : le mode strict y est implicite
	// et import/export y sont admis au premier niveau.
	Module bool
	// Strict force le mode strict sur une source de script.
	Strict bool
}

type parser struct {
	lx  *lexer.Lexer
	tok lexer.Token

	strict bool
	module bool

	// Contexte grammatical. yield et await ne sont des opérateurs que dans les
	// fonctions qui les admettent ; ailleurs ce sont des identifiants.
	inGenerator bool
	inAsync     bool
	inFunction  bool
	inIteration int
	inSwitch    int

	// noIn suspend l'opérateur « in » dans l'en-tête d'un for, où il
	// introduirait une ambiguïté avec la forme for-in.
	noIn bool

	// parens retient les expressions écrites entre parenthèses. L'arbre ne
	// conserve pas la parenthésation, et la règle qui interdit de mêler ?? à
	// && ou || sans parenthèses en a besoin.
	parens map[ast.Expr]bool

	// depth borne la récursion : une source profondément imbriquée doit produire
	// une erreur de syntaxe, jamais un dépassement de pile du processus.
	depth int
}

const maxDepth = 800

// Parse analyse src et rend le programme. Toute erreur est une *SyntaxError ;
// aucune entrée, valide ou non, ne provoque de panique.
func Parse(src string, opt Options) (prog *ast.Program, err error) {
	p := &parser{
		lx:     lexer.New(src),
		strict: opt.Strict || opt.Module,
		module: opt.Module,
		parens: map[ast.Expr]bool{},
	}

	defer func() {
		if r := recover(); r != nil {
			if se, ok := r.(*SyntaxError); ok {
				prog, err = nil, se
				return
			}
			panic(r)
		}
	}()

	p.advance()
	body := []ast.Stmt{}
	for p.tok.Kind != lexer.EOF {
		body = append(body, p.parseStatementListItem())
	}
	return &ast.Program{
		Base:   ast.Base{P: lexer.Position{Line: 1, Col: 1}},
		Body:   body,
		Module: opt.Module,
	}, nil
}

// ─── Machinerie ─────────────────────────────────────────────────────────────

func (p *parser) fail(msg string) {
	panic(&SyntaxError{Msg: msg, Pos: p.tok.Pos})
}

func (p *parser) failAt(msg string, pos lexer.Position) {
	panic(&SyntaxError{Msg: msg, Pos: pos})
}

func (p *parser) enter() {
	p.depth++
	if p.depth > maxDepth {
		p.fail("imbrication trop profonde")
	}
}

func (p *parser) leave() { p.depth-- }

// advance lit le lexème suivant, en préférant la division. La relecture en mode
// expression régulière se fait par regexpHere, au moment où le parser sait être
// en position de préfixe.
func (p *parser) advance() {
	p.tok = p.lx.Next(false)
	if p.tok.Kind == lexer.Illegal {
		p.fail(p.tok.Message)
	}
}

// regexpHere relit le lexème courant comme un littéral d'expression régulière.
// Appelé uniquement en position de préfixe.
func (p *parser) regexpHere() {
	if p.tok.Kind != lexer.Slash && p.tok.Kind != lexer.SlashAssign {
		return
	}
	nl := p.tok.NewlineBefore
	p.lx.SeekTo(p.tok.Pos)
	p.tok = p.lx.Next(true)
	p.tok.NewlineBefore = nl
	if p.tok.Kind == lexer.Illegal {
		p.fail(p.tok.Message)
	}
}

func (p *parser) is(k lexer.Kind) bool { return p.tok.Kind == k }

func (p *parser) isKeyword(w string) bool {
	return p.tok.Kind == lexer.Keyword && p.tok.Value == w
}

// isName accepte un identifiant ou un mot réservé employé comme nom de
// propriété, ce que la grammaire autorise après un point.
func (p *parser) isName() bool {
	return p.tok.Kind == lexer.Ident || p.tok.Kind == lexer.Keyword
}

func identName(v string) string {
	if !strings.Contains(v, `\`) {
		return v
	}
	var b strings.Builder
	b.Grow(len(v))
	for i := 0; i < len(v); i++ {
		if v[i] != '\\' || i+1 >= len(v) || v[i+1] != 'u' {
			b.WriteByte(v[i])
			continue
		}
		if i+2 < len(v) && v[i+2] == '{' {
			j := i + 3
			cp := 0
			for j < len(v) && v[j] != '}' {
				h := v[j]
				switch {
				case h >= '0' && h <= '9':
					cp = cp*16 + int(h-'0')
				case h >= 'a' && h <= 'f':
					cp = cp*16 + int(h-'a') + 10
				case h >= 'A' && h <= 'F':
					cp = cp*16 + int(h-'A') + 10
				}
				j++
			}
			b.WriteRune(rune(cp))
			i = j
			continue
		}
		if i+6 <= len(v) {
			cp := 0
			for _, h := range v[i+2 : i+6] {
				switch {
				case h >= '0' && h <= '9':
					cp = cp*16 + int(h-'0')
				case h >= 'a' && h <= 'f':
					cp = cp*16 + int(h-'a') + 10
				case h >= 'A' && h <= 'F':
					cp = cp*16 + int(h-'A') + 10
				}
			}
			b.WriteRune(rune(cp))
			i += 5
			continue
		}
		b.WriteByte(v[i])
	}
	return b.String()
}

func (p *parser) eat(k lexer.Kind) bool {
	if p.tok.Kind == k {
		p.advance()
		return true
	}
	return false
}

func (p *parser) eatKeyword(w string) bool {
	if p.isKeyword(w) {
		p.advance()
		return true
	}
	return false
}

func (p *parser) expect(k lexer.Kind) lexer.Token {
	t := p.tok
	if t.Kind != k {
		p.fail(fmt.Sprintf("%s attendu, %s trouvé", k, t.Kind))
	}
	p.advance()
	return t
}

// semicolon applique l'insertion automatique de points-virgules : un
// point-virgule explicite, ou bien une accolade fermante, la fin de source, ou
// un terminateur de ligne franchi.
func (p *parser) semicolon() {
	if p.eat(lexer.Semicolon) {
		return
	}
	if p.is(lexer.RBrace) || p.is(lexer.EOF) || p.tok.NewlineBefore {
		return
	}
	p.fail(fmt.Sprintf("point-virgule attendu, %s trouvé", p.tok.Kind))
}

// ─── Expressions ────────────────────────────────────────────────────────────

// parseExpression lit une expression complète, virgules comprises.
func (p *parser) parseExpression() ast.Expr {
	first := p.parseAssign()
	if !p.is(lexer.Comma) {
		return first
	}
	seq := []ast.Expr{first}
	for p.eat(lexer.Comma) {
		seq = append(seq, p.parseAssign())
	}
	return &ast.SequenceExpr{Base: ast.Base{P: first.Pos()}, Exprs: seq}
}

var assignOps = map[lexer.Kind]string{
	lexer.Assign: "=", lexer.PlusAssign: "+=", lexer.MinusAssign: "-=",
	lexer.StarAssign: "*=", lexer.SlashAssign: "/=", lexer.PercentAssign: "%=",
	lexer.StarStarAssign: "**=", lexer.ShlAssign: "<<=", lexer.ShrAssign: ">>=",
	lexer.UShrAssign: ">>>=", lexer.AndAssign: "&=", lexer.OrAssign: "|=",
	lexer.XorAssign: "^=", lexer.AndAndAssign: "&&=", lexer.OrOrAssign: "||=",
	lexer.QuestionQuestionAssign: "??=",
}

func (p *parser) parseAssign() ast.Expr {
	p.enter()
	defer p.leave()

	if p.inGenerator && p.isKeyword("yield") {
		return p.parseYield()
	}

	start := p.tok.Pos

	// Flèche à paramètre unique sans parenthèses : x => ...
	if p.tok.Kind == lexer.Ident {
		save := p.lx.Save()
		name := p.tok
		p.advance()
		if p.is(lexer.Arrow) && !p.tok.NewlineBefore {
			p.advance()
			id := &ast.Ident{Base: ast.Base{P: name.Pos}, Name: name.Value}
			return p.parseArrowBody([]ast.Expr{id}, start, false)
		}
		p.lx.Restore(save)
		p.tok = name
	}

	// async x => ... et async (a, b) => ...
	if p.isKeyword("async") || (p.tok.Kind == lexer.Ident && p.tok.Value == "async") {
		if arrow := p.tryAsyncArrow(); arrow != nil {
			return arrow
		}
	}

	left := p.parseConditional()

	if op, ok := assignOps[p.tok.Kind]; ok {
		p.advance()
		p.regexpHere()
		target := left
		if op == "=" {
			target = p.toPattern(left)
			// Une valeur par défaut n'est licite qu'À L'INTÉRIEUR d'un motif de
			// décomposition, jamais comme cible d'une affectation : « (a=0)=1 »
			// est une erreur. Sans ce contrôle, toPattern acceptait la forme et
			// produisait un arbre irréimprimable. Défaut trouvé par le fuzz.
			switch target.(type) {
			case *ast.AssignPattern, *ast.RestElement:
				p.failAt("cible d'affectation invalide", left.Pos())
			}
		} else if !isSimpleTarget(left) {
			p.failAt("cible d'affectation invalide", left.Pos())
		}
		right := p.parseAssign()
		return &ast.AssignExpr{Base: ast.Base{P: start}, Op: op, Target: target, Value: right}
	}
	return left
}

func (p *parser) parseYield() ast.Expr {
	start := p.tok.Pos
	p.advance()
	y := &ast.YieldExpr{Base: ast.Base{P: start}}
	if p.tok.NewlineBefore {
		return y
	}
	if p.eat(lexer.Star) {
		y.Delegate = true
		p.regexpHere()
		y.Argument = p.parseAssign()
		return y
	}
	if p.startsExpression() {
		p.regexpHere()
		y.Argument = p.parseAssign()
	}
	return y
}

// tryAsyncArrow reconnaît une fonction fléchée asynchrone. Rend nil sans
// consommer si le motif ne s'applique pas.
func (p *parser) tryAsyncArrow() ast.Expr {
	save := p.lx.Save()
	asyncTok := p.tok
	start := asyncTok.Pos
	p.advance()

	if p.tok.NewlineBefore {
		p.lx.Restore(save)
		p.tok = asyncTok
		return nil
	}

	if p.tok.Kind == lexer.Ident {
		nameTok := p.tok
		inner := p.lx.Save()
		p.advance()
		if p.is(lexer.Arrow) && !p.tok.NewlineBefore {
			p.advance()
			id := &ast.Ident{Base: ast.Base{P: nameTok.Pos}, Name: nameTok.Value}
			return p.parseArrowBody([]ast.Expr{id}, start, true)
		}
		p.lx.Restore(inner)
		p.tok = nameTok
	}

	// async ( ... ) => ... . La liste se lit selon la grammaire de couverture ;
	// sans flèche derrière, « async(x) » reste un appel et l'on revient en
	// arrière. La lecture spéculative ne peut pas échouer bruyamment : elle est
	// isolée par recover pour ne pas transformer un appel licite en erreur.
	if p.is(lexer.LParen) {
		if items, ok := p.trySpeculativeParenList(); ok {
			if p.is(lexer.Arrow) && !p.tok.NewlineBefore {
				p.advance()
				return p.parseArrowBody(items, start, true)
			}
		}
	}

	p.lx.Restore(save)
	p.tok = asyncTok
	return nil
}

// trySpeculativeParenList lit une liste entre parenthèses en spéculation. Rend
// ok à faux si la lecture échoue, sans propager l'erreur : l'appelant reviendra
// en arrière et réanalysera la même source autrement.
func (p *parser) trySpeculativeParenList() (items []ast.Expr, ok bool) {
	savedDepth := p.depth
	defer func() {
		if r := recover(); r != nil {
			if _, isSyntax := r.(*SyntaxError); isSyntax {
				p.depth = savedDepth
				items, ok = nil, false
				return
			}
			panic(r)
		}
	}()

	p.expect(lexer.LParen)
	out := []ast.Expr{}
	for !p.is(lexer.RParen) {
		if p.is(lexer.Ellipsis) {
			sp := p.tok.Pos
			p.advance()
			p.regexpHere()
			out = append(out, &ast.RestElement{Base: ast.Base{P: sp}, Argument: p.parseAssign()})
			break
		}
		p.regexpHere()
		out = append(out, p.parseAssign())
		if !p.eat(lexer.Comma) {
			break
		}
	}
	p.expect(lexer.RParen)
	return out, true
}

func (p *parser) parseArrowBody(params []ast.Expr, start lexer.Position, async bool) ast.Expr {
	for i, prm := range params {
		params[i] = p.toPattern(prm)
	}
	fn := &ast.ArrowFunction{Base: ast.Base{P: start}, Params: params, Async: async}

	savedAsync, savedGen, savedFn := p.inAsync, p.inGenerator, p.inFunction
	p.inAsync, p.inGenerator, p.inFunction = async, false, true
	defer func() { p.inAsync, p.inGenerator, p.inFunction = savedAsync, savedGen, savedFn }()

	if p.is(lexer.LBrace) {
		fn.Body = p.parseBlock()
	} else {
		p.regexpHere()
		fn.Body = p.parseAssign()
	}
	return fn
}

func (p *parser) parseConditional() ast.Expr {
	start := p.tok.Pos
	test := p.parseBinary(0)
	if !p.is(lexer.Question) {
		return test
	}
	p.advance()
	p.regexpHere()
	cons := p.parseAssign()
	p.expect(lexer.Colon)
	p.regexpHere()
	alt := p.parseAssign()
	return &ast.ConditionalExpr{Base: ast.Base{P: start}, Test: test, Consequent: cons, Alternate: alt}
}

type binOp struct {
	text  string
	prec  int
	logic bool
}

var binOps = map[lexer.Kind]binOp{
	lexer.QuestionQuestion: {"??", 1, true},
	lexer.OrOr:             {"||", 2, true},
	lexer.AndAnd:           {"&&", 3, true},
	lexer.Or:               {"|", 4, false},
	lexer.Xor:              {"^", 5, false},
	lexer.And:              {"&", 6, false},
	lexer.EqEq:             {"==", 7, false},
	lexer.NotEq:            {"!=", 7, false},
	lexer.EqEqEq:           {"===", 7, false},
	lexer.NotEqEq:          {"!==", 7, false},
	lexer.Lt:               {"<", 8, false},
	lexer.Gt:               {">", 8, false},
	lexer.Le:               {"<=", 8, false},
	lexer.Ge:               {">=", 8, false},
	lexer.Shl:              {"<<", 9, false},
	lexer.Shr:              {">>", 9, false},
	lexer.UShr:             {">>>", 9, false},
	lexer.Plus:             {"+", 10, false},
	lexer.Minus:            {"-", 10, false},
	lexer.Star:             {"*", 11, false},
	lexer.Slash:            {"/", 11, false},
	lexer.Percent:          {"%", 11, false},
	lexer.StarStar:         {"**", 12, false},
}

// parseBinary applique la montée en précédence. noIn suspend l'opérateur « in »
// dans l'en-tête d'un for, où il introduirait une ambiguïté ; il est porté par
// le champ noIn du parser pour ne pas alourdir la signature.
func (p *parser) parseBinary(minPrec int) ast.Expr {
	p.enter()
	defer p.leave()

	left := p.parseUnary()

	for {
		var op binOp
		var ok bool

		if p.tok.Kind == lexer.Keyword {
			switch p.tok.Value {
			case "instanceof":
				op, ok = binOp{"instanceof", 8, false}, true
			case "in":
				if p.noIn {
					ok = false
				} else {
					op, ok = binOp{"in", 8, false}, true
				}
			}
		}
		if !ok {
			op, ok = binOps[p.tok.Kind]
		}
		if !ok || op.prec < minPrec {
			return left
		}

		// ?? ne se mêle pas à && ni || sans parenthèses, dans un sens comme
		// dans l'autre. Une expression parenthésée échappe à la règle.
		if p.mixesNullish(op.text, left) {
			p.fail("?? ne peut se mêler à && ou || sans parenthèses")
		}

		if op.text == "**" {
			p.checkExponentLeft(left)
		}

		start := left.Pos()
		p.advance()
		p.regexpHere()

		// ** est associatif à droite ; les autres à gauche.
		next := op.prec + 1
		if op.text == "**" {
			next = op.prec
		}
		right := p.parseBinary(next)

		if p.mixesNullish(op.text, right) {
			p.failAt("?? ne peut se mêler à && ou || sans parenthèses", right.Pos())
		}

		if op.logic {
			left = &ast.LogicalExpr{Base: ast.Base{P: start}, Op: op.text, Left: left, Right: right}
		} else {
			left = &ast.BinaryExpr{Base: ast.Base{P: start}, Op: op.text, Left: left, Right: right}
		}
	}
}

// mixesNullish indique que l'opérande viole l'interdiction de mêler ?? à && ou
// || sans parenthèses. Une expression parenthésée est toujours admise.
func (p *parser) mixesNullish(op string, operand ast.Expr) bool {
	l, ok := operand.(*ast.LogicalExpr)
	if !ok || p.parens[operand] {
		return false
	}
	if op == "??" {
		return l.Op == "&&" || l.Op == "||"
	}
	if op == "&&" || op == "||" {
		return l.Op == "??"
	}
	return false
}

var unaryOps = map[lexer.Kind]string{
	lexer.Not: "!", lexer.Tilde: "~", lexer.Plus: "+", lexer.Minus: "-",
}

func (p *parser) parseUnary() ast.Expr {
	p.enter()
	defer p.leave()

	start := p.tok.Pos

	if op, ok := unaryOps[p.tok.Kind]; ok {
		p.advance()
		p.regexpHere()
		return &ast.UnaryExpr{Base: ast.Base{P: start}, Op: op, Operand: p.parseUnary()}
	}
	if p.tok.Kind == lexer.Keyword {
		switch p.tok.Value {
		case "typeof", "void", "delete":
			op := p.tok.Value
			p.advance()
			p.regexpHere()
			return &ast.UnaryExpr{Base: ast.Base{P: start}, Op: op, Operand: p.parseUnary()}
		case "await":
			if p.inAsync || (p.module && !p.inFunction) {
				p.advance()
				p.regexpHere()
				return &ast.AwaitExpr{Base: ast.Base{P: start}, Argument: p.parseUnary()}
			}
		}
	}
	if p.is(lexer.PlusPlus) || p.is(lexer.MinusMinus) {
		op := p.tok.Value
		p.advance()
		p.regexpHere()
		operand := p.parseUnary()
		if !isSimpleTarget(operand) {
			p.failAt("cible d'incrémentation invalide", operand.Pos())
		}
		return &ast.UpdateExpr{Base: ast.Base{P: start}, Op: op, Prefix: true, Operand: operand}
	}

	return p.parsePostfix()
}

// checkExponentLeft applique la restriction de la grammaire : l'opérande gauche
// de ** doit être une expression de mise à jour, ce qui exclut un unaire non
// parenthésé. « -a ** b » est invalide, « (-a) ** b » et « a ** -b » sont licites.
func (p *parser) checkExponentLeft(left ast.Expr) {
	if p.parens[left] {
		return
	}
	switch left.(type) {
	case *ast.UnaryExpr, *ast.AwaitExpr:
		p.failAt("un opérande unaire non parenthésé ne peut précéder **", left.Pos())
	}
}

func (p *parser) parsePostfix() ast.Expr {
	expr := p.parseCallOrMember(true)
	if (p.is(lexer.PlusPlus) || p.is(lexer.MinusMinus)) && !p.tok.NewlineBefore {
		if !isSimpleTarget(expr) {
			p.failAt("cible d'incrémentation invalide", expr.Pos())
		}
		op := p.tok.Value
		start := expr.Pos()
		p.advance()
		return &ast.UpdateExpr{Base: ast.Base{P: start}, Op: op, Operand: expr}
	}
	return expr
}

// parseCallOrMember lit un accès, un appel, un gabarit étiqueté ou une chaîne
// optionnelle. allowCall vaut faux dans l'argument de new, où « ( » ouvre la
// liste d'arguments du new et non un appel du callee.
func (p *parser) parseCallOrMember(allowCall bool) ast.Expr {
	p.enter()
	defer p.leave()

	var expr ast.Expr
	if p.isKeyword("new") {
		expr = p.parseNew()
	} else {
		expr = p.parsePrimary()
	}

	optionalSeen := false
	for {
		switch {
		case p.is(lexer.Dot):
			p.advance()
			if !p.isName() && p.tok.Kind != lexer.PrivateIdent {
				p.fail("nom de propriété attendu après « . »")
			}
			prop := p.propertyNameNode()
			expr = &ast.MemberExpr{Base: ast.Base{P: expr.Pos()}, Object: expr, Property: prop}

		case p.is(lexer.QuestionDot):
			optionalSeen = true
			p.advance()
			switch {
			case p.is(lexer.LParen) && allowCall:
				args := p.parseArguments()
				expr = &ast.CallExpr{Base: ast.Base{P: expr.Pos()}, Callee: expr, Args: args, Optional: true}
			case p.is(lexer.LBracket):
				p.advance()
				p.regexpHere()
				idx := p.parseExpression()
				p.expect(lexer.RBracket)
				expr = &ast.MemberExpr{Base: ast.Base{P: expr.Pos()}, Object: expr,
					Property: idx, Computed: true, Optional: true}
			default:
				if !p.isName() && p.tok.Kind != lexer.PrivateIdent {
					p.fail("nom de propriété attendu après « ?. »")
				}
				prop := p.propertyNameNode()
				expr = &ast.MemberExpr{Base: ast.Base{P: expr.Pos()}, Object: expr,
					Property: prop, Optional: true}
			}

		case p.is(lexer.LBracket):
			p.advance()
			p.regexpHere()
			idx := p.parseExpression()
			p.expect(lexer.RBracket)
			expr = &ast.MemberExpr{Base: ast.Base{P: expr.Pos()}, Object: expr,
				Property: idx, Computed: true}

		case p.is(lexer.LParen) && allowCall:
			args := p.parseArguments()
			expr = &ast.CallExpr{Base: ast.Base{P: expr.Pos()}, Callee: expr, Args: args}

		case p.is(lexer.NoSubTemplate) || p.is(lexer.TemplateHead):
			quasi := p.parseTemplate()
			expr = &ast.TaggedTemplate{Base: ast.Base{P: expr.Pos()}, Tag: expr, Quasi: quasi}

		default:
			if optionalSeen {
				return &ast.ChainExpr{Base: ast.Base{P: expr.Pos()}, Expression: expr}
			}
			return expr
		}
	}
}

func (p *parser) propertyNameNode() ast.Expr {
	t := p.tok
	p.advance()
	if t.Kind == lexer.PrivateIdent {
		return &ast.PrivateName{Base: ast.Base{P: t.Pos}, Name: t.Value}
	}
	return &ast.Ident{Base: ast.Base{P: t.Pos}, Name: t.Value}
}

func (p *parser) parseNew() ast.Expr {
	start := p.tok.Pos
	p.advance() // new

	if p.is(lexer.Dot) {
		p.advance()
		if !p.isName() || p.tok.Value != "target" {
			p.fail("« new.target » attendu")
		}
		p.advance()
		return &ast.MetaProperty{Base: ast.Base{P: start}, Meta: "new", Property: "target"}
	}

	callee := p.parseCallOrMember(false)
	n := &ast.NewExpr{Base: ast.Base{P: start}, Callee: callee}
	if p.is(lexer.LParen) {
		n.Args = p.parseArguments()
	}
	return n
}

func (p *parser) parseArguments() []ast.Expr {
	p.expect(lexer.LParen)
	args := []ast.Expr{}
	p.regexpHere()
	for !p.is(lexer.RParen) {
		if p.is(lexer.Ellipsis) {
			start := p.tok.Pos
			p.advance()
			p.regexpHere()
			args = append(args, &ast.SpreadElement{Base: ast.Base{P: start}, Argument: p.parseAssign()})
		} else {
			args = append(args, p.parseAssign())
		}
		if !p.eat(lexer.Comma) {
			break
		}
		p.regexpHere()
	}
	p.expect(lexer.RParen)
	return args
}

// startsExpression indique si le lexème courant peut ouvrir une expression.
// Sert à yield et return, dont l'argument est facultatif.
func (p *parser) startsExpression() bool {
	switch p.tok.Kind {
	case lexer.EOF, lexer.Semicolon, lexer.RBrace, lexer.RParen, lexer.RBracket,
		lexer.Comma, lexer.Colon:
		return false
	}
	return true
}
