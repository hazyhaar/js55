package parser

import "github.com/hazyhaar/js55/pkg/js55/ast"

// isSimpleTarget indique si e peut être la cible d'une affectation composée ou
// d'une incrémentation : un nom ou un accès de propriété, à l'exclusion des
// motifs de décomposition.
func isSimpleTarget(e ast.Expr) bool {
	switch v := e.(type) {
	case *ast.Ident:
		return true
	case *ast.MemberExpr:
		return true
	case *ast.ChainExpr:
		// « a?.b = 1 » est invalide : une chaîne optionnelle n'est pas une cible.
		_ = v
		return false
	}
	return false
}

// toPattern réinterprète une expression en motif de liaison. C'est la seconde
// moitié de la grammaire de couverture : « {a, b} » se lit d'abord comme un
// littéral objet, puis se convertit lorsqu'une flèche ou un signe d'égalité
// révèle qu'il s'agissait d'un motif.
//
// La conversion est totale : tout nœud qui ne peut pas devenir un motif produit
// une erreur de syntaxe située, jamais un arbre approximatif.
func (p *parser) toPattern(e ast.Expr) ast.Expr {
	switch v := e.(type) {

	case *ast.Ident:
		if p.strict && (v.Name == "arguments" || v.Name == "eval" || v.Name == "yield") {
			p.failAt("cible d'affectation invalide en mode strict", v.Pos())
		}
		if p.inGenerator && v.Name == "yield" {
			p.failAt("cible d'affectation invalide dans un générateur", v.Pos())
		}
		return e
	case *ast.MemberExpr, *ast.ArrayPattern, *ast.ObjectPattern,
		*ast.RestElement:
		return e
	case *ast.AssignPattern:
		return &ast.AssignPattern{Base: v.Base, Target: p.toPattern(v.Target), Default: v.Default}

	case *ast.ArrayLit:
		if v.RestTrailingComma {
			p.failAt("virgule après l'élément de reste", v.Pos())
		}
		out := &ast.ArrayPattern{Base: v.Base}
		for i, el := range v.Elements {
			if el == nil {
				out.Elements = append(out.Elements, nil)
				continue
			}
			if sp, ok := el.(*ast.SpreadElement); ok {
				if i != len(v.Elements)-1 {
					p.failAt("l'élément de reste doit être le dernier", sp.Pos())
				}
				arg := p.toPattern(sp.Argument)
				if _, ok := arg.(*ast.AssignPattern); ok {
					p.failAt("le reste n'admet pas d'initialiseur", sp.Pos())
				}
				out.Elements = append(out.Elements,
					&ast.RestElement{Base: sp.Base, Argument: arg})
				continue
			}
			out.Elements = append(out.Elements, p.toPattern(el))
		}
		return out

	case *ast.ObjectLit:
		if v.RestTrailingComma {
			p.failAt("virgule après l'élément de reste", v.Pos())
		}
		out := &ast.ObjectPattern{Base: v.Base}
		for i, prop := range v.Properties {
			if prop.Kind == "spread" {
				if i != len(v.Properties)-1 {
					p.failAt("l'élément de reste doit être le dernier", prop.Pos())
				}
				out.Properties = append(out.Properties, &ast.Property{
					Base: prop.Base, Kind: "rest",
					Value: p.toPattern(prop.Value),
				})
				continue
			}
			if prop.Method || prop.Kind == "get" || prop.Kind == "set" {
				p.failAt("une méthode ne peut figurer dans un motif de décomposition", prop.Pos())
			}
			np := *prop
			np.Value = p.toPattern(prop.Value)
			out.Properties = append(out.Properties, &np)
		}
		return out

	case *ast.AssignExpr:
		if v.Op != "=" {
			p.failAt("seule l'affectation simple admet une valeur par défaut dans un motif", v.Pos())
		}
		return &ast.AssignPattern{Base: v.Base, Target: p.toPattern(v.Target), Default: v.Value}

	case *ast.SpreadElement:
		return &ast.RestElement{Base: v.Base, Argument: p.toPattern(v.Argument)}
	}

	p.failAt("cible d'affectation invalide", e.Pos())
	return nil
}

func checkForDeclEarlyErrors(p *parser, target ast.Expr, body ast.Stmt) {
	seen := map[string]int{}
	if dup := boundNameDup(target, seen); dup != "" {
		p.fail("nom lié en double : " + dup)
	}
	if seen["let"] > 0 {
		p.fail("« let » n'est pas un nom lié licite")
	}
	vars := map[string]bool{}
	collectVarDeclaredNames(body, vars)
	for n := range seen {
		if vars[n] {
			p.fail("redeclaration de « " + n + " »")
		}
	}
}

func collectVarDeclaredNames(st ast.Stmt, names map[string]bool) {
	if st == nil {
		return
	}
	switch s := st.(type) {
	case *ast.VarDecl:
		if s == nil || s.DeclKind != "var" {
			return
		}
		for _, d := range s.Decls {
			seen := map[string]int{}
			boundNameDup(d.Target, seen)
			for n := range seen {
				names[n] = true
			}
		}
	case *ast.BlockStmt:
		if s == nil {
			return
		}
		for _, in := range s.Body {
			collectVarDeclaredNames(in, names)
		}
	case *ast.IfStmt:
		if s == nil {
			return
		}
		collectVarDeclaredNames(s.Consequent, names)
		collectVarDeclaredNames(s.Alternate, names)
	case *ast.WhileStmt:
		if s == nil {
			return
		}
		collectVarDeclaredNames(s.Body, names)
	case *ast.DoWhileStmt:
		if s == nil {
			return
		}
		collectVarDeclaredNames(s.Body, names)
	case *ast.ForStmt:
		if s == nil {
			return
		}
		if vd, ok := s.Init.(*ast.VarDecl); ok {
			collectVarDeclaredNames(vd, names)
		}
		collectVarDeclaredNames(s.Body, names)
	case *ast.ForInStmt:
		if s == nil {
			return
		}
		if vd, ok := s.Left.(*ast.VarDecl); ok {
			collectVarDeclaredNames(vd, names)
		}
		collectVarDeclaredNames(s.Body, names)
	case *ast.TryStmt:
		if s == nil {
			return
		}
		if s.Block != nil {
			collectVarDeclaredNames(s.Block, names)
		}
		if s.Handler != nil && s.Handler.Body != nil {
			collectVarDeclaredNames(s.Handler.Body, names)
		}
		if s.Finalizer != nil {
			collectVarDeclaredNames(s.Finalizer, names)
		}
	case *ast.LabeledStmt:
		if s == nil {
			return
		}
		collectVarDeclaredNames(s.Body, names)
	case *ast.WithStmt:
		if s == nil {
			return
		}
		collectVarDeclaredNames(s.Body, names)
	case *ast.SwitchStmt:
		if s == nil {
			return
		}
		for _, c := range s.Cases {
			if c == nil {
				continue
			}
			for _, in := range c.Body {
				collectVarDeclaredNames(in, names)
			}
		}
	}
}

func boundNameDup(e ast.Expr, seen map[string]int) string {
	switch v := e.(type) {
	case *ast.Ident:
		if seen[v.Name] > 0 {
			return v.Name
		}
		seen[v.Name]++
	case *ast.ArrayPattern:
		for _, el := range v.Elements {
			if el == nil {
				continue
			}
			if d := boundNameDup(el, seen); d != "" {
				return d
			}
		}
	case *ast.ObjectPattern:
		for _, p := range v.Properties {
			if p == nil {
				continue
			}
			if d := boundNameDup(p.Value, seen); d != "" {
				return d
			}
		}
	case *ast.AssignPattern:
		return boundNameDup(v.Target, seen)
	case *ast.RestElement:
		return boundNameDup(v.Argument, seen)
	}
	return ""
}
