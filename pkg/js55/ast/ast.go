// Package ast décrit l'arbre syntaxique d'ECMAScript 2020.
//
// Note sur la contrainte C1 du plan : l'interdiction d'interface{} porte sur les
// frontières de CALCUL, où un passage par interface impose un boxing et une
// allocation sur le tas. L'arbre syntaxique n'en est pas une : il est construit
// une fois, parcouru une fois par le compilateur de bytecode, puis abandonné.
// Une interface Node y est donc légitime, et rien de tout cela n'atteint la
// boucle d'interprétation.
package ast

import "github.com/hazyhaar/js55/pkg/js55/lexer"

// Node est le contrat commun à tous les nœuds.
type Node interface {
	Pos() lexer.Position
	NodeType() string
}

// Base porte la position, partagée par tous les nœuds.
type Base struct{ P lexer.Position }

func (b Base) Pos() lexer.Position { return b.P }

// Expr et Stmt distinguent expressions et instructions au niveau du type, pour
// que le parser ne puisse pas les confondre par inadvertance.
type Expr interface {
	Node
	exprNode()
}

type Stmt interface {
	Node
	stmtNode()
}

// ─── Expressions ────────────────────────────────────────────────────────────

type (
	// Ident est un identifiant. Name porte le texte source, non déséchappé.
	Ident struct {
		Base
		Name string
	}

	// PrivateName est un nom privé de classe, #x.
	PrivateName struct {
		Base
		Name string
	}

	// NumberLit et BigIntLit conservent le texte source : la conversion en
	// valeur relève du moteur de nombres, feuille émise du jalon 5.
	NumberLit struct {
		Base
		Raw string
	}
	BigIntLit struct {
		Base
		Raw string
	}

	// StringLit conserve le texte brut, guillemets compris. Le déséchappement
	// est un travail du compilateur de bytecode, qui seul connaît le mode.
	StringLit struct {
		Base
		Raw string
	}

	BoolLit struct {
		Base
		Value bool
	}
	NullLit struct{ Base }

	RegExpLit struct {
		Base
		Raw string
	}

	// TemplateLit alterne fragments littéraux et expressions substituées.
	// len(Quasis) == len(Exprs)+1 en toute circonstance.
	TemplateLit struct {
		Base
		Quasis []string
		Exprs  []Expr
	}

	TaggedTemplate struct {
		Base
		Tag   Expr
		Quasi *TemplateLit
	}

	ThisExpr  struct{ Base }
	SuperExpr struct{ Base }

	// MetaProperty couvre new.target et import.meta.
	MetaProperty struct {
		Base
		Meta, Property string
	}

	ArrayLit struct {
		Base
		// Elements peut contenir nil pour une case élidée : [1,,2].
		Elements          []Expr
		RestTrailingComma bool
	}

	// Property est un membre de littéral objet ou de motif de décomposition.
	Property struct {
		Base
		Key       Expr // Ident, StringLit, NumberLit, PrivateName, ou calculé
		Value     Expr
		Computed  bool
		Shorthand bool
		// Kind vaut "init", "get", "set" ou "spread".
		Kind   string
		Method bool
	}

	ObjectLit struct {
		Base
		Properties        []*Property
		RestTrailingComma bool
	}

	FunctionExpr struct {
		Base
		Name      *Ident
		Params    []Expr
		Body      *BlockStmt
		Generator bool
		Async     bool
	}

	ArrowFunction struct {
		Base
		Params []Expr
		// Body est un *BlockStmt, ou une Expr pour un corps concis.
		Body  Node
		Async bool
	}

	ClassExpr struct {
		Base
		Name       *Ident
		SuperClass Expr
		Body       []*ClassMember
	}

	ClassMember struct {
		Base
		Key      Expr
		Value    Expr
		Kind     string // "method", "get", "set", "constructor", "field", "static-block"
		Static   bool
		Computed bool
	}

	UnaryExpr struct {
		Base
		Op      string
		Operand Expr
	}

	UpdateExpr struct {
		Base
		Op      string
		Prefix  bool
		Operand Expr
	}

	BinaryExpr struct {
		Base
		Op          string
		Left, Right Expr
	}

	LogicalExpr struct {
		Base
		Op          string // "&&", "||", "??"
		Left, Right Expr
	}

	AssignExpr struct {
		Base
		Op            string
		Target, Value Expr
	}

	ConditionalExpr struct {
		Base
		Test, Consequent, Alternate Expr
	}

	CallExpr struct {
		Base
		Callee   Expr
		Args     []Expr
		Optional bool // a?.()
	}

	NewExpr struct {
		Base
		Callee Expr
		Args   []Expr
	}

	MemberExpr struct {
		Base
		Object   Expr
		Property Expr
		Computed bool
		Optional bool // a?.b
	}

	// ChainExpr enveloppe une chaîne optionnelle, pour que le court-circuit
	// porte sur la chaîne entière et non sur un maillon.
	ChainExpr struct {
		Base
		Expression Expr
	}

	SequenceExpr struct {
		Base
		Exprs []Expr
	}

	SpreadElement struct {
		Base
		Argument Expr
	}

	YieldExpr struct {
		Base
		Argument Expr
		Delegate bool
	}

	AwaitExpr struct {
		Base
		Argument Expr
	}

	// ─── Motifs de décomposition ────────────────────────────────────────────

	ArrayPattern struct {
		Base
		Elements []Expr
	}
	ObjectPattern struct {
		Base
		Properties []*Property
	}
	AssignPattern struct {
		Base
		Target  Expr
		Default Expr
	}
	RestElement struct {
		Base
		Argument Expr
	}
)

func (*Ident) exprNode()           {}
func (*PrivateName) exprNode()     {}
func (*NumberLit) exprNode()       {}
func (*BigIntLit) exprNode()       {}
func (*StringLit) exprNode()       {}
func (*BoolLit) exprNode()         {}
func (*NullLit) exprNode()         {}
func (*RegExpLit) exprNode()       {}
func (*TemplateLit) exprNode()     {}
func (*TaggedTemplate) exprNode()  {}
func (*ThisExpr) exprNode()        {}
func (*SuperExpr) exprNode()       {}
func (*MetaProperty) exprNode()    {}
func (*ArrayLit) exprNode()        {}
func (*ObjectLit) exprNode()       {}
func (*FunctionExpr) exprNode()    {}
func (*ArrowFunction) exprNode()   {}
func (*ClassExpr) exprNode()       {}
func (*UnaryExpr) exprNode()       {}
func (*UpdateExpr) exprNode()      {}
func (*BinaryExpr) exprNode()      {}
func (*LogicalExpr) exprNode()     {}
func (*AssignExpr) exprNode()      {}
func (*ConditionalExpr) exprNode() {}
func (*CallExpr) exprNode()        {}
func (*NewExpr) exprNode()         {}
func (*MemberExpr) exprNode()      {}
func (*ChainExpr) exprNode()       {}
func (*SequenceExpr) exprNode()    {}
func (*SpreadElement) exprNode()   {}
func (*YieldExpr) exprNode()       {}
func (*AwaitExpr) exprNode()       {}
func (*ArrayPattern) exprNode()    {}
func (*ObjectPattern) exprNode()   {}
func (*AssignPattern) exprNode()   {}
func (*RestElement) exprNode()     {}
func (*Property) exprNode()        {}

func (*Ident) NodeType() string           { return "Ident" }
func (*PrivateName) NodeType() string     { return "PrivateName" }
func (*NumberLit) NodeType() string       { return "NumberLit" }
func (*BigIntLit) NodeType() string       { return "BigIntLit" }
func (*StringLit) NodeType() string       { return "StringLit" }
func (*BoolLit) NodeType() string         { return "BoolLit" }
func (*NullLit) NodeType() string         { return "NullLit" }
func (*RegExpLit) NodeType() string       { return "RegExpLit" }
func (*TemplateLit) NodeType() string     { return "TemplateLit" }
func (*TaggedTemplate) NodeType() string  { return "TaggedTemplate" }
func (*ThisExpr) NodeType() string        { return "ThisExpr" }
func (*SuperExpr) NodeType() string       { return "SuperExpr" }
func (*MetaProperty) NodeType() string    { return "MetaProperty" }
func (*ArrayLit) NodeType() string        { return "ArrayLit" }
func (*ObjectLit) NodeType() string       { return "ObjectLit" }
func (*FunctionExpr) NodeType() string    { return "FunctionExpr" }
func (*ArrowFunction) NodeType() string   { return "ArrowFunction" }
func (*ClassExpr) NodeType() string       { return "ClassExpr" }
func (*ClassMember) NodeType() string     { return "ClassMember" }
func (*UnaryExpr) NodeType() string       { return "UnaryExpr" }
func (*UpdateExpr) NodeType() string      { return "UpdateExpr" }
func (*BinaryExpr) NodeType() string      { return "BinaryExpr" }
func (*LogicalExpr) NodeType() string     { return "LogicalExpr" }
func (*AssignExpr) NodeType() string      { return "AssignExpr" }
func (*ConditionalExpr) NodeType() string { return "ConditionalExpr" }
func (*CallExpr) NodeType() string        { return "CallExpr" }
func (*NewExpr) NodeType() string         { return "NewExpr" }
func (*MemberExpr) NodeType() string      { return "MemberExpr" }
func (*ChainExpr) NodeType() string       { return "ChainExpr" }
func (*SequenceExpr) NodeType() string    { return "SequenceExpr" }
func (*SpreadElement) NodeType() string   { return "SpreadElement" }
func (*YieldExpr) NodeType() string       { return "YieldExpr" }
func (*AwaitExpr) NodeType() string       { return "AwaitExpr" }
func (*ArrayPattern) NodeType() string    { return "ArrayPattern" }
func (*ObjectPattern) NodeType() string   { return "ObjectPattern" }
func (*AssignPattern) NodeType() string   { return "AssignPattern" }
func (*RestElement) NodeType() string     { return "RestElement" }
func (*Property) NodeType() string        { return "Property" }

// ─── Instructions ───────────────────────────────────────────────────────────

type (
	Program struct {
		Base
		Body   []Stmt
		Module bool
	}

	BlockStmt struct {
		Base
		Body []Stmt
	}

	ExpressionStmt struct {
		Base
		Expression Expr
	}

	EmptyStmt    struct{ Base }
	DebuggerStmt struct{ Base }

	VarDeclarator struct {
		Base
		Target Expr
		Init   Expr
	}

	VarDecl struct {
		Base
		DeclKind string // "var", "let", "const"
		Decls    []*VarDeclarator
	}

	FunctionDecl struct {
		Base
		Fn *FunctionExpr
	}

	ClassDecl struct {
		Base
		Class *ClassExpr
	}

	ReturnStmt struct {
		Base
		Argument Expr
	}

	IfStmt struct {
		Base
		Test                  Expr
		Consequent, Alternate Stmt
	}

	ForStmt struct {
		Base
		Init         Node // Stmt (VarDecl) ou Expr, ou nil
		Test, Update Expr
		Body         Stmt
	}

	ForInStmt struct {
		Base
		Left  Node // VarDecl ou Expr
		Right Expr
		Body  Stmt
		Of    bool // false pour for-in, true pour for-of
		Await bool
	}

	WhileStmt struct {
		Base
		Test Expr
		Body Stmt
	}

	DoWhileStmt struct {
		Base
		Body Stmt
		Test Expr
	}

	BreakStmt struct {
		Base
		Label *Ident
	}
	ContinueStmt struct {
		Base
		Label *Ident
	}

	LabeledStmt struct {
		Base
		Label *Ident
		Body  Stmt
	}

	ThrowStmt struct {
		Base
		Argument Expr
	}

	CatchClause struct {
		Base
		Param Expr // nil pour catch sans liaison
		Body  *BlockStmt
	}

	TryStmt struct {
		Base
		Block     *BlockStmt
		Handler   *CatchClause
		Finalizer *BlockStmt
	}

	SwitchCase struct {
		Base
		Test Expr // nil pour default
		Body []Stmt
	}

	SwitchStmt struct {
		Base
		Discriminant Expr
		Cases        []*SwitchCase
	}

	WithStmt struct {
		Base
		Object Expr
		Body   Stmt
	}

	// ─── Modules ────────────────────────────────────────────────────────────

	ImportSpecifier struct {
		Base
		Imported *Ident // nil pour un import par défaut ou d'espace de noms
		Local    *Ident
		Kind     string // "named", "default", "namespace"
	}

	ImportDecl struct {
		Base
		Specifiers []*ImportSpecifier
		Source     *StringLit
	}

	ExportSpecifier struct {
		Base
		Local, Exported *Ident
	}

	ExportDecl struct {
		Base
		Declaration Stmt
		Specifiers  []*ExportSpecifier
		Source      *StringLit
		Default     bool
		Star        bool
		DefaultExpr Expr
	}
)

func (*Program) stmtNode()        {}
func (*BlockStmt) stmtNode()      {}
func (*ExpressionStmt) stmtNode() {}
func (*EmptyStmt) stmtNode()      {}
func (*DebuggerStmt) stmtNode()   {}
func (*VarDecl) stmtNode()        {}
func (*FunctionDecl) stmtNode()   {}
func (*ClassDecl) stmtNode()      {}
func (*ReturnStmt) stmtNode()     {}
func (*IfStmt) stmtNode()         {}
func (*ForStmt) stmtNode()        {}
func (*ForInStmt) stmtNode()      {}
func (*WhileStmt) stmtNode()      {}
func (*DoWhileStmt) stmtNode()    {}
func (*BreakStmt) stmtNode()      {}
func (*ContinueStmt) stmtNode()   {}
func (*LabeledStmt) stmtNode()    {}
func (*ThrowStmt) stmtNode()      {}
func (*TryStmt) stmtNode()        {}
func (*SwitchStmt) stmtNode()     {}
func (*WithStmt) stmtNode()       {}
func (*ImportDecl) stmtNode()     {}
func (*ExportDecl) stmtNode()     {}

func (*Program) NodeType() string         { return "Program" }
func (*BlockStmt) NodeType() string       { return "BlockStmt" }
func (*ExpressionStmt) NodeType() string  { return "ExpressionStmt" }
func (*EmptyStmt) NodeType() string       { return "EmptyStmt" }
func (*DebuggerStmt) NodeType() string    { return "DebuggerStmt" }
func (*VarDecl) NodeType() string         { return "VarDecl" }
func (*VarDeclarator) NodeType() string   { return "VarDeclarator" }
func (*FunctionDecl) NodeType() string    { return "FunctionDecl" }
func (*ClassDecl) NodeType() string       { return "ClassDecl" }
func (*ReturnStmt) NodeType() string      { return "ReturnStmt" }
func (*IfStmt) NodeType() string          { return "IfStmt" }
func (*ForStmt) NodeType() string         { return "ForStmt" }
func (*ForInStmt) NodeType() string       { return "ForInStmt" }
func (*WhileStmt) NodeType() string       { return "WhileStmt" }
func (*DoWhileStmt) NodeType() string     { return "DoWhileStmt" }
func (*BreakStmt) NodeType() string       { return "BreakStmt" }
func (*ContinueStmt) NodeType() string    { return "ContinueStmt" }
func (*LabeledStmt) NodeType() string     { return "LabeledStmt" }
func (*ThrowStmt) NodeType() string       { return "ThrowStmt" }
func (*CatchClause) NodeType() string     { return "CatchClause" }
func (*TryStmt) NodeType() string         { return "TryStmt" }
func (*SwitchCase) NodeType() string      { return "SwitchCase" }
func (*SwitchStmt) NodeType() string      { return "SwitchStmt" }
func (*WithStmt) NodeType() string        { return "WithStmt" }
func (*ImportSpecifier) NodeType() string { return "ImportSpecifier" }
func (*ImportDecl) NodeType() string      { return "ImportDecl" }
func (*ExportSpecifier) NodeType() string { return "ExportSpecifier" }
func (*ExportDecl) NodeType() string      { return "ExportDecl" }
