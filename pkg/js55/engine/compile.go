package engine

import (
	"fmt"
	"unicode/utf16"

	"github.com/hazyhaar/js55/pkg/js55/ast"
	"github.com/hazyhaar/js55/pkg/js55/str"
)

// Compilateur d'arbre syntaxique vers bytecode (plan §J4).
//
// PORTÉE COUVERTE ET LIMITES NOMMÉES.
//
// Sont compilés : littéraux, identifiants, opérateurs unaires, binaires,
// logiques (y compris ??), chaînes optionnelles, restes et diffusions de tableaux,
// décomposition (y compris reste), affectation, conditionnelle, appels, accès de
// propriété par point et par crochets, littéraux de tableau et d'objet,
// fonctions déclarées, fonctions expressions, fonctions fléchées, gabarits,
// séquences ; instructions d'expression, déclarations var/let/const, if, while,
// do-while, for à trois clauses, for-in, for-of (tableaux), switch, blocs, break, continue, return, throw, try/catch/finally.
//
// NE SONT PAS ENCORE COMPILÉS, et le compilateur le dit au lieu d'émettre du
// code faux : async, super, modules. Chacun produit une erreur de compilation nommée.
//
// SIMPLIFICATION DE PORTÉE, à lever plus tard : les variables sont allouées au
// niveau de la FONCTION, pas du bloc. « let » dans un bloc ne crée donc pas de
// portée propre et la zone morte temporelle n'est pas appliquée. Les fermetures,
// elles, sont réelles : chaque appel alloue un environnement dont le parent est
// l'environnement de définition.

// CompileError décrit un refus de compilation et sa position.
type CompileError struct {
	Msg  string
	Line int
	Col  int
}

func (e *CompileError) Error() string {
	return fmt.Sprintf("CompileError: %s (ligne %d, colonne %d)", e.Msg, e.Line, e.Col)
}

type funcScope struct {
	parent *funcScope
	names  map[string]int
	chunk  *Chunk
	consts map[string]bool
}

func (s *funcScope) declare(name string) int {
	if slot, ok := s.names[name]; ok {
		return slot
	}
	slot := s.chunk.Locals
	s.names[name] = slot
	s.chunk.Locals++
	return slot
}

type loopCtx struct {
	breaks    []int
	continues []int
	// continueTarget est le décalage vers lequel « continue » saute ; -1 tant
	// qu'il n'est pas connu.
	continueTarget int
	isSwitch       bool
	isLabel        bool
	label          string
	cptnSlot       int
	iterSlot       int
}

// Compiler produit du bytecode.
type Compiler struct {
	heap            *Heap
	chunk           *Chunk
	scope           *funcScope // nil au premier niveau : les noms y sont des globales
	loops           []*loopCtx
	pendingLabel    string
	strict          bool
	pendingFuncName string
	constNames      map[string]bool
	declaredVars    map[string]bool
	inGenerator     bool
	inArrow         bool
	iterIIFE        bool
	tdzHead         map[string]bool
	iterBind        map[string]bool
}

func (c *Compiler) exprNamed(e ast.Expr, name string) {
	saved := c.pendingFuncName
	if name != "" {
		c.pendingFuncName = name
	}
	c.expr(e)
	c.pendingFuncName = saved
}

// Compile compile un programme en une unité exécutable.
func Compile(h *Heap, prog *ast.Program, name string) (chunk *Chunk, err error) {
	return CompileMode(h, prog, name, false)
}

func CompileMode(h *Heap, prog *ast.Program, name string, strict bool) (chunk *Chunk, err error) {
	if prog != nil && prog.Module {
		strict = true
	}
	c := &Compiler{heap: h, chunk: NewChunk(name), strict: strict, constNames: map[string]bool{}}
	c.chunk.Strict = strict

	defer func() {
		if r := recover(); r != nil {
			if ce, ok := r.(*CompileError); ok {
				chunk, err = nil, ce
				return
			}
			panic(r)
		}
	}()

	c.hoistLexicals(prog.Body)
	c.hoistFunctions(prog.Body)

	for i, st := range prog.Body {
		last := i == len(prog.Body)-1
		c.stmt(st, last)
	}
	c.chunk.emit(OpHalt, 0)
	filterArchtimeCandidates(c.chunk)
	return c.chunk, nil
}

func (c *Compiler) fail(n ast.Node, msg string) {
	p := n.Pos()
	panic(&CompileError{Msg: msg, Line: p.Line, Col: p.Col})
}

func (c *Compiler) line(n ast.Node) int { return n.Pos().Line }

func (c *Compiler) constString(s string) uint32 {
	return c.chunk.addConst(Const{Kind: ConstString, Text: c.heap.Intern().InternGo(s)})
}

func (c *Compiler) constNumber(f float64) uint32 {
	return c.chunk.addConst(Const{Kind: ConstNumber, Num: f})
}

// resolve cherche un nom dans la chaîne de portées. Rend la profondeur
// d'environnement, l'emplacement, et si le nom est local.
func (c *Compiler) resolve(name string) (depth, slot int, ok bool) {
	d := 0
	for s := c.scope; s != nil; s = s.parent {
		if sl, found := s.names[name]; found {
			return d, sl, true
		}
		d++
	}
	return 0, 0, false
}

func packVar(depth, slot int) uint32 { return uint32(depth)<<16 | uint32(uint16(slot)) }

// ─── Instructions ───────────────────────────────────────────────────────────

// hoistFunctions déclare les fonctions d'un corps avant de le compiler.
func (c *Compiler) hoistLexicals(body []ast.Stmt) {
	for _, st := range body {
		var vd *ast.VarDecl
		switch s := st.(type) {
		case *ast.ClassDecl:
			if s.Class.Name != nil {
				c.emitTDZ(s.Class.Name.Name, c.line(s))
			}
		case *ast.VarDecl:
			vd = s
		case *ast.ExportDecl:
			if v, ok := s.Declaration.(*ast.VarDecl); ok {
				vd = v
			}
			if cl, ok := s.Declaration.(*ast.ClassDecl); ok && cl.Class.Name != nil {
				c.emitTDZ(cl.Class.Name.Name, c.line(cl))
			}
		}
		if vd == nil || vd.DeclKind == "var" {
			continue
		}
		for _, dec := range vd.Decls {
			collectLexicalIdents(dec.Target, func(name string) {
				if c.scope == nil {
					c.constNames[name] = vd.DeclKind == "const"
				} else {
					if c.scope.consts == nil {
						c.scope.consts = map[string]bool{}
					}
					c.scope.consts[name] = vd.DeclKind == "const"
				}
			})
			id, ok := dec.Target.(*ast.Ident)
			if !ok {
				collectLexicalIdents(dec.Target, func(name string) {
					c.emitTDZ(name, c.line(vd))
				})
				continue
			}
			c.emitTDZ(id.Name, c.line(vd))
		}
	}
}

func collectLexicalIdents(e ast.Expr, fn func(string)) {
	switch t := e.(type) {
	case *ast.Ident:
		fn(t.Name)
	case *ast.ArrayPattern:
		for _, el := range t.Elements {
			collectLexicalIdents(el, fn)
		}
	case *ast.ObjectPattern:
		for _, p := range t.Properties {
			if p != nil {
				collectLexicalIdents(p.Value, fn)
			}
		}
	case *ast.AssignPattern:
		collectLexicalIdents(t.Target, fn)
	case *ast.RestElement:
		collectLexicalIdents(t.Argument, fn)
	}
}

func (c *Compiler) emitTDZ(name string, line int) {
	c.chunk.emit32(OpGetGlobal, c.constString("\x00tdz"), line)
	if c.scope == nil {
		c.chunk.emit32(OpDefGlobal, c.constString(name), line)
		return
	}
	delete(c.scope.names, name)
	slot := c.scope.declare(name)
	c.chunk.emit32(OpSetLocal, packVar(0, slot), line)
	c.chunk.emit(OpPop, line)
}

func (c *Compiler) hoistVars(body []ast.Stmt) {
	if c.scope == nil {
		return
	}
	c.walkVarStmts(body)
}

func (c *Compiler) hoistVarDecl(d *ast.VarDecl) {
	if d == nil || d.DeclKind != "var" {
		return
	}
	for _, dec := range d.Decls {
		collectLexicalIdents(dec.Target, func(name string) {
			c.scope.declare(name)
		})
	}
}

// Blocks use distinct slots in the same function environment. Copy the name
// table so closures retain the binding selected at their declaration site.
func (c *Compiler) blockScope() *funcScope {
	old := c.scope
	if old != nil {
		c.scope = &funcScope{parent: old.parent, chunk: old.chunk, names: map[string]int{}, consts: map[string]bool{}}
		for name, slot := range old.names {
			c.scope.names[name] = slot
		}
		for name, constant := range old.consts {
			c.scope.consts[name] = constant
		}
	}
	return old
}

func (c *Compiler) walkVarStmts(body []ast.Stmt) {
	for _, st := range body {
		c.walkVarStmt(st)
	}
}

func (c *Compiler) walkVarStmt(st ast.Stmt) {
	if st == nil {
		return
	}
	switch s := st.(type) {
	case *ast.VarDecl:
		c.hoistVarDecl(s)
	case *ast.BlockStmt:
		c.walkVarStmts(s.Body)
	case *ast.IfStmt:
		c.walkVarStmt(s.Consequent)
		c.walkVarStmt(s.Alternate)
	case *ast.WhileStmt:
		c.walkVarStmt(s.Body)
	case *ast.DoWhileStmt:
		c.walkVarStmt(s.Body)
	case *ast.ForStmt:
		if vd, ok := s.Init.(*ast.VarDecl); ok {
			c.hoistVarDecl(vd)
		}
		c.walkVarStmt(s.Body)
	case *ast.ForInStmt:
		if vd, ok := s.Left.(*ast.VarDecl); ok {
			c.hoistVarDecl(vd)
		}
		c.walkVarStmt(s.Body)
	case *ast.TryStmt:
		if s.Block != nil {
			c.walkVarStmt(s.Block)
		}
		if s.Handler != nil && s.Handler.Body != nil {
			c.walkVarStmt(s.Handler.Body)
		}
		if s.Finalizer != nil {
			c.walkVarStmt(s.Finalizer)
		}
	case *ast.SwitchStmt:
		for _, cs := range s.Cases {
			if cs != nil {
				c.walkVarStmts(cs.Body)
			}
		}
	case *ast.LabeledStmt:
		c.walkVarStmt(s.Body)
	case *ast.WithStmt:
		c.walkVarStmt(s.Body)
	case *ast.ExportDecl:
		if s.Declaration != nil {
			c.walkVarStmt(s.Declaration)
		}
	}
}

func (c *Compiler) hoistFunctions(body []ast.Stmt) {
	type hoistFn struct {
		fd              *ast.FunctionDecl
		name            string
		slot            int
		isExportDefault bool
		exportNamed     string
	}
	var list []hoistFn
	for _, st := range body {
		switch s := st.(type) {
		case *ast.FunctionDecl:
			name := s.Fn.Name.Name
			it := hoistFn{fd: s, name: name}
			if c.scope != nil {
				it.slot = c.scope.declare(name)
			}
			list = append(list, it)
		case *ast.ExportDecl:
			if s.Declaration != nil {
				if fd, ok := s.Declaration.(*ast.FunctionDecl); ok {
					if s.Default {
						name := "default"
						if fd.Fn.Name != nil {
							name = fd.Fn.Name.Name
						}
						it := hoistFn{fd: fd, name: name, isExportDefault: true}
						if fd.Fn.Name != nil && c.scope != nil {
							it.slot = c.scope.declare(name)
						}
						list = append(list, it)
					} else {
						if fd.Fn.Name == nil {
							c.fail(s, "export de fonction sans nom")
						}
						name := fd.Fn.Name.Name
						it := hoistFn{fd: fd, name: name, exportNamed: name}
						if c.scope != nil {
							it.slot = c.scope.declare(name)
						}
						list = append(list, it)
					}
				}
			}
		}
	}
	for _, it := range list {
		fn := c.compileFunction(it.fd.Fn, it.name, false)
		k := c.chunk.addConst(Const{Kind: ConstFunction, Fn: fn})
		c.chunk.emit32(OpConst, k, c.line(it.fd))
		if it.isExportDefault {
			if it.fd.Fn.Name != nil {
				c.chunk.emit(OpDup, c.line(it.fd))
				if c.scope == nil {
					c.chunk.emit32(OpDefGlobal, c.constString(it.name), c.line(it.fd))
				} else {
					c.chunk.emit32(OpSetLocal, packVar(0, it.slot), c.line(it.fd))
					c.chunk.emit(OpPop, c.line(it.fd))
				}
			}
			c.chunk.emit32(OpExport, c.constString("default"), c.line(it.fd))
			c.chunk.emit(OpPop, c.line(it.fd))
			c.chunk.ExportDefault = true
			if !c.chunk.HasExport("default") {
				c.chunk.ExportNames = append(c.chunk.ExportNames, "default")
			}
		} else if it.exportNamed != "" {
			c.chunk.emit(OpDup, c.line(it.fd))
			if c.scope == nil {
				c.chunk.emit32(OpDefGlobal, c.constString(it.name), c.line(it.fd))
			} else {
				c.chunk.emit32(OpSetLocal, packVar(0, it.slot), c.line(it.fd))
				c.chunk.emit(OpPop, c.line(it.fd))
			}
			c.chunk.emit32(OpExport, c.constString(it.exportNamed), c.line(it.fd))
			c.chunk.emit(OpPop, c.line(it.fd))
			if !c.chunk.HasExport(it.exportNamed) {
				c.chunk.ExportNames = append(c.chunk.ExportNames, it.exportNamed)
			}
		} else {
			if c.scope == nil {
				c.chunk.emit32(OpDefGlobal, c.constString(it.name), c.line(it.fd))
			} else {
				c.chunk.emit32(OpSetLocal, packVar(0, it.slot), c.line(it.fd))
				c.chunk.emit(OpPop, c.line(it.fd))
			}
		}
	}
}

// stmt compile une instruction. keepValue demande de laisser la valeur de la
// dernière expression sur la pile, ce qui donne au script sa valeur de
// complétion.
func (c *Compiler) stmt(st ast.Stmt, keepValue bool) {
	switch s := st.(type) {

	case *ast.ExpressionStmt:
		c.expr(s.Expression)
		if slot := c.innermostCptn(); slot >= 0 {
			c.chunk.emit32(OpSetLocal, packVar(0, slot), c.line(s))
		}
		if !keepValue {
			c.chunk.emit(OpPop, c.line(s))
		}

	case *ast.EmptyStmt, *ast.DebuggerStmt:
		if keepValue {
			c.chunk.emit(OpUndefined, c.line(st))
		}

	case *ast.VarDecl:
		c.varDecl(s)
		if keepValue {
			c.chunk.emit(OpUndefined, c.line(s))
		}

	case *ast.FunctionDecl:
		// Déjà émise par hoistFunctions.
		if keepValue {
			c.chunk.emit(OpUndefined, c.line(s))
		}

	case *ast.BlockStmt:
		outer := c.blockScope()
		if c.scope != nil {
			c.hoistLexicals(s.Body)
		}
		c.hoistFunctions(s.Body)
		for i, in := range s.Body {
			c.stmt(in, keepValue && i == len(s.Body)-1)
		}
		if keepValue && len(s.Body) == 0 {
			c.chunk.emit(OpUndefined, c.line(s))
		}
		c.scope = outer

	case *ast.IfStmt:
		c.expr(s.Test)
		jf := c.chunk.emit32(OpJumpIfFalse, 0, c.line(s))
		c.stmt(s.Consequent, false)
		if s.Alternate != nil {
			jend := c.chunk.emit32(OpJump, 0, c.line(s))
			c.chunk.patch32(jf, uint32(int32(len(c.chunk.Code)-(jf+5))))
			c.stmt(s.Alternate, false)
			c.chunk.patch32(jend, uint32(int32(len(c.chunk.Code)-(jend+5))))
		} else {
			c.chunk.patch32(jf, uint32(int32(len(c.chunk.Code)-(jf+5))))
		}
		if keepValue {
			c.chunk.emit(OpUndefined, c.line(s))
		}

	case *ast.WhileStmt:
		top := len(c.chunk.Code)
		c.expr(s.Test)
		jf := c.chunk.emit32(OpJumpIfFalse, 0, c.line(s))
		lp := c.pushLoop()
		c.initLoopCptn(lp, keepValue, c.line(s))
		c.stmt(s.Body, false)
		lp.continueTarget = len(c.chunk.Code)
		back := c.chunk.emit32(OpJump, 0, c.line(s))
		c.chunk.patch32(back, uint32(int32(top-(back+5))))
		c.chunk.patch32(jf, uint32(int32(len(c.chunk.Code)-(jf+5))))
		c.popLoop(lp, len(c.chunk.Code))
		c.emitLoopCptn(lp, keepValue, c.line(s))

	case *ast.DoWhileStmt:
		top := len(c.chunk.Code)
		lp := c.pushLoop()
		c.initLoopCptn(lp, keepValue, c.line(s))
		c.stmt(s.Body, false)
		lp.continueTarget = len(c.chunk.Code)
		c.expr(s.Test)
		jt := c.chunk.emit32(OpJumpIfTrue, 0, c.line(s))
		c.chunk.patch32(jt, uint32(int32(top-(jt+5))))
		c.popLoop(lp, len(c.chunk.Code))
		c.emitLoopCptn(lp, keepValue, c.line(s))

	case *ast.ForStmt:
		outer := c.scope
		if init, ok := s.Init.(*ast.VarDecl); ok && init.DeclKind != "var" && c.scope != nil {
			c.blockScope()
			c.hoistLexicals([]ast.Stmt{init})
		}
		if s.Init != nil {
			switch in := s.Init.(type) {
			case *ast.VarDecl:
				c.varDecl(in)
			case ast.Expr:
				c.expr(in)
				c.chunk.emit(OpPop, c.line(s))
			}
		}
		top := len(c.chunk.Code)
		jf := -1
		if s.Test != nil {
			c.expr(s.Test)
			jf = c.chunk.emit32(OpJumpIfFalse, 0, c.line(s))
		}
		lp := c.pushLoop()
		c.initLoopCptn(lp, keepValue, c.line(s))
		c.stmt(s.Body, false)
		lp.continueTarget = len(c.chunk.Code)
		if s.Update != nil {
			c.expr(s.Update)
			c.chunk.emit(OpPop, c.line(s))
		}
		back := c.chunk.emit32(OpJump, 0, c.line(s))
		c.chunk.patch32(back, uint32(int32(top-(back+5))))
		if jf >= 0 {
			c.chunk.patch32(jf, uint32(int32(len(c.chunk.Code)-(jf+5))))
		}
		c.popLoop(lp, len(c.chunk.Code))
		c.emitLoopCptn(lp, keepValue, c.line(s))
		c.scope = outer

	case *ast.BreakStmt:
		lp := c.findBreakTarget(s.Label)
		if lp == nil {
			c.fail(s, "« break » hors d'une boucle")
		}
		c.closeForOfUntil(lp, s)
		lp.breaks = append(lp.breaks, c.chunk.emit32(OpJump, 0, c.line(s)))

	case *ast.ContinueStmt:
		lp := c.findContinueTarget(s.Label)
		if lp == nil {
			c.fail(s, "« continue » hors d'une boucle")
		}
		c.closeForOfUntil(lp, s)
		lp.continues = append(lp.continues, c.chunk.emit32(OpJump, 0, c.line(s)))

	case *ast.ReturnStmt:
		if s.Argument != nil {
			c.expr(s.Argument)
		} else {
			c.chunk.emit(OpUndefined, c.line(s))
		}
		needClose := false
		for _, lp := range c.loops {
			if lp.iterSlot >= 0 {
				needClose = true
				break
			}
		}
		if needClose {
			tmp := c.allocLocal()
			c.chunk.emit32(OpSetLocal, packVar(0, tmp), c.line(s))
			c.chunk.emit(OpPop, c.line(s))
			c.closeForOfUntil(nil, s)
			c.chunk.emit32(OpGetLocal, packVar(0, tmp), c.line(s))
		}
		c.chunk.emit(OpReturn, c.line(s))

	case *ast.ThrowStmt:
		c.expr(s.Argument)
		c.chunk.emit(OpThrow, c.line(s))

	case *ast.LabeledStmt:
		switch s.Body.(type) {
		case *ast.ForStmt, *ast.WhileStmt, *ast.DoWhileStmt, *ast.ForInStmt, *ast.SwitchStmt:
			c.pendingLabel = s.Label.Name
			c.stmt(s.Body, keepValue)
		default:
			lp := &loopCtx{continueTarget: -1, label: s.Label.Name, isLabel: true}
			c.loops = append(c.loops, lp)
			c.stmt(s.Body, keepValue)
			c.popLoop(lp, len(c.chunk.Code))
		}
	case *ast.TryStmt:
		tryStart := len(c.chunk.Code)
		c.stmt(s.Block, keepValue)
		tryEnd := len(c.chunk.Code)

		jumpOverCatch := -1
		if s.Handler != nil || s.Finalizer != nil {
			jumpOverCatch = c.chunk.emit32(OpJump, 0, c.line(s))
		}

		catchIP := -1
		catchSlot := -1
		catchDepth := 0
		if s.Handler != nil {
			catchIP = len(c.chunk.Code)
			if s.Handler.Param != nil {
				if ident, ok := s.Handler.Param.(*ast.Ident); ok {
					if c.scope != nil {
						catchSlot = c.scope.declare(ident.Name)
					} else {
						nameConst := c.chunk.addConst(Const{Kind: ConstString, Text: str.FromGo(ident.Name)})
						c.chunk.emit32(OpDefGlobal, nameConst, c.line(s.Handler))
					}
				}
			} else {
				c.chunk.emit(OpPop, c.line(s.Handler))
			}

			c.stmt(s.Handler.Body, keepValue)
		}

		if jumpOverCatch >= 0 {
			c.chunk.patch32(jumpOverCatch, uint32(int32(len(c.chunk.Code)-(jumpOverCatch+5))))
		}

		finallyIP := -1
		finallyEnd := -1
		if s.Finalizer != nil {
			finallyIP = len(c.chunk.Code)
			c.stmt(s.Finalizer, false)
			finallyEnd = len(c.chunk.Code)
		}

		c.chunk.Handlers = append(c.chunk.Handlers, ExceptionHandler{
			Start:      tryStart,
			End:        tryEnd,
			CatchIP:    catchIP,
			FinallyIP:  finallyIP,
			FinallyEnd: finallyEnd,
			CatchSlot:  catchSlot,
			Depth:      catchDepth,
		})
	case *ast.SwitchStmt:
		c.expr(s.Discriminant)
		lp := c.pushLoop()
		lp.isSwitch = true

		n := len(s.Cases)
		matchJumps := make([]int, n)
		defaultIdx := -1
		for i, cs := range s.Cases {
			if cs.Test == nil {
				defaultIdx = i
				matchJumps[i] = -1
				continue
			}
			c.chunk.emit(OpDup, c.line(cs))
			c.expr(cs.Test)
			c.chunk.emit(OpStrictEq, c.line(cs))
			matchJumps[i] = c.chunk.emit32(OpJumpIfTrue, 0, c.line(cs))
		}
		noMatch := c.chunk.emit32(OpJump, 0, c.line(s))

		entryIPs := make([]int, n)
		entryToBody := make([]int, n)
		for i, cs := range s.Cases {
			entryIPs[i] = len(c.chunk.Code)
			c.chunk.emit(OpPop, c.line(cs))
			entryToBody[i] = c.chunk.emit32(OpJump, 0, c.line(cs))
		}
		endPopIP := len(c.chunk.Code)
		c.chunk.emit(OpPop, c.line(s))
		endAfterPop := c.chunk.emit32(OpJump, 0, c.line(s))

		for i := range s.Cases {
			if matchJumps[i] >= 0 {
				c.chunk.patch32(matchJumps[i], uint32(int32(entryIPs[i]-(matchJumps[i]+5))))
			}
		}
		if defaultIdx >= 0 {
			c.chunk.patch32(noMatch, uint32(int32(entryIPs[defaultIdx]-(noMatch+5))))
		} else {
			c.chunk.patch32(noMatch, uint32(int32(endPopIP-(noMatch+5))))
		}

		for i, cs := range s.Cases {
			bodyIP := len(c.chunk.Code)
			c.chunk.patch32(entryToBody[i], uint32(int32(bodyIP-(entryToBody[i]+5))))
			for _, st := range cs.Body {
				c.stmt(st, false)
			}
		}
		endIP := len(c.chunk.Code)
		c.chunk.patch32(endAfterPop, uint32(int32(endIP-(endAfterPop+5))))
		c.popLoop(lp, endIP)
		if keepValue {
			c.chunk.emit(OpUndefined, c.line(s))
		}
	case *ast.ForInStmt:
		c.compileForInOf(s, keepValue)
	case *ast.ClassDecl:
		c.compileClass(s.Class)
		if s.Class.Name != nil {
			if c.scope != nil {
				slot := c.scope.declare(s.Class.Name.Name)
				c.chunk.emit32(OpSetLocal, packVar(0, slot), c.line(s))
				c.chunk.emit(OpPop, c.line(s))
			} else {
				c.chunk.emit32(OpDefGlobal, c.constString(s.Class.Name.Name), c.line(s))
			}
		}
		if keepValue {
			c.chunk.emit(OpUndefined, c.line(s))
		}
	case *ast.WithStmt:
		c.fail(s, "« with » n'est pas compilé et ne le sera pas")
	case *ast.ImportDecl:
		c.fail(s, "les modules ne sont pas encore compilés")
	case *ast.ExportDecl:
		c.compileExportDecl(s, keepValue)

	default:
		c.fail(st, "instruction non compilée : "+st.NodeType())
	}
}

func (c *Compiler) compileExportDecl(s *ast.ExportDecl, keepValue bool) {
	if s.Default {
		c.chunk.ExportDefault = true
		if !c.chunk.HasExport("default") {
			c.chunk.ExportNames = append(c.chunk.ExportNames, "default")
		}

		if s.Declaration != nil {
			switch d := s.Declaration.(type) {
			case *ast.FunctionDecl:
				// Déjà émise par hoistFunctions.
				if keepValue {
					c.chunk.emit(OpUndefined, c.line(s))
				}
			case *ast.ClassDecl:
				saved := c.pendingFuncName
				if d.Class.Name == nil {
					c.pendingFuncName = "default"
				} else {
					c.pendingFuncName = d.Class.Name.Name
				}
				c.compileClass(d.Class)
				c.pendingFuncName = saved
				if d.Class.Name != nil {
					c.chunk.emit(OpDup, c.line(s))
					if c.scope != nil {
						slot := c.scope.declare(d.Class.Name.Name)
						c.chunk.emit32(OpSetLocal, packVar(0, slot), c.line(s))
						c.chunk.emit(OpPop, c.line(s))
					} else {
						c.chunk.emit32(OpDefGlobal, c.constString(d.Class.Name.Name), c.line(s))
					}
				}
				c.chunk.emit32(OpExport, c.constString("default"), c.line(s))
				c.chunk.emit(OpPop, c.line(s))
				if keepValue {
					c.chunk.emit(OpUndefined, c.line(s))
				}
			default:
				c.fail(s, "déclaration exportée non supportée : "+d.NodeType())
			}
			return
		}

		if s.DefaultExpr != nil {
			c.exprNamed(s.DefaultExpr, "default")
			c.chunk.emit32(OpExport, c.constString("default"), c.line(s))
			if !keepValue {
				c.chunk.emit(OpPop, c.line(s))
			}
			return
		}

		c.fail(s, "export default vide")
		return
	}

	if s.Source != nil || s.Star {
		c.fail(s, "les modules ne sont pas encore compilés")
		return
	}

	if s.Declaration != nil {
		switch d := s.Declaration.(type) {
		case *ast.VarDecl:
			c.varDecl(d)
			for _, dec := range d.Decls {
				collectLexicalIdents(dec.Target, func(name string) {
					if !c.chunk.HasExport(name) {
						c.chunk.ExportNames = append(c.chunk.ExportNames, name)
					}
					c.loadIdent(&ast.Ident{Base: ast.Base{P: dec.Target.Pos()}, Name: name})
					c.chunk.emit32(OpExport, c.constString(name), c.line(s))
					c.chunk.emit(OpPop, c.line(s))
				})
			}
			if keepValue {
				c.chunk.emit(OpUndefined, c.line(s))
			}
		case *ast.FunctionDecl:
			// Déjà émise et exportée par hoistFunctions.
			if keepValue {
				c.chunk.emit(OpUndefined, c.line(s))
			}
		case *ast.ClassDecl:
			if d.Class.Name == nil {
				c.fail(s, "export de classe sans nom")
			}
			name := d.Class.Name.Name
			saved := c.pendingFuncName
			c.pendingFuncName = name
			c.compileClass(d.Class)
			c.pendingFuncName = saved
			c.chunk.emit(OpDup, c.line(s))
			if c.scope != nil {
				slot := c.scope.declare(name)
				c.chunk.emit32(OpSetLocal, packVar(0, slot), c.line(s))
				c.chunk.emit(OpPop, c.line(s))
			} else {
				c.chunk.emit32(OpDefGlobal, c.constString(name), c.line(s))
			}
			c.chunk.emit32(OpExport, c.constString(name), c.line(s))
			c.chunk.emit(OpPop, c.line(s))
			if !c.chunk.HasExport(name) {
				c.chunk.ExportNames = append(c.chunk.ExportNames, name)
			}
			if keepValue {
				c.chunk.emit(OpUndefined, c.line(s))
			}
		default:
			c.fail(s, "déclaration exportée non supportée : "+d.NodeType())
		}
		return
	}

	if len(s.Specifiers) > 0 {
		for _, spec := range s.Specifiers {
			exportName := spec.Local.Name
			if spec.Exported != nil {
				exportName = spec.Exported.Name
			}
			if !c.chunk.HasExport(exportName) {
				c.chunk.ExportNames = append(c.chunk.ExportNames, exportName)
			}
			if exportName == "default" {
				c.chunk.ExportDefault = true
			}
			c.loadIdent(spec.Local)
			c.chunk.emit32(OpExport, c.constString(exportName), c.line(spec))
			c.chunk.emit(OpPop, c.line(spec))
		}
		if keepValue {
			c.chunk.emit(OpUndefined, c.line(s))
		}
		return
	}

	c.fail(s, "déclaration d'exportation non supportée")
}

func (c *Compiler) allocLocal() int {
	if c.scope != nil {
		return c.scope.declare(fmt.Sprintf("\x00t%d", c.chunk.Locals))
	}
	n := c.chunk.Locals
	c.chunk.Locals++
	return n
}

func (c *Compiler) compileForInOf(s *ast.ForInStmt, keepValue bool) {
	line := c.line(s)
	arrSlot := c.allocLocal()
	idxSlot := c.allocLocal()
	if vd, ok := s.Left.(*ast.VarDecl); ok {
		if vd.DeclKind == "var" && !forOfPatternDecl(vd) {
			c.varDecl(vd)
		}
	}
	if s.Of {
		c.compileForOf(s, keepValue, line)
		return
	}
	c.chunk.emit32(OpGetGlobal, c.constString("Object"), line)
	c.expr(s.Right)
	// Unlike Object.keys, for-in skips null and undefined.
	c.chunk.emit(OpDup, line)
	c.chunk.emit(OpIsNullish, line)
	nonnull := c.chunk.emit32(OpJumpIfFalse, 0, line)
	c.chunk.emit(OpPop, line)
	c.chunk.emit(OpNewObject, line)
	c.chunk.patch32(nonnull, uint32(int32(len(c.chunk.Code)-(nonnull+5))))
	c.chunk.emit32(OpCallMethod, packVar(1, int(c.constString("keys"))), line)
	c.chunk.emit32(OpSetLocal, packVar(0, arrSlot), line)
	c.chunk.emit(OpPop, line)
	c.chunk.emit32(OpInt32, 0, line)
	c.chunk.emit32(OpSetLocal, packVar(0, idxSlot), line)
	c.chunk.emit(OpPop, line)

	top := len(c.chunk.Code)
	c.chunk.emit32(OpGetLocal, packVar(0, idxSlot), line)
	c.chunk.emit32(OpGetLocal, packVar(0, arrSlot), line)
	c.chunk.emit32(OpGetProp, c.constString("length"), line)
	c.chunk.emit(OpLt, line)
	jf := c.chunk.emit32(OpJumpIfFalse, 0, line)

	c.chunk.emit32(OpGetLocal, packVar(0, arrSlot), line)
	c.chunk.emit32(OpGetLocal, packVar(0, idxSlot), line)
	c.chunk.emit(OpGetElem, line)

	lp := c.pushLoop()
	c.initLoopCptn(lp, keepValue, line)
	outer := c.scope
	if vd, ok := s.Left.(*ast.VarDecl); ok && vd.DeclKind != "var" && c.scope != nil {
		c.blockScope()
		c.hoistLexicals([]ast.Stmt{vd})
	}
	c.storeLoopLeft(s.Left, line)
	c.stmt(s.Body, false)
	c.scope = outer
	lp.continueTarget = len(c.chunk.Code)
	c.chunk.emit32(OpGetLocal, packVar(0, idxSlot), line)
	c.chunk.emit32(OpInt32, 1, line)
	c.chunk.emit(OpAdd, line)
	c.chunk.emit32(OpSetLocal, packVar(0, idxSlot), line)
	c.chunk.emit(OpPop, line)
	back := c.chunk.emit32(OpJump, 0, line)
	c.chunk.patch32(back, uint32(int32(top-(back+5))))
	endIP := len(c.chunk.Code)
	c.chunk.patch32(jf, uint32(int32(endIP-(jf+5))))
	c.popLoop(lp, endIP)
	c.emitLoopCptn(lp, keepValue, line)
}

func (c *Compiler) compileForOf(s *ast.ForInStmt, keepValue bool, line int) {
	iterSlot := c.allocLocal()
	nextSlot := c.allocLocal()
	if vd, ok := s.Left.(*ast.VarDecl); ok && (vd.DeclKind == "let" || vd.DeclKind == "const") {
		savedTDZ := c.tdzHead
		c.tdzHead = map[string]bool{}
		collectLexicalIdents(vd.Decls[0].Target, func(name string) { c.tdzHead[name] = true })
		c.expr(s.Right)
		c.tdzHead = savedTDZ
	} else {
		c.expr(s.Right)
	}
	c.chunk.emit(OpGetIterator, line)
	c.chunk.emit32(OpSetLocal, packVar(0, iterSlot), line)
	c.chunk.emit(OpPop, line)
	c.chunk.emit32(OpGetLocal, packVar(0, iterSlot), line)
	c.chunk.emit32(OpGetProp, c.constString("next"), line)
	c.chunk.emit32(OpSetLocal, packVar(0, nextSlot), line)
	c.chunk.emit(OpPop, line)

	top := len(c.chunk.Code)
	c.chunk.emit32(OpGetLocal, packVar(0, iterSlot), line)
	c.chunk.emit32(OpGetLocal, packVar(0, nextSlot), line)
	c.chunk.emit32(OpNewArray, 0, line)
	c.chunk.emit(OpCallSpreadThis, line)
	if s.Await {
		c.chunk.emit(OpAwait, line)
	}
	c.chunk.emit(OpDup, line)
	c.chunk.emit(OpIsNullish, line)
	jNull := c.chunk.emit32(OpJumpIfTrue, 0, line)
	c.chunk.emit(OpDup, line)
	c.chunk.emit(OpTypeof, line)
	c.chunk.emit32(OpConst, c.constString("object"), line)
	c.chunk.emit(OpStrictEq, line)
	jObjOk := c.chunk.emit32(OpJumpIfTrue, 0, line)
	c.chunk.patch32(jNull, uint32(int32(len(c.chunk.Code)-(jNull+5))))
	c.chunk.emit(OpPop, line)
	c.chunk.emit32(OpGetGlobal, c.constString("TypeError"), line)
	c.chunk.emit32(OpConst, c.constString("Iterator result is not an object"), line)
	c.chunk.emit16(OpNew, 1, line)
	c.chunk.emit(OpThrow, line)
	c.chunk.patch32(jObjOk, uint32(int32(len(c.chunk.Code)-(jObjOk+5))))
	c.chunk.emit(OpDup, line)
	c.chunk.emit32(OpGetProp, c.constString("done"), line)
	jf := c.chunk.emit32(OpJumpIfTrue, 0, line)
	c.chunk.emit32(OpGetProp, c.constString("value"), line)
	c.chunk.emit(OpNop, line)
	tryStart := len(c.chunk.Code)
	lp := c.pushLoop()
	lp.iterSlot = iterSlot
	c.initLoopCptn(lp, keepValue, line)
	if vd, ok := s.Left.(*ast.VarDecl); ok && (vd.DeclKind == "let" || vd.DeclKind == "const") {
		c.compileLexicalIterBody(vd, s.Body, line)
	} else {
		c.storeLoopLeft(s.Left, line)
		c.stmt(s.Body, false)
	}
	tryEnd := len(c.chunk.Code)
	lp.continueTarget = len(c.chunk.Code)
	back := c.chunk.emit32(OpJump, 0, line)
	c.chunk.patch32(back, uint32(int32(top-(back+5))))

	jumpOverCatch := c.chunk.emit32(OpJump, 0, line)
	catchIP := len(c.chunk.Code)
	excSlot := c.allocLocal()
	c.chunk.emit32(OpSetLocal, packVar(0, excSlot), line)
	c.chunk.emit(OpPop, line)
	c.emitIteratorClose(iterSlot, line, false)
	c.chunk.emit32(OpGetLocal, packVar(0, excSlot), line)
	c.chunk.emit(OpThrow, line)
	c.chunk.Handlers = append(c.chunk.Handlers, ExceptionHandler{
		Start:     tryStart,
		End:       tryEnd,
		CatchIP:   catchIP,
		FinallyIP: -1,
		CatchSlot: -1,
	})
	c.chunk.patch32(jumpOverCatch, uint32(int32(len(c.chunk.Code)-(jumpOverCatch+5))))

	doneIP := len(c.chunk.Code)
	c.chunk.patch32(jf, uint32(int32(doneIP-(jf+5))))
	c.chunk.emit(OpPop, line)
	doneJump := c.chunk.emit32(OpJump, 0, line)

	closeIP := len(c.chunk.Code)
	c.emitIteratorClose(iterSlot, line, true)

	endIP := len(c.chunk.Code)
	c.chunk.patch32(doneJump, uint32(int32(endIP-(doneJump+5))))
	c.popLoop(lp, closeIP)
	c.emitLoopCptn(lp, keepValue, line)
}

func (c *Compiler) compileLexicalIterBody(vd *ast.VarDecl, body ast.Stmt, line int) {
	savedIIFE := c.iterIIFE
	savedBind := c.iterBind
	c.iterIIFE = true
	c.iterBind = map[string]bool{}
	collectLexicalIdents(vd.Decls[0].Target, func(name string) { c.iterBind[name] = true })
	defer func() { c.iterIIFE = savedIIFE; c.iterBind = savedBind }()
	tmp := c.allocLocal()
	c.chunk.emit32(OpSetLocal, packVar(0, tmp), line)
	c.chunk.emit(OpPop, line)
	target := vd.Decls[0].Target
	hidden := &ast.Ident{Base: ast.Base{P: vd.Pos()}, Name: "\x00t"}
	fn := &ast.FunctionExpr{
		Base:   ast.Base{P: vd.Pos()},
		Params: []ast.Expr{hidden},
		Body: &ast.BlockStmt{Base: ast.Base{P: vd.Pos()}, Body: []ast.Stmt{
			&ast.VarDecl{Base: vd.Base, DeclKind: "var", Decls: []*ast.VarDeclarator{
				{Base: vd.Base, Target: target, Init: hidden},
			}},
			body,
		}},
	}
	ch := c.compileFunction(fn, "", false)
	c.chunk.emit32(OpConst, c.chunk.addConst(Const{Kind: ConstFunction, Fn: ch}), line)
	c.chunk.emit32(OpGetLocal, packVar(0, tmp), line)
	c.chunk.emit16(OpCall, 1, line)
	c.chunk.emit(OpPop, line)
}

func (c *Compiler) compileArrayDestructure(p *ast.ArrayPattern, isDecl bool) {
	line := c.line(p)
	iterSlot := c.allocLocal()
	doneSlot := c.allocLocal()
	c.chunk.emit(OpGetIterator, line)
	c.chunk.emit32(OpSetLocal, packVar(0, iterSlot), line)
	c.chunk.emit(OpPop, line)
	c.chunk.emit(OpFalse, line)
	c.chunk.emit32(OpSetLocal, packVar(0, doneSlot), line)
	c.chunk.emit(OpPop, line)

	hasRest := false
	for _, el := range p.Elements {
		if _, ok := el.(*ast.RestElement); ok {
			hasRest = true
			break
		}
	}
	wrapClose := (c.inGenerator && exprHasYield(p)) || exprHasMember(p)
	tryStart := 0
	if wrapClose {
		tryStart = len(c.chunk.Code)
	}
	for _, el := range p.Elements {
		if rest, ok := el.(*ast.RestElement); ok {
			c.emitIteratorRest(iterSlot, doneSlot, rest, isDecl, line)
			continue
		}
		if mem, ok := el.(*ast.MemberExpr); ok {
			objTmp := c.allocLocal()
			keyTmp := c.allocLocal()
			c.expr(mem.Object)
			c.chunk.emit32(OpSetLocal, packVar(0, objTmp), line)
			c.chunk.emit(OpPop, line)
			if mem.Computed {
				c.expr(mem.Property)
			} else if id, ok := mem.Property.(*ast.Ident); ok {
				c.chunk.emit32(OpConst, c.constString(id.Name), line)
			} else {
				c.fail(mem, "nom de propriété non compilé")
			}
			c.chunk.emit32(OpSetLocal, packVar(0, keyTmp), line)
			c.chunk.emit(OpPop, line)
			c.chunk.emit32(OpGetLocal, packVar(0, objTmp), line)
			c.chunk.emit32(OpGetLocal, packVar(0, keyTmp), line)
			c.emitIteratorNextValue(iterSlot, doneSlot, line)
			c.chunk.emit(OpSetElem, line)
			c.chunk.emit(OpPop, line)
			continue
		}
		c.emitIteratorNextValue(iterSlot, doneSlot, line)
		if el == nil {
			c.chunk.emit(OpPop, line)
			continue
		}
		c.compileDestructuring(el, isDecl)
	}
	if !hasRest {
		c.chunk.emit32(OpGetLocal, packVar(0, doneSlot), line)
		skip := c.chunk.emit32(OpJumpIfTrue, 0, line)
		c.emitIteratorClose(iterSlot, line, true)
		c.chunk.patch32(skip, uint32(int32(len(c.chunk.Code)-(skip+5))))
	}
	if wrapClose {
		tryEnd := len(c.chunk.Code)
		jumpOver := c.chunk.emit32(OpJump, 0, line)
		catchIP := len(c.chunk.Code)
		excSlot := c.allocLocal()
		c.chunk.emit32(OpSetLocal, packVar(0, excSlot), line)
		c.chunk.emit(OpPop, line)
		c.chunk.emit32(OpGetLocal, packVar(0, doneSlot), line)
		skipDone := c.chunk.emit32(OpJumpIfTrue, 0, line)
		c.emitIteratorClose(iterSlot, line, c.inGenerator && exprHasYield(p))
		c.chunk.patch32(skipDone, uint32(int32(len(c.chunk.Code)-(skipDone+5))))
		c.chunk.emit32(OpGetLocal, packVar(0, excSlot), line)
		c.chunk.emit(OpThrow, line)
		c.chunk.Handlers = append(c.chunk.Handlers, ExceptionHandler{
			Start:     tryStart,
			End:       tryEnd,
			CatchIP:   catchIP,
			FinallyIP: -1,
			CatchSlot: -1,
		})
		c.chunk.patch32(jumpOver, uint32(int32(len(c.chunk.Code)-(jumpOver+5))))
	}
}

func exprHasMember(e ast.Expr) bool {
	if e == nil {
		return false
	}
	switch t := e.(type) {
	case *ast.MemberExpr:
		return true
	case *ast.ArrayPattern:
		for _, el := range t.Elements {
			if exprHasMember(el) {
				return true
			}
		}
	case *ast.AssignPattern:
		return exprHasMember(t.Target) || exprHasMember(t.Default)
	case *ast.RestElement:
		return exprHasMember(t.Argument)
	}
	return false
}

func exprHasYield(e ast.Expr) bool {
	if e == nil {
		return false
	}
	switch t := e.(type) {
	case *ast.YieldExpr:
		return true
	case *ast.ArrayPattern:
		for _, el := range t.Elements {
			if exprHasYield(el) {
				return true
			}
		}
	case *ast.ObjectPattern:
		for _, p := range t.Properties {
			if p != nil && (exprHasYield(p.Value) || exprHasYield(p.Key)) {
				return true
			}
		}
	case *ast.AssignPattern:
		return exprHasYield(t.Target) || exprHasYield(t.Default)
	case *ast.RestElement:
		return exprHasYield(t.Argument)
	case *ast.MemberExpr:
		return exprHasYield(t.Object) || exprHasYield(t.Property)
	case *ast.UnaryExpr:
		return exprHasYield(t.Operand)
	case *ast.BinaryExpr:
		return exprHasYield(t.Left) || exprHasYield(t.Right)
	case *ast.AssignExpr:
		return exprHasYield(t.Target) || exprHasYield(t.Value)
	case *ast.ConditionalExpr:
		return exprHasYield(t.Test) || exprHasYield(t.Consequent) || exprHasYield(t.Alternate)
	case *ast.LogicalExpr:
		return exprHasYield(t.Left) || exprHasYield(t.Right)
	case *ast.SpreadElement:
		return exprHasYield(t.Argument)
	case *ast.CallExpr:
		if exprHasYield(t.Callee) {
			return true
		}
		for _, a := range t.Args {
			if exprHasYield(a) {
				return true
			}
		}
	case *ast.ArrayLit:
		for _, el := range t.Elements {
			if exprHasYield(el) {
				return true
			}
		}
	case *ast.SequenceExpr:
		for _, el := range t.Exprs {
			if exprHasYield(el) {
				return true
			}
		}
	}
	return false
}

func (c *Compiler) emitIteratorNextValue(iterSlot, doneSlot, line int) {
	c.chunk.emit32(OpGetLocal, packVar(0, iterSlot), line)
	c.chunk.emit32(OpCallMethod, packVar(0, int(c.constString("next"))), line)
	c.chunk.emit(OpDup, line)
	c.chunk.emit32(OpGetProp, c.constString("done"), line)
	c.chunk.emit32(OpSetLocal, packVar(0, doneSlot), line)
	jf := c.chunk.emit32(OpJumpIfTrue, 0, line)
	c.chunk.emit32(OpGetProp, c.constString("value"), line)
	jmp := c.chunk.emit32(OpJump, 0, line)
	c.chunk.patch32(jf, uint32(int32(len(c.chunk.Code)-(jf+5))))
	c.chunk.emit(OpPop, line)
	c.chunk.emit(OpUndefined, line)
	c.chunk.patch32(jmp, uint32(int32(len(c.chunk.Code)-(jmp+5))))
}

func (c *Compiler) emitIteratorRest(iterSlot, doneSlot int, rest *ast.RestElement, isDecl bool, line int) {
	if mem, ok := rest.Argument.(*ast.MemberExpr); ok {
		objTmp := c.allocLocal()
		keyTmp := c.allocLocal()
		arrTmp := c.allocLocal()
		c.expr(mem.Object)
		c.chunk.emit32(OpSetLocal, packVar(0, objTmp), line)
		c.chunk.emit(OpPop, line)
		if mem.Computed {
			c.expr(mem.Property)
		} else if id, ok := mem.Property.(*ast.Ident); ok {
			c.chunk.emit32(OpConst, c.constString(id.Name), line)
		} else {
			c.fail(mem, "nom de propriété non compilé")
		}
		c.chunk.emit32(OpSetLocal, packVar(0, keyTmp), line)
		c.chunk.emit(OpPop, line)
		c.chunk.emit32(OpNewArray, 0, line)
		loop := len(c.chunk.Code)
		c.chunk.emit32(OpGetLocal, packVar(0, doneSlot), line)
		end := c.chunk.emit32(OpJumpIfTrue, 0, line)
		c.chunk.emit32(OpGetLocal, packVar(0, iterSlot), line)
		c.chunk.emit32(OpCallMethod, packVar(0, int(c.constString("next"))), line)
		c.chunk.emit(OpDup, line)
		c.chunk.emit32(OpGetProp, c.constString("done"), line)
		c.chunk.emit32(OpSetLocal, packVar(0, doneSlot), line)
		endPop := c.chunk.emit32(OpJumpIfTrue, 0, line)
		c.chunk.emit32(OpGetProp, c.constString("value"), line)
		c.chunk.emit(OpArrayPush, line)
		back := c.chunk.emit32(OpJump, 0, line)
		c.chunk.patch32(back, uint32(int32(loop-(back+5))))
		c.chunk.patch32(endPop, uint32(int32(len(c.chunk.Code)-(endPop+5))))
		c.chunk.emit(OpPop, line)
		c.chunk.patch32(end, uint32(int32(len(c.chunk.Code)-(end+5))))
		c.chunk.emit32(OpSetLocal, packVar(0, arrTmp), line)
		c.chunk.emit(OpPop, line)
		c.chunk.emit32(OpGetLocal, packVar(0, objTmp), line)
		c.chunk.emit32(OpGetLocal, packVar(0, keyTmp), line)
		c.chunk.emit32(OpGetLocal, packVar(0, arrTmp), line)
		c.chunk.emit(OpSetElem, line)
		c.chunk.emit(OpPop, line)
		return
	}
	c.chunk.emit32(OpNewArray, 0, line)
	loop := len(c.chunk.Code)
	c.chunk.emit32(OpGetLocal, packVar(0, doneSlot), line)
	end := c.chunk.emit32(OpJumpIfTrue, 0, line)
	c.chunk.emit32(OpGetLocal, packVar(0, iterSlot), line)
	c.chunk.emit32(OpCallMethod, packVar(0, int(c.constString("next"))), line)
	c.chunk.emit(OpDup, line)
	c.chunk.emit32(OpGetProp, c.constString("done"), line)
	c.chunk.emit32(OpSetLocal, packVar(0, doneSlot), line)
	endPop := c.chunk.emit32(OpJumpIfTrue, 0, line)
	c.chunk.emit32(OpGetProp, c.constString("value"), line)
	c.chunk.emit(OpArrayPush, line)
	back := c.chunk.emit32(OpJump, 0, line)
	c.chunk.patch32(back, uint32(int32(loop-(back+5))))
	c.chunk.patch32(endPop, uint32(int32(len(c.chunk.Code)-(endPop+5))))
	c.chunk.emit(OpPop, line)
	c.chunk.patch32(end, uint32(int32(len(c.chunk.Code)-(end+5))))
	c.compileDestructuring(rest.Argument, isDecl)
}

func (c *Compiler) emitIteratorClose(iterSlot, line int, checkResult bool) {
	c.chunk.emit32(OpGetLocal, packVar(0, iterSlot), line)
	skipAll := -1
	if !checkResult {
		ts := len(c.chunk.Code)
		c.chunk.emit32(OpGetProp, c.constString("return"), line)
		c.chunk.emit(OpDup, line)
		c.chunk.emit(OpTypeof, line)
		c.chunk.emit32(OpConst, c.constString("function"), line)
		c.chunk.emit(OpStrictEq, line)
		skipInner := c.chunk.emit32(OpJumpIfFalse, 0, line)
		c.chunk.emit(OpPop, line)
		c.chunk.emit32(OpGetLocal, packVar(0, iterSlot), line)
		c.chunk.emit32(OpCallMethod, packVar(0, int(c.constString("return"))), line)
		c.chunk.emit(OpPop, line)
		te := len(c.chunk.Code)
		over := c.chunk.emit32(OpJump, 0, line)
		c.chunk.patch32(skipInner, uint32(int32(len(c.chunk.Code)-(skipInner+5))))
		c.chunk.emit(OpPop, line)
		endClose := c.chunk.emit32(OpJump, 0, line)
		catchIP := len(c.chunk.Code)
		c.chunk.emit(OpPop, line)
		c.chunk.Handlers = append(c.chunk.Handlers, ExceptionHandler{
			Start: ts, End: te, CatchIP: catchIP, FinallyIP: -1, CatchSlot: -1,
		})
		c.chunk.patch32(over, uint32(int32(len(c.chunk.Code)-(over+5))))
		c.chunk.patch32(endClose, uint32(int32(len(c.chunk.Code)-(endClose+5))))
		return
	} else {
		c.chunk.emit32(OpGetProp, c.constString("return"), line)
	}
	c.chunk.emit(OpDup, line)
	c.chunk.emit(OpIsNullish, line)
	skipCall := c.chunk.emit32(OpJumpIfTrue, 0, line)
	c.chunk.emit(OpDup, line)
	c.chunk.emit(OpTypeof, line)
	c.chunk.emit32(OpConst, c.constString("function"), line)
	c.chunk.emit(OpStrictEq, line)
	jCall := c.chunk.emit32(OpJumpIfTrue, 0, line)
	c.chunk.emit32(OpGetGlobal, c.constString("TypeError"), line)
	c.chunk.emit32(OpConst, c.constString("return is not a function"), line)
	c.chunk.emit16(OpNew, 1, line)
	c.chunk.emit(OpThrow, line)
	c.chunk.patch32(jCall, uint32(int32(len(c.chunk.Code)-(jCall+5))))
	c.chunk.emit(OpPop, line)
	c.chunk.emit32(OpGetLocal, packVar(0, iterSlot), line)
	c.chunk.emit32(OpCallMethod, packVar(0, int(c.constString("return"))), line)
	if !checkResult {
		c.chunk.emit(OpPop, line)
		doneClose := c.chunk.emit32(OpJump, 0, line)
		c.chunk.patch32(skipCall, uint32(int32(len(c.chunk.Code)-(skipCall+5))))
		c.chunk.emit(OpPop, line)
		c.chunk.patch32(doneClose, uint32(int32(len(c.chunk.Code)-(doneClose+5))))
		if skipAll >= 0 {
			c.chunk.patch32(skipAll, uint32(int32(len(c.chunk.Code)-(skipAll+5))))
		}
		return
	}
	c.chunk.emit(OpDup, line)
	c.chunk.emit(OpIsNullish, line)
	jThrow := c.chunk.emit32(OpJumpIfTrue, 0, line)
	c.chunk.emit(OpDup, line)
	c.chunk.emit(OpTypeof, line)
	c.chunk.emit32(OpConst, c.constString("object"), line)
	c.chunk.emit(OpStrictEq, line)
	jOk := c.chunk.emit32(OpJumpIfTrue, 0, line)
	c.chunk.emit(OpDup, line)
	c.chunk.emit(OpTypeof, line)
	c.chunk.emit32(OpConst, c.constString("function"), line)
	c.chunk.emit(OpStrictEq, line)
	jOk2 := c.chunk.emit32(OpJumpIfTrue, 0, line)
	c.chunk.patch32(jThrow, uint32(int32(len(c.chunk.Code)-(jThrow+5))))
	c.chunk.emit(OpPop, line)
	c.chunk.emit32(OpGetGlobal, c.constString("TypeError"), line)
	c.chunk.emit32(OpConst, c.constString("Iterator result is not an object"), line)
	c.chunk.emit16(OpNew, 1, line)
	c.chunk.emit(OpThrow, line)
	okIP := len(c.chunk.Code)
	c.chunk.patch32(jOk, uint32(int32(okIP-(jOk+5))))
	c.chunk.patch32(jOk2, uint32(int32(okIP-(jOk2+5))))
	c.chunk.emit(OpPop, line)
	doneClose := c.chunk.emit32(OpJump, 0, line)
	c.chunk.patch32(skipCall, uint32(int32(len(c.chunk.Code)-(skipCall+5))))
	c.chunk.emit(OpPop, line)
	c.chunk.patch32(doneClose, uint32(int32(len(c.chunk.Code)-(doneClose+5))))
}

func forOfPatternDecl(vd *ast.VarDecl) bool {
	if vd == nil || len(vd.Decls) == 0 {
		return false
	}
	switch vd.Decls[0].Target.(type) {
	case *ast.ArrayPattern, *ast.ObjectPattern:
		return true
	}
	return false
}

func (c *Compiler) storeLoopLeft(left ast.Node, line int) {
	switch t := left.(type) {
	case *ast.VarDecl:
		if len(t.Decls) == 0 {
			c.fail(t, "for-in/for-of sans cible")
		}
		switch target := t.Decls[0].Target.(type) {
		case *ast.Ident:
			if c.scope != nil {
				slot := c.scope.declare(target.Name)
				c.chunk.emit32(OpSetLocal, packVar(0, slot), line)
				c.chunk.emit(OpPop, line)
			} else {
				c.chunk.emit32(OpDefGlobal, c.constString(target.Name), line)
			}
		case *ast.ArrayPattern:
			c.compileDestructuring(target, true)
		case *ast.ObjectPattern:
			c.compileDestructuring(target, true)
		default:
			c.fail(t, "cible for-in/for-of non compilée")
		}
	case *ast.Ident:
		c.storeIdent(t)
		c.chunk.emit(OpPop, line)
	case *ast.ArrayPattern:
		c.compileDestructuring(t, false)
	case *ast.ObjectPattern:
		c.compileDestructuring(t, false)
	case *ast.MemberExpr:
		c.compileDestructuring(t, false)
	default:
		c.fail(left, "cible for-in/for-of non compilée")
	}
}

func (c *Compiler) callHasSpread(args []ast.Expr) bool {
	for _, a := range args {
		if _, ok := a.(*ast.SpreadElement); ok {
			return true
		}
	}
	return false
}

func (c *Compiler) arrayHasSpread(els []ast.Expr) bool {
	for _, el := range els {
		if _, ok := el.(*ast.SpreadElement); ok {
			return true
		}
	}
	return false
}

func (c *Compiler) compileSpreadArgArray(args []ast.Expr, line int) {
	c.chunk.emit32(OpNewArray, 0, line)
	for _, a := range args {
		if sp, ok := a.(*ast.SpreadElement); ok {
			c.expr(sp.Argument)
			c.chunk.emit(OpArraySpread, line)
			continue
		}
		c.expr(a)
		c.chunk.emit(OpArrayPush, line)
	}
}

func (c *Compiler) compileOptionalMember(x *ast.MemberExpr) {
	line := c.line(x)
	c.expr(x.Object)
	c.chunk.emit(OpDup, line)
	c.chunk.emit(OpIsNullish, line)
	js := c.chunk.emit32(OpJumpIfTrue, 0, line)
	if x.Computed {
		c.expr(x.Property)
		c.chunk.emit(OpGetElem, line)
	} else {
		id, ok := x.Property.(*ast.Ident)
		if !ok {
			c.fail(x, "nom de propriété non compilé")
		}
		c.chunk.emit32(OpGetProp, c.constString(id.Name), line)
	}
	je := c.chunk.emit32(OpJump, 0, line)
	c.chunk.patch32(js, uint32(int32(len(c.chunk.Code)-(js+5))))
	c.chunk.emit(OpPop, line)
	c.chunk.emit(OpUndefined, line)
	c.chunk.patch32(je, uint32(int32(len(c.chunk.Code)-(je+5))))
}

func (c *Compiler) compileOptionalCall(x *ast.CallExpr) {
	line := c.line(x)
	c.expr(x.Callee)
	c.chunk.emit(OpDup, line)
	c.chunk.emit(OpIsNullish, line)
	js := c.chunk.emit32(OpJumpIfTrue, 0, line)
	if c.callHasSpread(x.Args) {
		c.compileSpreadArgArray(x.Args, line)
		c.chunk.emit(OpCallSpread, line)
	} else {
		for _, a := range x.Args {
			c.expr(a)
		}
		c.chunk.emit16(OpCall, uint16(len(x.Args)), line)
	}
	je := c.chunk.emit32(OpJump, 0, line)
	c.chunk.patch32(js, uint32(int32(len(c.chunk.Code)-(js+5))))
	c.chunk.emit(OpPop, line)
	c.chunk.emit(OpUndefined, line)
	c.chunk.patch32(je, uint32(int32(len(c.chunk.Code)-(je+5))))
}

func (c *Compiler) pushLoop() *loopCtx {
	lp := &loopCtx{continueTarget: -1, cptnSlot: -1, iterSlot: -1, label: c.pendingLabel}
	c.pendingLabel = ""
	c.loops = append(c.loops, lp)
	return lp
}

func (c *Compiler) initLoopCptn(lp *loopCtx, keepValue bool, line int) {
	if !keepValue {
		return
	}
	lp.cptnSlot = c.allocLocal()
	c.chunk.emit(OpUndefined, line)
	c.chunk.emit32(OpSetLocal, packVar(0, lp.cptnSlot), line)
	c.chunk.emit(OpPop, line)
}

func (c *Compiler) emitLoopCptn(lp *loopCtx, keepValue bool, line int) {
	if !keepValue {
		return
	}
	if lp.cptnSlot >= 0 {
		c.chunk.emit32(OpGetLocal, packVar(0, lp.cptnSlot), line)
		return
	}
	c.chunk.emit(OpUndefined, line)
}

func (c *Compiler) closeForOfUntil(target *loopCtx, n ast.Node) {
	for i := len(c.loops) - 1; i >= 0; i-- {
		cur := c.loops[i]
		if target != nil && cur == target {
			return
		}
		if cur.iterSlot >= 0 {
			c.emitIteratorClose(cur.iterSlot, c.line(n), true)
		}
	}
}

func (c *Compiler) innermostCptn() int {
	for i := len(c.loops) - 1; i >= 0; i-- {
		if c.loops[i].cptnSlot >= 0 {
			return c.loops[i].cptnSlot
		}
	}
	return -1
}

func (c *Compiler) findBreakTarget(lab *ast.Ident) *loopCtx {
	if lab == nil {
		if len(c.loops) == 0 {
			return nil
		}
		return c.loops[len(c.loops)-1]
	}
	for i := len(c.loops) - 1; i >= 0; i-- {
		if c.loops[i].label == lab.Name {
			return c.loops[i]
		}
	}
	return nil
}

func (c *Compiler) findContinueTarget(lab *ast.Ident) *loopCtx {
	if lab == nil {
		for i := len(c.loops) - 1; i >= 0; i-- {
			if !c.loops[i].isSwitch && !c.loops[i].isLabel {
				return c.loops[i]
			}
		}
		return nil
	}
	for i := len(c.loops) - 1; i >= 0; i-- {
		if c.loops[i].label == lab.Name && !c.loops[i].isSwitch && !c.loops[i].isLabel {
			return c.loops[i]
		}
	}
	return nil
}

func (c *Compiler) popLoop(lp *loopCtx, breakTarget int) {
	c.loops = c.loops[:len(c.loops)-1]
	for _, at := range lp.breaks {
		c.chunk.patch32(at, uint32(int32(breakTarget-(at+5))))
	}
	target := lp.continueTarget
	if target < 0 {
		target = breakTarget
	}
	for _, at := range lp.continues {
		c.chunk.patch32(at, uint32(int32(target-(at+5))))
	}
}

func (c *Compiler) varDecl(d *ast.VarDecl) {
	for _, dec := range d.Decls {
		switch target := dec.Target.(type) {
		case *ast.Ident:
			if d.DeclKind == "const" && c.scope == nil {
				if c.constNames == nil {
					c.constNames = map[string]bool{}
				}
				c.constNames[target.Name] = true
			}
			if c.iterIIFE && d.DeclKind == "var" && !c.iterBind[target.Name] {
				if c.scope != nil && c.scope.parent != nil {
					if _, _, ok := func() (int, int, bool) {
						d := 0
						for s := c.scope.parent; s != nil; s = s.parent {
							if sl, found := s.names[target.Name]; found {
								return d, sl, true
							}
							d++
						}
						return 0, 0, false
					}(); ok {
						if dec.Init != nil {
							c.exprNamed(dec.Init, target.Name)
						} else {
							c.chunk.emit(OpUndefined, c.line(dec))
						}
						c.storeIdent(target)
						c.chunk.emit(OpPop, c.line(dec))
						continue
					}
				} else if c.declaredVars[target.Name] {
					if dec.Init != nil {
						c.exprNamed(dec.Init, target.Name)
					} else {
						continue
					}
					c.chunk.emit32(OpSetGlobal, c.constString(target.Name), c.line(dec))
					c.chunk.emit(OpPop, c.line(dec))
					continue
				}
			}
			if dec.Init != nil {
				c.exprNamed(dec.Init, target.Name)
			} else if d.DeclKind == "var" {
				if c.scope != nil {
					c.scope.declare(target.Name)
					continue
				}
				if c.declaredVars[target.Name] {
					continue
				}
				if c.declaredVars == nil {
					c.declaredVars = map[string]bool{}
				}
				c.declaredVars[target.Name] = true
				c.chunk.emit(OpUndefined, c.line(dec))
				c.chunk.emit32(OpDefGlobal, c.constString(target.Name), c.line(dec))
				continue
			} else {
				c.chunk.emit(OpUndefined, c.line(dec))
			}
			if c.scope == nil {
				if c.declaredVars == nil {
					c.declaredVars = map[string]bool{}
				}
				c.declaredVars[target.Name] = true
				c.chunk.emit32(OpDefGlobal, c.constString(target.Name), c.line(dec))
			} else {
				slot := c.scope.declare(target.Name)
				c.chunk.emit32(OpSetLocal, packVar(0, slot), c.line(dec))
				c.chunk.emit(OpPop, c.line(dec))
			}
		case *ast.ArrayPattern, *ast.ObjectPattern:
			if dec.Init == nil {
				c.fail(dec, "initialiseur manquant dans la déclaration de décomposition")
			}
			c.expr(dec.Init)
			c.compileDestructuring(target, true)
		default:
			c.fail(dec, "cible de déclaration non compilée : "+dec.Target.NodeType())
		}
	}
}

func (c *Compiler) compileDestructuring(pattern ast.Expr, isDecl bool) {
	switch p := pattern.(type) {
	case *ast.Ident:
		if isDecl {
			if c.scope != nil {
				slot := c.scope.declare(p.Name)
				c.chunk.emit32(OpSetLocal, packVar(0, slot), c.line(p))
				c.chunk.emit(OpPop, c.line(p))
			} else {
				c.chunk.emit32(OpDefGlobal, c.constString(p.Name), c.line(p))
			}
		} else {
			c.storeIdent(p)
			c.chunk.emit(OpPop, c.line(p))
		}

	case *ast.AssignPattern:
		if p.Default != nil {
			c.chunk.emit(OpDup, c.line(p.Default))
			c.chunk.emit(OpUndefined, c.line(p.Default))
			c.chunk.emit(OpStrictNe, c.line(p.Default))
			jmp := c.chunk.emit32(OpJumpIfTrue, 0, c.line(p.Default))
			c.chunk.emit(OpPop, c.line(p.Default))
			saved := c.pendingFuncName
			if id, ok := p.Target.(*ast.Ident); ok && isAnonymousFnDef(p.Default) {
				c.pendingFuncName = id.Name
			}
			c.expr(p.Default)
			c.pendingFuncName = saved
			c.chunk.patch32(jmp, uint32(int32(len(c.chunk.Code)-(jmp+5))))
		}
		c.compileDestructuring(p.Target, isDecl)

	case *ast.ArrayPattern:
		c.compileArrayDestructure(p, isDecl)

	case *ast.ObjectPattern:
		c.chunk.emit(OpDup, c.line(p))
		c.chunk.emit(OpIsNullish, c.line(p))
		jOk := c.chunk.emit32(OpJumpIfFalse, 0, c.line(p))
		c.chunk.emit32(OpGetGlobal, c.constString("TypeError"), c.line(p))
		c.chunk.emit32(OpConst, c.constString("Cannot destructure null or undefined"), c.line(p))
		c.chunk.emit16(OpNew, 1, c.line(p))
		c.chunk.emit(OpThrow, c.line(p))
		c.chunk.patch32(jOk, uint32(int32(len(c.chunk.Code)-(jOk+5))))
		var rest *ast.Property
		skipSlot := -1
		for _, prop := range p.Properties {
			if prop.Kind == "rest" || prop.Kind == "spread" {
				rest = prop
				break
			}
		}
		if rest != nil {
			skipSlot = c.allocLocal()
			c.chunk.emit32(OpNewArray, 0, c.line(p))
			c.chunk.emit32(OpSetLocal, packVar(0, skipSlot), c.line(p))
			c.chunk.emit(OpPop, c.line(p))
		}
		for _, prop := range p.Properties {
			if prop.Kind == "rest" || prop.Kind == "spread" {
				continue
			}
			c.chunk.emit(OpDup, c.line(prop))
			if prop.Computed {
				c.expr(prop.Key)
				if skipSlot >= 0 {
					keyTmp := c.allocLocal()
					c.chunk.emit32(OpSetLocal, packVar(0, keyTmp), c.line(prop))
					c.chunk.emit(OpPop, c.line(prop))
					c.chunk.emit32(OpGetLocal, packVar(0, skipSlot), c.line(prop))
					c.chunk.emit32(OpGetLocal, packVar(0, keyTmp), c.line(prop))
					c.chunk.emit(OpArrayPush, c.line(prop))
					c.chunk.emit(OpPop, c.line(prop))
					c.chunk.emit32(OpGetLocal, packVar(0, keyTmp), c.line(prop))
				}
				c.chunk.emit(OpGetElem, c.line(prop))
			} else {
				name := propertyName(prop.Key)
				if name == "" {
					c.fail(prop, "nom de propriété non compilé")
				}
				if skipSlot >= 0 {
					c.chunk.emit32(OpGetLocal, packVar(0, skipSlot), c.line(prop))
					c.chunk.emit32(OpConst, c.constString(name), c.line(prop))
					c.chunk.emit(OpArrayPush, c.line(prop))
					c.chunk.emit(OpPop, c.line(prop))
				}
				c.chunk.emit32(OpGetProp, c.constString(name), c.line(prop))
			}
			c.compileDestructuring(prop.Value, isDecl)
		}
		if rest != nil {
			c.chunk.emit(OpDup, c.line(rest))
			c.chunk.emit32(OpGetLocal, packVar(0, skipSlot), c.line(rest))
			c.chunk.emit(OpObjectRest, c.line(rest))
			arg := rest.Value
			if arg == nil {
				c.fail(rest, "reste d'objet sans cible")
			}
			c.compileDestructuring(arg, isDecl)
		}
		c.chunk.emit(OpPop, c.line(p))

	case *ast.RestElement:
		c.compileDestructuring(p.Argument, isDecl)

	case *ast.MemberExpr:
		slot := c.allocLocal()
		c.chunk.emit32(OpSetLocal, packVar(0, slot), c.line(p))
		c.chunk.emit(OpPop, c.line(p))
		c.expr(p.Object)
		if p.Computed {
			c.expr(p.Property)
			c.chunk.emit32(OpGetLocal, packVar(0, slot), c.line(p))
			c.chunk.emit(OpSetElem, c.line(p))
			c.chunk.emit(OpPop, c.line(p))
		} else {
			id, ok := p.Property.(*ast.Ident)
			if !ok {
				c.fail(p, "nom de propriété non compilé")
			}
			c.chunk.emit32(OpGetLocal, packVar(0, slot), c.line(p))
			c.chunk.emit32(OpSetProp, c.constString(id.Name), c.line(p))
			c.chunk.emit(OpPop, c.line(p))
		}

	default:
		c.fail(pattern, "cible de décomposition non compilée : "+pattern.NodeType())
	}
}

// ─── Expressions ────────────────────────────────────────────────────────────

func (c *Compiler) expr(e ast.Expr) {
	switch x := e.(type) {

	case *ast.NumberLit:
		f := parseNumber(x.Raw)
		if f == float64(int32(f)) && f != 0 || f == 0 {
			c.chunk.emit32(OpInt32, uint32(int32(f)), c.line(x))
			return
		}
		c.chunk.emit32(OpConst, c.constNumber(f), c.line(x))

	case *ast.StringLit:
		raw := x.Raw
		inner := raw
		if len(raw) >= 2 {
			inner = raw[1 : len(raw)-1]
		}
		c.chunk.emit32(OpConst, c.chunk.addConst(Const{Kind: ConstString, Text: c.heap.Intern().Intern(unescapeToUTF16(inner))}), c.line(x))

	case *ast.BoolLit:
		if x.Value {
			c.chunk.emit(OpTrue, c.line(x))
		} else {
			c.chunk.emit(OpFalse, c.line(x))
		}

	case *ast.NullLit:
		c.chunk.emit(OpNull, c.line(x))

	case *ast.Ident:
		if x.Name == "undefined" && !c.tdzHead["undefined"] {
			c.chunk.emit(OpUndefined, c.line(x))
			return
		}
		c.loadIdent(x)

	case *ast.TemplateLit:
		// Un gabarit se compile en concaténations successives. Les fragments
		// portent leurs délimiteurs ; ils sont retirés ici.
		c.chunk.emit32(OpConst, c.constString(templateChunk(x.Quasis[0])), c.line(x))
		for i, sub := range x.Exprs {
			c.expr(sub)
			c.chunk.emit(OpAdd, c.line(x))
			c.chunk.emit32(OpConst, c.constString(templateChunk(x.Quasis[i+1])), c.line(x))
			c.chunk.emit(OpAdd, c.line(x))
		}

	case *ast.UnaryExpr:
		switch x.Op {
		case "-":
			c.expr(x.Operand)
			c.chunk.emit(OpNeg, c.line(x))
		case "+":
			c.expr(x.Operand)
			c.chunk.emit(OpPos, c.line(x))
		case "!":
			c.expr(x.Operand)
			c.chunk.emit(OpNot, c.line(x))
		case "~":
			c.expr(x.Operand)
			c.chunk.emit(OpBitNot, c.line(x))
		case "typeof":
			if id, ok := x.Operand.(*ast.Ident); ok {
				if c.tdzHead[id.Name] {
					c.loadIdent(id)
				} else if _, _, ok := c.resolve(id.Name); ok {
					c.loadIdent(id)
				} else {
					c.chunk.emit32(OpGetGlobalSoft, c.constString(id.Name), c.line(x))
				}
			} else {
				c.expr(x.Operand)
			}
			c.chunk.emit(OpTypeof, c.line(x))
		case "void":
			c.expr(x.Operand)
			c.chunk.emit(OpPop, c.line(x))
			c.chunk.emit(OpUndefined, c.line(x))
		case "delete":
			switch t := x.Operand.(type) {
			case *ast.Ident:
				c.chunk.emit32(OpDelete, (1<<31)|c.constString(t.Name), c.line(x))
			case *ast.MemberExpr:
				c.expr(t.Object)
				if t.Computed {
					c.expr(t.Property)
					c.chunk.emit32(OpDelete, 0, c.line(x))
					return
				}
				id, ok := t.Property.(*ast.Ident)
				if !ok {
					c.fail(x, "delete : nom de propriété non compilé")
				}
				c.chunk.emit32(OpDelete, c.constString(id.Name)+1, c.line(x))
			default:
				c.expr(x.Operand)
				c.chunk.emit(OpPop, c.line(x))
				c.chunk.emit(OpTrue, c.line(x))
			}
		default:
			c.fail(x, "opérateur unaire non compilé : "+x.Op)
		}

	case *ast.UpdateExpr:
		delta := int32(1)
		if x.Op == "--" {
			delta = -1
		}
		switch opnd := x.Operand.(type) {
		case *ast.Ident:
			c.loadIdent(opnd)
			if !x.Prefix {
				c.chunk.emit(OpDup, c.line(x))
			}
			c.chunk.emit32(OpInt32, uint32(delta), c.line(x))
			c.chunk.emit(OpAdd, c.line(x))
			c.storeIdent(opnd)
			if x.Prefix {
				return
			}
			c.chunk.emit(OpPop, c.line(x))
		case *ast.MemberExpr:
			if opnd.Computed {
				objSlot := c.allocLocal()
				keySlot := c.allocLocal()
				c.expr(opnd.Object)
				c.chunk.emit32(OpSetLocal, packVar(0, objSlot), c.line(x))
				c.chunk.emit(OpPop, c.line(x))
				c.expr(opnd.Property)
				c.chunk.emit32(OpSetLocal, packVar(0, keySlot), c.line(x))
				c.chunk.emit(OpPop, c.line(x))
				c.chunk.emit32(OpGetLocal, packVar(0, objSlot), c.line(x))
				c.chunk.emit32(OpGetLocal, packVar(0, keySlot), c.line(x))
				c.chunk.emit(OpGetElem, c.line(x))
				if !x.Prefix {
					c.chunk.emit(OpDup, c.line(x))
				}
				c.chunk.emit32(OpInt32, uint32(delta), c.line(x))
				c.chunk.emit(OpAdd, c.line(x))
				valSlot := c.allocLocal()
				c.chunk.emit32(OpSetLocal, packVar(0, valSlot), c.line(x))
				c.chunk.emit(OpPop, c.line(x))
				c.chunk.emit32(OpGetLocal, packVar(0, objSlot), c.line(x))
				c.chunk.emit32(OpGetLocal, packVar(0, keySlot), c.line(x))
				c.chunk.emit32(OpGetLocal, packVar(0, valSlot), c.line(x))
				c.chunk.emit(OpSetElem, c.line(x))
				c.chunk.emit(OpPop, c.line(x))
				if x.Prefix {
					c.chunk.emit32(OpGetLocal, packVar(0, valSlot), c.line(x))
				}
				return
			}
			id, ok := opnd.Property.(*ast.Ident)
			if !ok {
				c.fail(x, "nom de propriété non compilé")
			}
			c.expr(opnd.Object)
			c.chunk.emit(OpDup, c.line(x))
			c.chunk.emit32(OpGetProp, c.constString(id.Name), c.line(x))
			c.chunk.emit32(OpInt32, uint32(delta), c.line(x))
			c.chunk.emit(OpAdd, c.line(x))
			c.chunk.emit32(OpSetProp, c.constString(id.Name), c.line(x))
			if !x.Prefix {
				c.chunk.emit32(OpInt32, uint32(-delta), c.line(x))
				c.chunk.emit(OpAdd, c.line(x))
			}
		default:
			c.fail(x, "la mise à jour ne porte pour l'instant que sur un identifiant ou une propriété")
		}

	case *ast.BinaryExpr:
		c.expr(x.Left)
		c.expr(x.Right)
		op, ok := binaryOps[x.Op]
		if !ok {
			c.fail(x, "opérateur binaire non compilé : "+x.Op)
		}
		c.chunk.emit(op, c.line(x))

	case *ast.LogicalExpr:
		c.expr(x.Left)
		c.chunk.emit(OpDup, c.line(x))
		var jmp int
		switch x.Op {
		case "&&":
			jmp = c.chunk.emit32(OpJumpIfFalse, 0, c.line(x))
		case "||":
			jmp = c.chunk.emit32(OpJumpIfTrue, 0, c.line(x))
		case "??":
			c.chunk.emit(OpIsNullish, c.line(x))
			jmp = c.chunk.emit32(OpJumpIfFalse, 0, c.line(x))
			c.chunk.emit(OpPop, c.line(x))
			c.expr(x.Right)
			c.chunk.patch32(jmp, uint32(int32(len(c.chunk.Code)-(jmp+5))))
			return
		default:
			c.fail(x, "opérateur logique non compilé : "+x.Op)
		}
		c.chunk.emit(OpPop, c.line(x))
		c.expr(x.Right)
		c.chunk.patch32(jmp, uint32(int32(len(c.chunk.Code)-(jmp+5))))

	case *ast.ConditionalExpr:
		c.expr(x.Test)
		jf := c.chunk.emit32(OpJumpIfFalse, 0, c.line(x))
		c.expr(x.Consequent)
		jend := c.chunk.emit32(OpJump, 0, c.line(x))
		c.chunk.patch32(jf, uint32(int32(len(c.chunk.Code)-(jf+5))))
		c.expr(x.Alternate)
		c.chunk.patch32(jend, uint32(int32(len(c.chunk.Code)-(jend+5))))

	case *ast.AssignExpr:
		c.assign(x)

	case *ast.SequenceExpr:
		for i, sub := range x.Exprs {
			c.expr(sub)
			if i < len(x.Exprs)-1 {
				c.chunk.emit(OpPop, c.line(x))
			}
		}

	case *ast.MemberExpr:
		if x.Optional {
			c.compileOptionalMember(x)
			return
		}
		if _, ok := x.Object.(*ast.SuperExpr); ok {
			if x.Computed {
				c.fail(x, "super calculé n'est pas compilé")
			}
			id, ok := x.Property.(*ast.Ident)
			if !ok {
				c.fail(x, "nom de propriété non compilé")
			}
			c.chunk.emit32(OpSuperGet, c.constString(id.Name), c.line(x))
			return
		}
		c.expr(x.Object)
		if x.Computed {
			c.expr(x.Property)
			c.chunk.emit(OpGetElem, c.line(x))
			return
		}
		id, ok := x.Property.(*ast.Ident)
		if !ok {
			c.fail(x, "nom de propriété non compilé")
		}
		c.chunk.emit32(OpGetProp, c.constString(id.Name), c.line(x))

	case *ast.CallExpr:
		if x.Optional {
			c.compileOptionalCall(x)
			return
		}
		if _, ok := x.Callee.(*ast.SuperExpr); ok {
			if c.callHasSpread(x.Args) {
				c.compileSpreadArgArray(x.Args, c.line(x))
				c.chunk.emit(OpSuperCallSpread, c.line(x))
				return
			}
			for _, a := range x.Args {
				c.expr(a)
			}
			c.chunk.emit16(OpSuperCall, uint16(len(x.Args)), c.line(x))
			return
		}
		if mem, ok := x.Callee.(*ast.MemberExpr); ok && !mem.Computed {
			if _, ok := mem.Object.(*ast.SuperExpr); ok {
				id, ok := mem.Property.(*ast.Ident)
				if !ok {
					c.fail(x, "nom de propriété non compilé")
				}
				c.chunk.emit32(OpSuperGet, c.constString(id.Name), c.line(x))
				for _, a := range x.Args {
					if _, isSpread := a.(*ast.SpreadElement); isSpread {
						c.fail(x, "la diffusion d'arguments n'est pas encore compilée")
					}
					c.expr(a)
				}
				c.chunk.emit16(OpCall, uint16(len(x.Args)), c.line(x))
				return
			}
			if id, ok := mem.Property.(*ast.Ident); ok {
				c.expr(mem.Object)
				if c.callHasSpread(x.Args) {
					c.chunk.emit(OpDup, c.line(x))
					c.chunk.emit32(OpGetProp, c.constString(id.Name), c.line(x))
					c.compileSpreadArgArray(x.Args, c.line(x))
					c.chunk.emit(OpCallSpreadThis, c.line(x))
					return
				}
				for _, a := range x.Args {
					c.expr(a)
				}
				argc := len(x.Args)
				constIdx := c.constString(id.Name)
				c.chunk.emit32(OpCallMethod, packVar(argc, int(constIdx)), c.line(x))
				return
			}
		}
		if mem, ok := x.Callee.(*ast.MemberExpr); ok && mem.Computed {
			c.expr(mem.Object)
			c.chunk.emit(OpDup, c.line(x))
			c.expr(mem.Property)
			c.chunk.emit(OpGetElem, c.line(x))
			c.chunk.emit32(OpNewArray, 0, c.line(x))
			for _, a := range x.Args {
				if _, isSpread := a.(*ast.SpreadElement); isSpread {
					c.fail(x, "la diffusion d'arguments n'est pas encore compilée")
				}
				c.expr(a)
				c.chunk.emit(OpArrayPush, c.line(x))
			}
			c.chunk.emit(OpCallSpreadThis, c.line(x))
			return
		}
		if c.callHasSpread(x.Args) {
			c.expr(x.Callee)
			c.compileSpreadArgArray(x.Args, c.line(x))
			c.chunk.emit(OpCallSpread, c.line(x))
			return
		}
		c.expr(x.Callee)
		for _, a := range x.Args {
			c.expr(a)
		}
		c.chunk.emit16(OpCall, uint16(len(x.Args)), c.line(x))

	case *ast.ArrayLit:
		if c.arrayHasSpread(x.Elements) {
			c.chunk.emit32(OpNewArray, 0, c.line(x))
			for _, el := range x.Elements {
				if el == nil {
					c.chunk.emit(OpUndefined, c.line(x))
					c.chunk.emit(OpArrayPush, c.line(x))
					continue
				}
				if sp, ok := el.(*ast.SpreadElement); ok {
					c.expr(sp.Argument)
					c.chunk.emit(OpArraySpread, c.line(x))
					continue
				}
				c.expr(el)
				c.chunk.emit(OpArrayPush, c.line(x))
			}
			return
		}
		for _, el := range x.Elements {
			if el == nil {
				c.chunk.emit(OpUndefined, c.line(x))
				continue
			}
			c.expr(el)
		}
		c.chunk.emit32(OpNewArray, uint32(len(x.Elements)), c.line(x))

	case *ast.ObjectLit:
		c.chunk.emit(OpNewObject, c.line(x))
		for _, p := range x.Properties {
			if p.Kind == "spread" || p.Kind == "rest" {
				c.expr(p.Value)
				c.chunk.emit(OpObjectAssign, c.line(x))
				continue
			}
			if p.Kind == "get" || p.Kind == "set" {
				name := propertyName(p.Key)
				if name == "" {
					c.fail(x, "nom d'accesseur non compilé")
				}
				c.chunk.emit(OpDup, c.line(x))
				saved := c.pendingFuncName
				c.pendingFuncName = name
				c.expr(p.Value)
				c.pendingFuncName = saved
				flag := uint32(0)
				if p.Kind == "set" {
					flag = 1 << 31
				}
				c.chunk.emit32(OpDefAccessor, flag|c.constString(name), c.line(x))
				c.chunk.emit(OpPop, c.line(x))
				continue
			}
			if p.Computed {
				c.chunk.emit(OpDup, c.line(x))
				c.expr(p.Key)
				c.expr(p.Value)
				c.chunk.emit(OpSetElem, c.line(x))
				c.chunk.emit(OpPop, c.line(x))
				continue
			}
			name := propertyName(p.Key)
			if name == "" {
				c.fail(x, "nom de propriété non compilé")
			}
			c.chunk.emit(OpDup, c.line(x))
			saved := c.pendingFuncName
			c.pendingFuncName = name
			c.expr(p.Value)
			c.pendingFuncName = saved
			c.chunk.emit32(OpSetProp, c.constString(name), c.line(x))
			c.chunk.emit(OpPop, c.line(x))
		}

	case *ast.FunctionExpr:
		name := ""
		if x.Name != nil {
			name = x.Name.Name
		} else if c.pendingFuncName != "" {
			name = c.pendingFuncName
		}
		fn := c.compileFunction(x, name, true)
		c.chunk.emit32(OpConst, c.chunk.addConst(Const{Kind: ConstFunction, Fn: fn}), c.line(x))

	case *ast.ArrowFunction:
		fn := c.compileArrow(x)
		c.chunk.emit32(OpConst, c.chunk.addConst(Const{Kind: ConstFunction, Fn: fn}), c.line(x))

	case *ast.ChainExpr:
		c.expr(x.Expression)
	case *ast.NewExpr:
		c.expr(x.Callee)
		if c.callHasSpread(x.Args) {
			c.compileSpreadArgArray(x.Args, c.line(x))
			c.chunk.emit(OpNewSpread, c.line(x))
			break
		}
		for _, a := range x.Args {
			c.expr(a)
		}
		c.chunk.emit16(OpNew, uint16(len(x.Args)), c.line(x))
	case *ast.ThisExpr:
		c.chunk.emit(OpThis, c.line(x))
	case *ast.ClassExpr:
		c.compileClass(x)
	case *ast.YieldExpr:
		if x.Argument != nil {
			c.expr(x.Argument)
		} else {
			c.chunk.emit(OpUndefined, c.line(x))
		}
		flag := uint16(0)
		if x.Delegate {
			flag = 1
		}
		c.chunk.emit16(OpYield, flag, c.line(x))
	case *ast.AwaitExpr:
		if x.Argument != nil {
			c.expr(x.Argument)
		} else {
			c.chunk.emit(OpUndefined, c.line(x))
		}
		c.chunk.emit(OpAwait, c.line(x))
	case *ast.TaggedTemplate:
		c.expr(x.Tag)
		c.chunk.emit32(OpNewArray, 0, c.line(x))
		for _, q := range x.Quasi.Quasis {
			c.chunk.emit32(OpConst, c.constString(templateChunk(q)), c.line(x))
			c.chunk.emit(OpArrayPush, c.line(x))
		}
		for _, sub := range x.Quasi.Exprs {
			c.expr(sub)
		}
		c.chunk.emit16(OpCall, uint16(1+len(x.Quasi.Exprs)), c.line(x))
	case *ast.MetaProperty:
		if x.Meta == "new" && x.Property == "target" {
			c.chunk.emit(OpNewTarget, c.line(x))
			break
		}
		c.fail(x, "import.meta n'est pas compilé")
	case *ast.SuperExpr:
		c.fail(x, "super nu n'est pas compilé")
	case *ast.RegExpLit:
		pat, flags, ok := splitRegexpRaw(x.Raw)
		if !ok {
			c.fail(x, "littéral d'expression régulière invalide")
		}
		if _, err := compileJSRegexp(pat, flags); err != nil {
			c.fail(x, "expression régulière invalide")
		}
		c.chunk.emit32(OpGetGlobal, c.constString("RegExp"), c.line(x))
		c.chunk.emit32(OpConst, c.constString(pat), c.line(x))
		c.chunk.emit32(OpConst, c.constString(flags), c.line(x))
		c.chunk.emit16(OpNew, 2, c.line(x))
	case *ast.BigIntLit:
		n, err := parseBigInt64(x.Raw)
		if err != nil {
			c.fail(x, "BigInt n'est pas compilable en int64")
		}
		c.chunk.emit32(OpConst, c.chunk.addConst(Const{Kind: ConstBigInt, Big: n}), c.line(x))
	case *ast.ArrayPattern, *ast.ObjectPattern, *ast.AssignPattern, *ast.RestElement:
		c.fail(e, "motif de décomposition inattendu en position d'expression")

	default:
		c.fail(e, "expression non compilée : "+e.NodeType())
	}
}

func isAnonymousFnDef(e ast.Expr) bool {
	switch t := e.(type) {
	case *ast.FunctionExpr:
		return t.Name == nil
	case *ast.ArrowFunction:
		return true
	case *ast.ClassExpr:
		if t.Name != nil {
			return false
		}
		for _, m := range t.Body {
			if !m.Static {
				continue
			}
			if id, ok := m.Key.(*ast.Ident); ok && id.Name == "name" {
				return false
			}
		}
		return true
	}
	return false
}

func (c *Compiler) compileClass(x *ast.ClassExpr) {
	name := "anonymous"
	if x.Name != nil {
		name = x.Name.Name
	} else if c.pendingFuncName != "" {
		name = c.pendingFuncName
	}
	var ctorMember *ast.ClassMember
	for _, m := range x.Body {
		if m.Kind == "constructor" {
			ctorMember = m
			break
		}
	}

	if ctorMember != nil {
		if fnExpr, ok := ctorMember.Value.(*ast.FunctionExpr); ok {
			fn := c.compileFunction(fnExpr, name, true)
			c.chunk.emit32(OpConst, c.chunk.addConst(Const{Kind: ConstFunction, Fn: fn}), c.line(x))
		} else {
			c.fail(x, "constructeur de classe invalide")
		}
	} else if x.SuperClass != nil {
		fnChunk := NewChunk(name)
		fnChunk.Locals = 1
		fnChunk.ArgumentsSlot = 0
		line := c.line(x)
		fnChunk.emit32(OpGetLocal, packVar(0, 0), line)
		fnChunk.emit(OpSuperCallSpread, line)
		fnChunk.emit(OpReturn, line)
		c.chunk.emit32(OpConst, c.chunk.addConst(Const{Kind: ConstFunction, Fn: fnChunk}), line)
	} else {
		fnChunk := NewChunk(name)
		fnChunk.emit(OpUndefined, c.line(x))
		fnChunk.emit(OpReturn, c.line(x))
		c.chunk.emit32(OpConst, c.chunk.addConst(Const{Kind: ConstFunction, Fn: fnChunk}), c.line(x))
	}
	if x.SuperClass != nil {
		c.chunk.emit(OpDup, c.line(x))
		c.expr(x.SuperClass)
		c.chunk.emit(OpSetProto, c.line(x))
		c.chunk.emit(OpPop, c.line(x))
		c.chunk.emit(OpDup, c.line(x))
		c.chunk.emit32(OpGetProp, c.constString("prototype"), c.line(x))
		c.expr(x.SuperClass)
		c.chunk.emit32(OpGetProp, c.constString("prototype"), c.line(x))
		c.chunk.emit(OpSetProto, c.line(x))
		c.chunk.emit(OpPop, c.line(x))
	}

	for _, m := range x.Body {
		if m.Kind == "constructor" {
			continue
		}
		if m.Kind == "get" || m.Kind == "set" || m.Kind == "method" {
			methodName := ""
			if id, ok := m.Key.(*ast.Ident); ok {
				methodName = id.Name
			} else if strLit, ok := m.Key.(*ast.StringLit); ok {
				methodName = unquote(strLit.Raw)
			}
			if methodName == "" {
				continue
			}

			if m.Static {
				c.chunk.emit(OpDup, c.line(x))
			} else {
				c.chunk.emit(OpDup, c.line(x))
				c.chunk.emit32(OpGetProp, c.constString("prototype"), c.line(x))
			}
			c.exprNamed(m.Value, methodName)
			c.chunk.emit(OpSetHomeObject, c.line(m))
			if m.Kind == "get" || m.Kind == "set" {
				flag := uint32(0)
				if m.Kind == "set" {
					flag = 1 << 31
				}
				c.chunk.emit32(OpDefAccessor, flag|c.constString(methodName), c.line(x))
			} else {
				c.chunk.emit32(OpSetProp, c.constString(methodName), c.line(x))
			}
			c.chunk.emit(OpPop, c.line(x))
		}
	}
}

var binaryOps = map[string]Op{
	"+": OpAdd, "-": OpSub, "*": OpMul, "/": OpDiv, "%": OpMod, "**": OpPow,
	"&": OpBitAnd, "|": OpBitOr, "^": OpBitXor,
	"<<": OpShl, ">>": OpShr, ">>>": OpUShr,
	"==": OpEq, "!=": OpNe, "===": OpStrictEq, "!==": OpStrictNe,
	"<": OpLt, ">": OpGt, "<=": OpLe, ">=": OpGe,
	"instanceof": OpInstanceof,
	"in":         OpIn,
}

func (c *Compiler) loadIdent(id *ast.Ident) {
	if c.tdzHead[id.Name] {
		c.chunk.emit32(OpGetGlobal, c.constString("ReferenceError"), c.line(id))
		c.chunk.emit32(OpConst, c.constString("Cannot access '"+id.Name+"' before initialization"), c.line(id))
		c.chunk.emit16(OpNew, 1, c.line(id))
		c.chunk.emit(OpThrow, c.line(id))
		return
	}
	if d, slot, ok := c.resolve(id.Name); ok {
		c.chunk.emit32(OpGetLocal, packVar(d, slot), c.line(id))
		return
	}
	c.chunk.emit32(OpGetGlobal, c.constString(id.Name), c.line(id))
}

// storeIdent écrit la valeur au sommet de la pile et l'y laisse.
func (c *Compiler) storeIdent(id *ast.Ident) {
	d, slot, local := c.resolve(id.Name)
	constant := c.constNames[id.Name]
	if local {
		owner := c.scope
		for i := 0; i < d; i++ {
			owner = owner.parent
		}
		constant = owner.consts[id.Name]
	}
	if constant {
		c.chunk.emit32(OpGetGlobal, c.constString("TypeError"), c.line(id))
		c.chunk.emit32(OpConst, c.constString("Assignment to constant variable"), c.line(id))
		c.chunk.emit16(OpNew, 1, c.line(id))
		c.chunk.emit(OpThrow, c.line(id))
		return
	}
	if local {
		c.chunk.emit32(OpSetLocal, packVar(d, slot), c.line(id))
		return
	}
	c.chunk.emit32(OpSetGlobal, c.constString(id.Name), c.line(id))
}

func (c *Compiler) assign(x *ast.AssignExpr) {
	if x.Op != "=" {
		op, ok := binaryOps[x.Op[:len(x.Op)-1]]
		if !ok {
			c.fail(x, "affectation composée non compilée : "+x.Op)
		}
		switch t := x.Target.(type) {
		case *ast.Ident:
			c.loadIdent(t)
			c.expr(x.Value)
			c.chunk.emit(op, c.line(x))
			c.storeIdent(t)
			return
		case *ast.MemberExpr:
			if t.Computed {
				objSlot := c.allocLocal()
				keySlot := c.allocLocal()
				valSlot := c.allocLocal()
				c.expr(t.Object)
				c.chunk.emit32(OpSetLocal, packVar(0, objSlot), c.line(x))
				c.chunk.emit(OpPop, c.line(x))
				c.expr(t.Property)
				c.chunk.emit32(OpSetLocal, packVar(0, keySlot), c.line(x))
				c.chunk.emit(OpPop, c.line(x))
				c.chunk.emit32(OpGetLocal, packVar(0, objSlot), c.line(x))
				c.chunk.emit32(OpGetLocal, packVar(0, keySlot), c.line(x))
				c.chunk.emit(OpGetElem, c.line(x))
				c.expr(x.Value)
				c.chunk.emit(op, c.line(x))
				c.chunk.emit32(OpSetLocal, packVar(0, valSlot), c.line(x))
				c.chunk.emit(OpPop, c.line(x))
				c.chunk.emit32(OpGetLocal, packVar(0, objSlot), c.line(x))
				c.chunk.emit32(OpGetLocal, packVar(0, keySlot), c.line(x))
				c.chunk.emit32(OpGetLocal, packVar(0, valSlot), c.line(x))
				c.chunk.emit(OpSetElem, c.line(x))
				return
			}
			c.expr(t.Object)
			c.chunk.emit(OpDup, c.line(x))
			id, ok := t.Property.(*ast.Ident)
			if !ok {
				c.fail(x, "nom de propriété non compilé")
			}
			c.chunk.emit32(OpGetProp, c.constString(id.Name), c.line(x))
			c.expr(x.Value)
			c.chunk.emit(op, c.line(x))
			c.chunk.emit32(OpSetProp, c.constString(id.Name), c.line(x))
			return
		default:
			c.fail(x, "cible d'affectation composée non compilée")
		}
	}

	switch t := x.Target.(type) {
	case *ast.Ident:
		c.exprNamed(x.Value, t.Name)
		c.storeIdent(t)
	case *ast.MemberExpr:
		c.expr(t.Object)
		if t.Computed {
			c.expr(t.Property)
			c.expr(x.Value)
			c.chunk.emit(OpSetElem, c.line(x))
			return
		}
		id, ok := t.Property.(*ast.Ident)
		if !ok {
			c.fail(x, "nom de propriété non compilé")
		}
		c.exprNamed(x.Value, id.Name)
		c.chunk.emit32(OpSetProp, c.constString(id.Name), c.line(x))
	case *ast.ArrayPattern, *ast.ObjectPattern:
		c.expr(x.Value)
		c.chunk.emit(OpDup, c.line(x))
		c.compileDestructuring(t, false)
	default:
		c.fail(x, "cible d'affectation non compilée : "+x.Target.NodeType())
	}
}

// ─── Fonctions ──────────────────────────────────────────────────────────────

// compileFunction compile une fonction. selfBind demande la liaison du nom à
// soi-même, ce qui ne vaut que pour une EXPRESSION de fonction nommée.
func (c *Compiler) compileFunction(fn *ast.FunctionExpr, name string, selfBind bool) *Chunk {
	var self *ast.Ident
	if selfBind {
		self = fn.Name
	}
	savedGen := c.inGenerator
	c.inGenerator = fn.Generator || fn.Async
	ch := c.compileBody(name, fn.Params, fn.Body, self)
	c.inGenerator = savedGen
	ch.Generator = fn.Generator || fn.Async
	ch.Async = fn.Async
	ch.SafeEnv = regularFrameBody(ch)
	ch.safeEnvVerified = ch.SafeEnv
	return ch
}

func (c *Compiler) compileArrow(fn *ast.ArrowFunction) *Chunk {
	body, isBlock := fn.Body.(*ast.BlockStmt)
	if !isBlock {
		// Corps concis : la valeur de l'expression est retournée.
		expr, ok := fn.Body.(ast.Expr)
		if !ok {
			c.fail(fn, "corps de fonction fléchée non compilé")
		}
		body = &ast.BlockStmt{Base: ast.Base{P: fn.Pos()},
			Body: []ast.Stmt{&ast.ReturnStmt{Base: ast.Base{P: fn.Pos()}, Argument: expr}}}
	}
	name := ""
	if c.pendingFuncName != "" {
		name = c.pendingFuncName
	}
	savedArrow := c.inArrow
	c.inArrow = true
	ch := c.compileBody(name, fn.Params, body, nil)
	c.inArrow = savedArrow
	ch.Async = fn.Async
	ch.Arrow = true
	if fn.Async {
		ch.Generator = true
	}
	// Arrow environments remain on the generic path in this first tranche.
	ch.SafeEnv = false
	return ch
}

// compileBody compile un corps de fonction dans une portée neuve.
func (c *Compiler) compileBody(name string, params []ast.Expr, body *ast.BlockStmt, self *ast.Ident) *Chunk {
	sub := NewChunk(name)
	sub.Strict = c.strict || hasUseStrict(body)
	scope := &funcScope{parent: c.scope, names: map[string]int{}, chunk: sub}

	// Chaque POSITION de paramètre reçoit son propre emplacement, même si deux
	// paramètres portent le même nom : « function f(b, b) » est licite hors mode
	// strict, et le dernier l'emporte. Partager l'emplacement faisait que le
	// nombre de paramètres dépassait la taille de l'environnement, et l'appel
	// indexait hors borne — défaut trouvé par le fuzz.
	type defParam struct {
		slot int
		expr ast.Expr
		line int
		name string
	}
	type destParam struct {
		slot int
		pat  ast.Expr
		line int
	}
	var defs []defParam
	var dests []destParam
	nPos := 0
	for _, p := range params {
		switch t := p.(type) {
		case *ast.Ident:
			slot := sub.Locals
			sub.Locals++
			scope.names[t.Name] = slot
			nPos++
		case *ast.AssignPattern:
			slot := sub.Locals
			sub.Locals++
			nPos++
			dp := defParam{slot: slot, expr: t.Default, line: c.line(p)}
			if id, ok := t.Target.(*ast.Ident); ok {
				scope.names[id.Name] = slot
				dp.name = id.Name
				defs = append(defs, dp)
				break
			}
			defs = append(defs, dp)
			dests = append(dests, destParam{slot: slot, pat: t.Target, line: c.line(p)})
		case *ast.ObjectPattern, *ast.ArrayPattern:
			slot := sub.Locals
			sub.Locals++
			nPos++
			dests = append(dests, destParam{slot: slot, pat: t, line: c.line(p)})
		case *ast.RestElement:
			id, ok := t.Argument.(*ast.Ident)
			if !ok {
				c.fail(p, "reste de paramètre non identifiant")
			}
			slot := sub.Locals
			sub.Locals++
			scope.names[id.Name] = slot
			sub.RestSlot = slot
		default:
			c.fail(p, "paramètre non compilé : décomposition ou reste")
		}
	}
	sub.Params = nPos
	sub.ArgumentsSlot = -1
	if !c.inArrow {
		if _, exists := scope.names["arguments"]; !exists && (bodyNeedsArguments(body) || exprsNeedArguments(params)) {
			sub.ArgumentsSlot = scope.declare("arguments")
		}
	}

	// Une expression de fonction nommée voit son propre nom. L'emplacement est
	// renseigné par l'appel, qui seul dispose de la valeur de la fonction.
	if self != nil {
		if _, exists := scope.names[self.Name]; !exists {
			sub.SelfSlot = scope.declare(self.Name)
		}
	}

	savedChunk, savedScope, savedLoops, savedStrict := c.chunk, c.scope, c.loops, c.strict
	c.chunk, c.scope, c.loops, c.strict = sub, scope, nil, sub.Strict

	for _, d := range defs {
		c.chunk.emit32(OpGetLocal, packVar(0, d.slot), d.line)
		c.chunk.emit(OpDup, d.line)
		c.chunk.emit(OpUndefined, d.line)
		c.chunk.emit(OpStrictNe, d.line)
		jmp := c.chunk.emit32(OpJumpIfTrue, 0, d.line)
		c.chunk.emit(OpPop, d.line)
		c.exprNamed(d.expr, d.name)
		c.chunk.emit32(OpSetLocal, packVar(0, d.slot), d.line)
		c.chunk.patch32(jmp, uint32(int32(len(c.chunk.Code)-(jmp+5))))
		c.chunk.emit(OpPop, d.line)
	}
	for _, d := range dests {
		c.chunk.emit32(OpGetLocal, packVar(0, d.slot), d.line)
		c.compileDestructuring(d.pat, true)
	}

	c.hoistVars(body.Body)
	c.hoistLexicals(body.Body)
	c.hoistFunctions(body.Body)
	for _, st := range body.Body {
		c.stmt(st, false)
	}
	// Retour implicite.
	c.chunk.emit(OpUndefined, 0)
	c.chunk.emit(OpReturn, 0)

	c.chunk, c.scope, c.loops, c.strict = savedChunk, savedScope, savedLoops, savedStrict
	filterArchtimeCandidates(sub)
	return sub
}

func hasUseStrict(body *ast.BlockStmt) bool {
	if body == nil {
		return false
	}
	for _, st := range body.Body {
		es, ok := st.(*ast.ExpressionStmt)
		if !ok {
			break
		}
		lit, ok := es.Expression.(*ast.StringLit)
		if !ok {
			break
		}
		switch lit.Raw {
		case `"use strict"`, `'use strict'`:
			return true
		}
	}
	return false
}

// ─── Utilitaires ────────────────────────────────────────────────────────────

func propertyName(key ast.Expr) string {
	switch k := key.(type) {
	case *ast.Ident:
		return k.Name
	case *ast.StringLit:
		return unquote(k.Raw)
	case *ast.NumberLit:
		return numberToString(parseNumber(k.Raw))
	}
	return ""
}

// unquote retire les délimiteurs d'un littéral de chaîne et interprète ses
// échappements.
//
// unquote interprète les échappements courants, Unicode \uXXXX / \u{...}
// et les octaux hérités.
func unquote(raw string) string {
	if len(raw) < 2 {
		return raw
	}
	return unescape(raw[1 : len(raw)-1])
}

func templateChunk(raw string) string {
	// Un fragment de gabarit porte « ` », « ${ », « } » selon sa position.
	s := raw
	s = trimPrefixAny(s, "`", "}")
	s = trimSuffixAny(s, "`", "${")
	return unescape(s)
}

func trimPrefixAny(s string, prefixes ...string) string {
	for _, p := range prefixes {
		if len(s) >= len(p) && s[:len(p)] == p {
			return s[len(p):]
		}
	}
	return s
}

func trimSuffixAny(s string, suffixes ...string) string {
	for _, p := range suffixes {
		if len(s) >= len(p) && s[len(s)-len(p):] == p {
			return s[:len(s)-len(p)]
		}
	}
	return s
}

func unescapeToUTF16(s string) *str.String {
	if !containsBackslash(s) {
		return str.FromGo(s)
	}
	u := make([]uint16, 0, len(s))
	rs := []rune(s)
	appendRune := func(r rune) {
		if r > 0xFFFF {
			r1, r2 := utf16.EncodeRune(r)
			u = append(u, uint16(r1), uint16(r2))
			return
		}
		u = append(u, uint16(r))
	}
	for i := 0; i < len(rs); i++ {
		if rs[i] != '\\' || i+1 >= len(rs) {
			appendRune(rs[i])
			continue
		}
		i++
		switch rs[i] {
		case 'n':
			appendRune('\n')
		case 't':
			appendRune('\t')
		case 'r':
			appendRune('\r')
		case 'b':
			appendRune('\b')
		case 'f':
			appendRune('\f')
		case 'v':
			appendRune('\v')
		case 'x':
			if i+2 < len(rs) {
				v := hexVal(rs[i+1])*16 + hexVal(rs[i+2])
				u = append(u, uint16(v))
				i += 2
			}
		case 'u':
			if i+1 < len(rs) && rs[i+1] == '{' {
				j := i + 2
				v := 0
				for j < len(rs) && rs[j] != '}' {
					v = v*16 + hexVal(rs[j])
					j++
				}
				appendRune(rune(v))
				i = j
			} else if i+4 < len(rs) {
				v := 0
				for k := 1; k <= 4; k++ {
					v = v*16 + hexVal(rs[i+k])
				}
				u = append(u, uint16(v))
				i += 4
			}
		case '\n':
		default:
			if rs[i] >= '0' && rs[i] <= '7' {
				v := int(rs[i] - '0')
				n := 1
				for n < 3 && i+1 < len(rs) && rs[i+1] >= '0' && rs[i+1] <= '7' {
					i++
					v = v*8 + int(rs[i]-'0')
					n++
				}
				appendRune(rune(v))
			} else {
				appendRune(rs[i])
			}
		}
	}
	return str.FromUTF16(u)
}

func unescape(s string) string {
	if !containsBackslash(s) {
		return s
	}
	out := make([]rune, 0, len(s))
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		if rs[i] != '\\' || i+1 >= len(rs) {
			out = append(out, rs[i])
			continue
		}
		i++
		switch rs[i] {
		case 'n':
			out = append(out, '\n')
		case 't':
			out = append(out, '\t')
		case 'r':
			out = append(out, '\r')
		case 'b':
			out = append(out, '\b')
		case 'f':
			out = append(out, '\f')
		case 'v':
			out = append(out, '\v')
		case 'x':
			if i+2 < len(rs) {
				v := hexVal(rs[i+1])*16 + hexVal(rs[i+2])
				out = append(out, rune(v))
				i += 2
			}
		case 'u':
			if i+1 < len(rs) && rs[i+1] == '{' {
				j := i + 2
				v := 0
				for j < len(rs) && rs[j] != '}' {
					v = v*16 + hexVal(rs[j])
					j++
				}
				out = append(out, rune(v))
				i = j
			} else if i+4 < len(rs) {
				v := 0
				for k := 1; k <= 4; k++ {
					v = v*16 + hexVal(rs[i+k])
				}
				out = append(out, rune(v))
				i += 4
			}
		case '\n':
			// Continuation de ligne : rien n'est produit.
		default:
			if rs[i] >= '0' && rs[i] <= '7' {
				v := int(rs[i] - '0')
				n := 1
				for n < 3 && i+1 < len(rs) && rs[i+1] >= '0' && rs[i+1] <= '7' {
					i++
					v = v*8 + int(rs[i]-'0')
					n++
				}
				out = append(out, rune(v))
			} else {
				out = append(out, rs[i])
			}
		}
	}
	return string(out)
}

func containsBackslash(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' {
			return true
		}
	}
	return false
}

func hexVal(r rune) int {
	switch {
	case r >= '0' && r <= '9':
		return int(r - '0')
	case r >= 'a' && r <= 'f':
		return int(r-'a') + 10
	case r >= 'A' && r <= 'F':
		return int(r-'A') + 10
	}
	return 0
}

var _ = str.Empty
