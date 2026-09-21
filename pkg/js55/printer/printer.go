// Package printer réimprime un arbre syntaxique en source ECMAScript.
//
// Son objet n'est pas l'élégance de la sortie mais la FIDÉLITÉ : réanalyser ce
// qu'il produit doit rendre un arbre structurellement identique à l'original
// (plan T1.2). C'est l'instrument qui attrape les erreurs de précédence et
// d'associativité, que la suite de conformité laisse largement passer parce
// qu'elle observe des résultats, pas des arbres.
//
// Le parti pris est donc de parenthéser généreusement : une parenthèse
// superflue ne change pas l'arbre relu, une parenthèse manquante le change.
package printer

import (
	"strconv"
	"strings"

	"github.com/hazyhaar/js55/pkg/js55/ast"
)

// Print réimprime n. La sortie est réanalysable.
func Print(n ast.Node) string {
	var p printer
	p.node(n)
	return p.b.String()
}

type printer struct{ b strings.Builder }

func (p *printer) s(x string) { p.b.WriteString(x) }

func (p *printer) list(items []ast.Expr, sep string) {
	for i, it := range items {
		if i > 0 {
			p.s(sep)
		}
		if it == nil { // case élidée d'un littéral de tableau
			continue
		}
		p.node(it)
	}
}

func (p *printer) stmts(items []ast.Stmt) {
	for _, st := range items {
		p.node(st)
		p.s("\n")
	}
}

func (p *printer) node(n ast.Node) {
	switch v := n.(type) {

	case nil:
		return

	// ─── Programme et instructions ──────────────────────────────────────────

	case *ast.Program:
		p.stmts(v.Body)

	case *ast.BlockStmt:
		p.s("{\n")
		p.stmts(v.Body)
		p.s("}")

	case *ast.ExpressionStmt:
		// Une parenthèse évite qu'une expression commençant par « { » ou
		// « function » soit relue comme un bloc ou une déclaration.
		p.s("(")
		p.node(v.Expression)
		p.s(");")

	case *ast.EmptyStmt:
		p.s(";")

	case *ast.DebuggerStmt:
		p.s("debugger;")

	case *ast.VarDecl:
		p.varDecl(v)
		p.s(";")

	case *ast.VarDeclarator:
		p.node(v.Target)
		if v.Init != nil {
			p.s("=")
			p.node(v.Init)
		}

	case *ast.FunctionDecl:
		p.function(v.Fn, true)

	case *ast.ClassDecl:
		p.class(v.Class, true)

	case *ast.ReturnStmt:
		p.s("return")
		if v.Argument != nil {
			p.s(" ")
			p.node(v.Argument)
		}
		p.s(";")

	case *ast.IfStmt:
		p.s("if(")
		p.node(v.Test)
		p.s(")")
		p.node(v.Consequent)
		if v.Alternate != nil {
			p.s("\nelse ")
			p.node(v.Alternate)
		}

	case *ast.ForStmt:
		p.s("for(")
		if v.Init != nil {
			if d, ok := v.Init.(*ast.VarDecl); ok {
				p.varDecl(d)
			} else {
				p.node(v.Init)
			}
		}
		p.s(";")
		p.node(v.Test)
		p.s(";")
		p.node(v.Update)
		p.s(")")
		p.node(v.Body)

	case *ast.ForInStmt:
		p.s("for")
		if v.Await {
			p.s(" await")
		}
		p.s("(")
		if d, ok := v.Left.(*ast.VarDecl); ok {
			p.varDecl(d)
		} else {
			p.node(v.Left)
		}
		if v.Of {
			p.s(" of ")
		} else {
			p.s(" in ")
		}
		p.node(v.Right)
		p.s(")")
		p.node(v.Body)

	case *ast.WhileStmt:
		p.s("while(")
		p.node(v.Test)
		p.s(")")
		p.node(v.Body)

	case *ast.DoWhileStmt:
		p.s("do ")
		p.node(v.Body)
		p.s("while(")
		p.node(v.Test)
		p.s(");")

	case *ast.BreakStmt:
		p.s("break")
		if v.Label != nil {
			p.s(" " + v.Label.Name)
		}
		p.s(";")

	case *ast.ContinueStmt:
		p.s("continue")
		if v.Label != nil {
			p.s(" " + v.Label.Name)
		}
		p.s(";")

	case *ast.LabeledStmt:
		p.s(v.Label.Name + ":")
		p.node(v.Body)

	case *ast.ThrowStmt:
		p.s("throw ")
		p.node(v.Argument)
		p.s(";")

	case *ast.TryStmt:
		p.s("try")
		p.node(v.Block)
		if v.Handler != nil {
			p.s("catch")
			if v.Handler.Param != nil {
				p.s("(")
				p.node(v.Handler.Param)
				p.s(")")
			}
			p.node(v.Handler.Body)
		}
		if v.Finalizer != nil {
			p.s("finally")
			p.node(v.Finalizer)
		}

	case *ast.SwitchStmt:
		p.s("switch(")
		p.node(v.Discriminant)
		p.s("){\n")
		for _, c := range v.Cases {
			if c.Test != nil {
				p.s("case ")
				p.node(c.Test)
				p.s(":\n")
			} else {
				p.s("default:\n")
			}
			p.stmts(c.Body)
		}
		p.s("}")

	case *ast.WithStmt:
		p.s("with(")
		p.node(v.Object)
		p.s(")")
		p.node(v.Body)

	// ─── Modules ────────────────────────────────────────────────────────────

	case *ast.ImportDecl:
		p.s("import")
		var dflt, ns string
		var named []string
		for _, sp := range v.Specifiers {
			switch sp.Kind {
			case "default":
				dflt = sp.Local.Name
			case "namespace":
				ns = sp.Local.Name
			default:
				if sp.Imported.Name == sp.Local.Name {
					named = append(named, sp.Local.Name)
				} else {
					named = append(named, sp.Imported.Name+" as "+sp.Local.Name)
				}
			}
		}
		parts := []string{}
		if dflt != "" {
			parts = append(parts, dflt)
		}
		if ns != "" {
			parts = append(parts, "* as "+ns)
		}
		if len(named) > 0 {
			parts = append(parts, "{"+strings.Join(named, ",")+"}")
		}
		if len(parts) > 0 {
			p.s(" " + strings.Join(parts, ",") + " from")
		}
		p.s(" " + v.Source.Raw + ";")

	case *ast.ExportDecl:
		p.s("export")
		switch {
		case v.Default:
			p.s(" default ")
			if v.Declaration != nil {
				p.node(v.Declaration)
			} else {
				p.node(v.DefaultExpr)
				p.s(";")
			}
		case v.Star:
			p.s(" *")
			for _, sp := range v.Specifiers {
				if sp.Exported != nil {
					p.s(" as " + sp.Exported.Name)
				}
			}
			p.s(" from " + v.Source.Raw + ";")
		case v.Declaration != nil:
			p.s(" ")
			p.node(v.Declaration)
		default:
			names := make([]string, 0, len(v.Specifiers))
			for _, sp := range v.Specifiers {
				if sp.Local.Name == sp.Exported.Name {
					names = append(names, sp.Local.Name)
				} else {
					names = append(names, sp.Local.Name+" as "+sp.Exported.Name)
				}
			}
			p.s("{" + strings.Join(names, ",") + "}")
			if v.Source != nil {
				p.s(" from " + v.Source.Raw)
			}
			p.s(";")
		}

	// ─── Expressions ────────────────────────────────────────────────────────

	case *ast.Ident:
		p.s(v.Name)
	case *ast.PrivateName:
		p.s(v.Name)
	case *ast.NumberLit:
		p.s(v.Raw)
	case *ast.BigIntLit:
		p.s(v.Raw)
	case *ast.StringLit:
		p.s(v.Raw)
	case *ast.RegExpLit:
		// Parenthéser systématiquement : « 0 / /0/ » est une division par une
		// expression régulière, et « 0//0/ » réimprimé sans séparation devient
		// un commentaire de ligne. La parenthèse lève l'ambiguïté sans dépendre
		// d'une espace. Défaut trouvé par le fuzz de l'aller-retour.
		p.s("(" + v.Raw + ")")
	case *ast.BoolLit:
		p.s(strconv.FormatBool(v.Value))
	case *ast.NullLit:
		p.s("null")
	case *ast.ThisExpr:
		p.s("this")
	case *ast.SuperExpr:
		p.s("super")
	case *ast.MetaProperty:
		p.s(v.Meta + "." + v.Property)

	case *ast.TemplateLit:
		// Les fragments conservent leurs délimiteurs d'origine : réimprimer le
		// texte brut garantit la relecture à l'identique.
		for i, q := range v.Quasis {
			p.s(q)
			if i < len(v.Exprs) {
				p.node(v.Exprs[i])
			}
		}

	case *ast.TaggedTemplate:
		p.node(v.Tag)
		p.node(v.Quasi)

	case *ast.ArrayLit:
		p.elements(v.Elements)

	case *ast.ArrayPattern:
		p.elements(v.Elements)

	case *ast.ObjectLit:
		p.s("{")
		for i, prop := range v.Properties {
			if i > 0 {
				p.s(",")
			}
			p.property(prop)
		}
		p.s("}")

	case *ast.ObjectPattern:
		p.s("{")
		for i, prop := range v.Properties {
			if i > 0 {
				p.s(",")
			}
			p.property(prop)
		}
		p.s("}")

	case *ast.AssignPattern:
		p.node(v.Target)
		p.s("=")
		p.node(v.Default)

	case *ast.RestElement:
		p.s("...")
		p.node(v.Argument)

	case *ast.SpreadElement:
		p.s("...")
		p.node(v.Argument)

	case *ast.FunctionExpr:
		p.s("(")
		p.function(v, false)
		p.s(")")

	case *ast.ArrowFunction:
		p.s("(")
		if v.Async {
			p.s("async")
		}
		p.s("(")
		p.list(v.Params, ",")
		p.s(")=>")
		if b, ok := v.Body.(*ast.BlockStmt); ok {
			p.node(b)
		} else {
			p.s("(")
			p.node(v.Body)
			p.s(")")
		}
		p.s(")")

	case *ast.ClassExpr:
		p.s("(")
		p.class(v, false)
		p.s(")")

	case *ast.UnaryExpr:
		p.s("(" + v.Op)
		if isWordOp(v.Op) {
			p.s(" ")
		}
		p.node(v.Operand)
		p.s(")")

	case *ast.UpdateExpr:
		p.s("(")
		if v.Prefix {
			p.s(v.Op)
			p.node(v.Operand)
		} else {
			p.node(v.Operand)
			p.s(v.Op)
		}
		p.s(")")

	case *ast.BinaryExpr:
		p.s("(")
		p.node(v.Left)
		if isWordOp(v.Op) {
			p.s(" " + v.Op + " ")
		} else {
			p.s(v.Op)
		}
		p.node(v.Right)
		p.s(")")

	case *ast.LogicalExpr:
		p.s("(")
		p.node(v.Left)
		p.s(v.Op)
		p.node(v.Right)
		p.s(")")

	case *ast.AssignExpr:
		p.s("(")
		p.node(v.Target)
		p.s(v.Op)
		p.node(v.Value)
		p.s(")")

	case *ast.ConditionalExpr:
		p.s("(")
		p.node(v.Test)
		p.s("?")
		p.node(v.Consequent)
		p.s(":")
		p.node(v.Alternate)
		p.s(")")

	case *ast.CallExpr:
		p.node(v.Callee)
		if v.Optional {
			p.s("?.")
		}
		p.s("(")
		p.list(v.Args, ",")
		p.s(")")

	case *ast.NewExpr:
		// Le callee se parenthèse : « new (0()) » a pour callee un appel, et
		// « new 0()() » réimprimé sans parenthèse se relit en « new 0() »
		// suivi d'un appel — un autre arbre. Défaut trouvé par le fuzz.
		p.s("(new (")
		p.node(v.Callee)
		p.s(")(")
		p.list(v.Args, ",")
		p.s("))")

	case *ast.MemberExpr:
		// « 0 .A » est un accès de propriété sur un littéral numérique. Réimprimé
		// sans séparation, « 0.A » se relit en littéral numérique suivi d'un
		// identifiant accolé, donc en erreur. Parenthéser lève l'ambiguïté sans
		// dépendre d'une espace, que rien ne garantit de conserver.
		if _, isNum := v.Object.(*ast.NumberLit); isNum && !v.Computed {
			p.s("(")
			p.node(v.Object)
			p.s(")")
		} else {
			p.node(v.Object)
		}
		switch {
		case v.Computed && v.Optional:
			p.s("?.[")
			p.node(v.Property)
			p.s("]")
		case v.Computed:
			p.s("[")
			p.node(v.Property)
			p.s("]")
		case v.Optional:
			p.s("?.")
			p.node(v.Property)
		default:
			p.s(".")
			p.node(v.Property)
		}

	case *ast.ChainExpr:
		p.node(v.Expression)

	case *ast.SequenceExpr:
		p.s("(")
		p.list(v.Exprs, ",")
		p.s(")")

	case *ast.YieldExpr:
		p.s("(yield")
		if v.Delegate {
			p.s("*")
		}
		if v.Argument != nil {
			p.s(" ")
			p.node(v.Argument)
		}
		p.s(")")

	case *ast.AwaitExpr:
		p.s("(await ")
		p.node(v.Argument)
		p.s(")")

	case *ast.Property:
		p.property(v)
	}
}

// elements imprime une liste d'éléments de tableau en conservant les cases
// élidées. La virgule finale n'est pas décorative : « [,] » porte un élément
// élidé et « [] » n'en porte aucun — imprimer le second pour le premier change
// la longueur du tableau. Défaut trouvé par le fuzz de l'aller-retour.
func (p *printer) elements(els []ast.Expr) {
	p.s("[")
	for i, el := range els {
		if i > 0 {
			p.s(",")
		}
		p.node(el)
	}
	if len(els) > 0 && els[len(els)-1] == nil {
		p.s(",")
	}
	p.s("]")
}

func isWordOp(op string) bool {
	switch op {
	case "typeof", "void", "delete", "in", "instanceof":
		return true
	}
	return false
}

func (p *printer) varDecl(v *ast.VarDecl) {
	p.s(v.DeclKind + " ")
	for i, d := range v.Decls {
		if i > 0 {
			p.s(",")
		}
		p.node(d)
	}
}

func (p *printer) property(prop *ast.Property) {
	switch prop.Kind {
	case "spread", "rest":
		p.s("...")
		p.node(prop.Value)
		return
	case "get", "set":
		p.s(prop.Kind + " ")
		p.propKey(prop)
		if fn, ok := prop.Value.(*ast.FunctionExpr); ok {
			p.s("(")
			p.list(fn.Params, ",")
			p.s(")")
			p.node(fn.Body)
		}
		return
	}

	if prop.Method {
		fn, _ := prop.Value.(*ast.FunctionExpr)
		if fn != nil {
			if fn.Async {
				p.s("async ")
			}
			if fn.Generator {
				p.s("*")
			}
		}
		p.propKey(prop)
		if fn != nil {
			p.s("(")
			p.list(fn.Params, ",")
			p.s(")")
			p.node(fn.Body)
		}
		return
	}

	// La forme abrégée se réimprime abrégée : « {a} » et non « {a:a} ». Le
	// second relit un Property non abrégé, ce qui rompt l'aller-retour.
	if prop.Shorthand {
		if ap, ok := prop.Value.(*ast.AssignPattern); ok {
			p.node(ap.Target)
			p.s("=")
			p.node(ap.Default)
			return
		}
		p.node(prop.Key)
		return
	}

	p.propKey(prop)
	p.s(":")
	p.node(prop.Value)
}

func (p *printer) propKey(prop *ast.Property) {
	if prop.Computed {
		p.s("[")
		p.node(prop.Key)
		p.s("]")
		return
	}
	p.node(prop.Key)
}

func (p *printer) function(fn *ast.FunctionExpr, decl bool) {
	if fn.Async {
		p.s("async ")
	}
	p.s("function")
	if fn.Generator {
		p.s("*")
	}
	if fn.Name != nil {
		p.s(" " + fn.Name.Name)
	}
	p.s("(")
	p.list(fn.Params, ",")
	p.s(")")
	p.node(fn.Body)
}

func (p *printer) class(cl *ast.ClassExpr, decl bool) {
	p.s("class")
	if cl.Name != nil {
		p.s(" " + cl.Name.Name)
	}
	if cl.SuperClass != nil {
		p.s(" extends ")
		p.node(cl.SuperClass)
	}
	p.s("{\n")
	for _, m := range cl.Body {
		p.classMember(m)
		p.s("\n")
	}
	p.s("}")
}

func (p *printer) classMember(m *ast.ClassMember) {
	if m.Static {
		p.s("static ")
	}
	if m.Kind == "static-block" {
		if fn, ok := m.Value.(*ast.FunctionExpr); ok {
			p.node(fn.Body)
		}
		return
	}
	if m.Kind == "get" || m.Kind == "set" {
		p.s(m.Kind + " ")
	}

	fn, isFn := m.Value.(*ast.FunctionExpr)
	if isFn && m.Kind != "field" {
		if fn.Async {
			p.s("async ")
		}
		if fn.Generator {
			p.s("*")
		}
	}

	if m.Computed {
		p.s("[")
		p.node(m.Key)
		p.s("]")
	} else {
		p.node(m.Key)
	}

	if m.Kind == "field" {
		if m.Value != nil {
			p.s("=")
			p.node(m.Value)
		}
		p.s(";")
		return
	}
	if isFn {
		p.s("(")
		p.list(fn.Params, ",")
		p.s(")")
		p.node(fn.Body)
	}
}
